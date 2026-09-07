package testkit

// 계약 점검 헬퍼 — Extension 이 SDK 계약을 지키는지 **Core 없이** 확인한다.
//
// 각 함수는 위반 목록을 문자열 슬라이스로 돌려준다(비어 있으면 통과). testing 패키지에
// 의존하지 않는 이유는 확장의 통합 테스트·로컬 점검 도구·CI 스크립트에서도 그대로 쓸 수
// 있어야 하기 때문이다.
//
//	if issues := testkit.CheckExtension(ext); len(issues) > 0 {
//	    t.Fatalf("SDK 계약 위반:\n- %s", strings.Join(issues, "\n- "))
//	}

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// reservedSubPaths 는 Core 관리 API 가 선점한 서브경로다.
//
// ⚠ Core(internal/extensionhost/manifest_load.go)·extserver(run.go)와 **같은 목록**이어야 한다.
// 여기가 뒤처지면 testkit 은 통과하는데 설치가 거부되는 확장이 만들어진다.
var reservedSubPaths = []string{"health", "install-events", "enable", "disable", "update", "rollback", "assets"}

// reservedExtensionIDs 는 확장 ID 로 쓸 수 없는 이름이다(Core 관리 API 정적 경로).
var reservedExtensionIDs = []string{"catalog", "install", "ui-manifest", "_host", "trusted-publishers"}

// CheckManifest 는 Manifest 계약을 점검한다(SDK Validate + Core 예약어 규칙).
//
// extensionv1.Manifest.Validate 는 SDK 가 아는 규칙만 본다. 예약 ID·예약 서브경로는 Core 의
// 라우팅 사정이라 SDK 에 넣을 수 없으므로, 설치하기 전에 여기서 알려 준다.
func CheckManifest(m extv1.Manifest) []string {
	var issues []string
	if err := m.Validate(); err != nil {
		for _, line := range strings.Split(err.Error(), "\n") {
			if s := strings.TrimSpace(line); s != "" {
				issues = append(issues, s)
			}
		}
	}
	for _, id := range reservedExtensionIDs {
		if m.ID == id {
			issues = append(issues, fmt.Sprintf("id %q 는 Core 관리 API 가 선점한 예약 ID 입니다(설치가 거부됩니다)", m.ID))
		}
	}
	routePrefix := "/api/extensions/" + m.ID + "/"
	for _, r := range m.Routes {
		rel := strings.TrimPrefix(r.Path, routePrefix)
		if rel == r.Path {
			continue // 접두 검사는 Validate 가 이미 했다
		}
		head := rel
		if i := strings.IndexByte(head, '/'); i >= 0 {
			head = head[:i]
		}
		for _, reserved := range reservedSubPaths {
			if head == reserved {
				issues = append(issues, fmt.Sprintf("routes[].path %q 는 Core 관리 API 가 선점한 예약 서브경로입니다(%s)", r.Path, reserved))
			}
		}
	}
	// capability 는 Manifest 가 요구한 최소권한이다 — 모르는 이름은 Validate 가 잡지만,
	// 여기서 "선언했는데 라우트가 하나도 없는" 과다 선언도 알려 준다(최소권한 점검).
	if len(m.Capabilities) > 0 && !m.Backend.Enabled {
		issues = append(issues, "backend.enabled=false 인데 capability 를 선언했습니다 — 백엔드가 없으면 Host 서비스를 쓸 수 없습니다")
	}
	return issues
}

// CheckRoutes 는 Extension.Routes() 가 Manifest 선언과 일치하는지 점검한다.
//
// Core 는 Manifest 의 routes[].path 아래에만 핸들러를 마운트한다. 구현이 선언에 없는 경로를
// 돌려주면 그 라우트는 **영원히 호출되지 않고**, 증상은 "코드는 있는데 404"로만 드러난다.
func CheckRoutes(ext extv1.Extension) []string {
	var issues []string
	m := ext.Manifest()
	declared := make(map[string]bool, len(m.Routes))
	prefix := "/api/extensions/" + m.ID
	for _, r := range m.Routes {
		declared[strings.TrimPrefix(r.Path, prefix)] = true
	}
	seen := map[string]bool{}
	for _, h := range ext.Routes() {
		method := strings.ToUpper(strings.TrimSpace(h.Method))
		if method == "" {
			method = http.MethodGet
		}
		if h.Handler == nil {
			issues = append(issues, fmt.Sprintf("라우트 %s %s 의 핸들러가 nil 입니다", method, h.Pattern))
		}
		if !strings.HasPrefix(h.Pattern, "/") {
			issues = append(issues, fmt.Sprintf("라우트 패턴은 마운트 접두 기준 상대 경로여야 합니다(/ 로 시작): %q", h.Pattern))
		}
		key := method + " " + h.Pattern
		if seen[key] {
			issues = append(issues, fmt.Sprintf("라우트가 중복 선언되었습니다: %s", key))
		}
		seen[key] = true
		if h.Permission != "" && !extv1.ValidPermissionKey(m.ID, h.Permission) && strings.HasPrefix(h.Permission, "ext.") {
			issues = append(issues, fmt.Sprintf("라우트 %s 의 권한 키가 자기 네임스페이스를 벗어났습니다: %q", key, h.Permission))
		}
	}
	return issues
}

