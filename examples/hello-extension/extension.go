// Package hello 는 AbleOps SDK 의 최소 Extension 예제다.
//
// 여기 있는 것이 확장 하나가 갖춰야 할 전부다:
//
//	Manifest()  선언(권한·라우트·capability) — 호출마다 같은 값
//	Start()     capability 주입 시점. 필요한 서비스가 nil 인지 반드시 확인한다
//	Stop()      멱등해야 한다(여러 번 불려도 안전)
//	Health()    빠르게 반환한다(외부 호출을 매번 하지 않는다)
//	Routes()    Manifest 의 routes[].path 기준 **상대** 경로
//
// 이 패키지는 SDK 하나만 import 한다 — 외부 Extension 이 실제로 그래야 하기 때문이다.
package hello

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// manifestYAML 을 바이너리에 넣는다 — 배포 후 파일이 유실·위변조되어도
// 확장이 조용히 다른 권한을 갖는 일이 없다.
//
//go:embed manifest.yaml
var manifestYAML []byte

// ID 는 이 확장의 식별자다(manifest.yaml 의 id 와 같아야 한다).
const ID = "hello"

// PermView 는 조회 권한 키다. 문자열을 직접 쓰지 않고 SDK 로 조립해
// `ext.<id>.<action>` 네임스페이스 규칙을 코드로 강제한다.
var PermView = extv1.PermissionKey(ID, "view")

// Manifest 파싱은 프로세스당 1회만 한다(호출마다 같은 값을 돌려주어야 한다는 SDK 계약).
var (
	manifestOnce sync.Once
	manifestVal  extv1.Manifest
	manifestErr  error
)

// Extension 은 예제 확장의 런타임이다.
//
// Core 는 인스턴스 하나를 만들고 여러 요청이 동시에 핸들러를 부른다 —
// 런타임 상태는 반드시 보호한다.
type Extension struct {
	mu        sync.RWMutex
	clusters  extv1.ClusterRegistry
	startedAt time.Time
}

// 컴파일 타임 계약 확인 — 메서드 누락을 빌드에서 잡는다.
var _ extv1.Extension = (*Extension)(nil)

// New 는 예제 확장 인스턴스를 만든다.
func New() *Extension { return &Extension{} }

// Manifest 는 embed 된 선언을 돌려준다.
func (e *Extension) Manifest() extv1.Manifest {
	manifestOnce.Do(func() { manifestVal, manifestErr = extv1.ParseManifest(manifestYAML) })
	if manifestErr != nil {
		// 빌드에 포함된 자산의 문제이므로 런타임 조건이 아니다 — 테스트가 고정한다.
		panic("manifest.yaml 파싱 실패: " + manifestErr.Error())
	}
	return manifestVal
}

// Start 는 capability 를 주입받는 유일한 지점이다.
//
// ⚠ **선언하지 않은 capability 의 필드는 nil 이다.** RequireX 헬퍼는 그 확인을
// 사람이 읽을 수 있는 오류로 바꿔 준다 — nil 역참조로 패닉하는 것보다 낫다.
func (e *Extension) Start(ctx context.Context, host extv1.HostContext) error {
	clusters, err := host.RequireClusters()
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clusters = clusters
	e.startedAt = time.Now()
	return nil
}

// Stop 은 멱등하다(정리할 자원이 없어도 계약은 지킨다).
func (e *Extension) Stop(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clusters = nil
	return nil
}

// Health 는 즉시 반환한다 — 외부 호출을 여기서 하지 않는다.
func (e *Extension) Health(context.Context) extv1.Health {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.clusters == nil {
		return extv1.Health{OK: false, Message: "아직 시작되지 않았습니다", CheckedAt: time.Now()}
	}
	return extv1.Health{OK: true, Message: "정상", CheckedAt: time.Now()}
}

// Routes 는 Manifest 의 routes[].path 를 기준으로 한 **상대** 경로를 돌려준다.
// Core 가 /api/extensions/hello 아래에 마운트하므로 다른 확장의 경로를 침범할 수 없다.
func (e *Extension) Routes() []extv1.RouteHandler {
	return []extv1.RouteHandler{{
		Method:     http.MethodGet,
		Pattern:    "/clusters",
		Permission: PermView,
		Handler:    http.HandlerFunc(e.handleClusters),
	}}
}

// handleClusters 는 클러스터 **개수만** 돌려준다.
// 접속정보(브로커 주소·SASL 계정)는 ClusterInfo 에 필드 자체가 없다 — 흘릴 수가 없다.
func (e *Extension) handleClusters(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	clusters := e.clusters
	e.mu.RUnlock()
	if clusters == nil {
		http.Error(w, `{"error":"확장이 시작되지 않았습니다"}`, http.StatusServiceUnavailable)
		return
	}
	list, err := clusters.List(r.Context())
	if err != nil {
		http.Error(w, `{"error":"클러스터 목록 조회에 실패했습니다"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": len(list)})
}