// CheckLifecycle 은 Start/Stop 계약을 점검한다.
//
// 확인하는 것:
//   - Start 가 주어진 ctx 취소를 존중하는가(오래 블로킹하지 않는가)
//   - Stop 이 **여러 번 불려도 안전한가**(멱등) — Core 는 정상 종료와 정리 경로에서 모두 부른다
//   - Manifest 가 호출마다 같은 값인가 — 런타임에 권한·라우트를 바꿔 검증을 우회할 수 없어야 한다
func CheckLifecycle(ext extv1.Extension, host extv1.HostContext) []string {
	var issues []string

	before := ext.Manifest()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- ext.Start(ctx, host) }()
	select {
	case err := <-done:
		if err != nil {
			issues = append(issues, fmt.Sprintf("Start 가 실패했습니다: %v", err))
			return issues
		}
	case <-ctx.Done():
		issues = append(issues, "Start 가 5초 안에 반환하지 않았습니다 — 오래 걸리는 작업은 고루틴으로 옮기세요")
		return issues
	}

	after := ext.Manifest()
	if before.ID != after.ID || before.Version != after.Version || len(before.Capabilities) != len(after.Capabilities) {
		issues = append(issues, "Manifest 가 호출마다 달라집니다 — 런타임에 선언을 바꾸면 Core 의 검증을 우회하게 됩니다")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := ext.Stop(stopCtx); err != nil {
		issues = append(issues, fmt.Sprintf("Stop 이 실패했습니다: %v", err))
	}
	// 멱등 확인 — 두 번째 Stop 도 오류 없이 끝나야 한다.
	if err := ext.Stop(stopCtx); err != nil {
		issues = append(issues, fmt.Sprintf("Stop 이 멱등하지 않습니다(두 번째 호출 실패): %v", err))
	}
	return issues
}

// CheckHealth 는 Health 계약을 점검한다.
//
// Health 는 **빠르게** 반환해야 한다(Core 가 주기적으로 폴링한다). 매번 외부 호출을 하면
// 폴링 주기마다 그 부하가 그대로 실린다 — 캐시해서 돌려주는 것이 계약이다.
func CheckHealth(ext extv1.Extension) []string {
	var issues []string
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := nowFunc()
	type result struct{ h extv1.Health }
	ch := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- result{extv1.Health{OK: false, Message: fmt.Sprint(r)}}
			}
		}()
		ch <- result{ext.Health(ctx)}
	}()
	select {
	case r := <-ch:
		if elapsed := nowFunc().Sub(start); elapsed > time.Second {
			issues = append(issues, fmt.Sprintf("Health 가 %s 걸립니다 — Core 가 주기적으로 폴링하므로 캐시해서 즉시 반환하세요", elapsed))
		}
		if r.h.CheckedAt.IsZero() {
			issues = append(issues, "Health.CheckedAt 이 비어 있습니다(언제 확인한 값인지 알 수 없습니다)")
		}
		if !r.h.OK && strings.TrimSpace(r.h.Message) == "" {
			issues = append(issues, "비정상 Health 에 사유(Message)가 없습니다 — 관리 화면에 원인이 표시되지 않습니다")
		}
	case <-ctx.Done():
		issues = append(issues, "Health 가 2초 안에 반환하지 않았습니다")
	}
	return issues
}

// CheckExtension 은 Manifest·라우트·생애주기·Health 계약을 한 번에 점검한다.
//
// host 가 zero value 면 capability 없는 HostContext 로 Start 를 시도한다 — capability 를
// 선언한 확장은 그 상태에서도 **죽지 않고** 오류를 돌려주거나 기능을 축소해야 한다.
func CheckExtension(ext extv1.Extension, host extv1.HostContext) []string {
	if ext == nil {
		return []string{"Extension 구현이 nil 입니다"}
	}
	issues := CheckManifest(ext.Manifest())
	issues = append(issues, CheckRoutes(ext)...)
	issues = append(issues, CheckLifecycle(ext, host)...)
	issues = append(issues, CheckHealth(ext)...)
	return issues
}
