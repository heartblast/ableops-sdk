package extensionv1

// Extension — built-in Extension 이 구현하는 계약.
//
// ⚠ 이 파일은 **net/http 만** 안다. chi 등 라우터 라이브러리를 import 하지 않는다:
// SDK 가 특정 라우터에 묶이면 외부 Extension 도 같은 라우터 버전을 강제받고,
// Core 가 라우터를 교체하는 순간 모든 Extension 이 깨진다.

import (
	"context"
	"net/http"
	"time"
)

// Health 는 Extension 이 보고하는 건강 상태다.
// 상태 전이(State) 판정은 Core 가 이 값 + 라우트/메뉴/권한 등록 결과를 종합해 수행한다 —
// Extension 이 자기 State 를 직접 정하지 않는다.
type Health struct {
	OK        bool      `json:"ok"`
	Message   string    `json:"message,omitempty"` // 한국어 상태 설명(비정상 사유 포함). 시크릿 금지.
	CheckedAt time.Time `json:"checkedAt"`
}

// RouteHandler 는 Extension 이 제공하는 HTTP 핸들러 1건이다.
//
// Pattern 은 Manifest 의 routes[].path 를 기준으로 한 **상대 경로**다(예: "/ping", "/reports/{id}").
// Core 가 `/api/extensions/<extensionID>` 아래에 마운트하므로, Extension 이 절대 경로나
// 다른 Extension 의 경로를 지정할 수 없다.
//
// Method 는 HTTP 메서드다(비우면 Core 가 GET 으로 본다).
// Permission 은 이 핸들러 호출에 필요한 권한 키이며(ext.<id>.<action>), Core 가 핸들러 진입 전
// 인가를 검사한다 — Extension 안에서 권한을 재구현하지 않는다.
type RouteHandler struct {
	Method     string
	Pattern    string
	Permission string
	Handler    http.Handler
}

// Extension 은 built-in(같은 프로세스) Extension 이 구현하는 계약이다.
//
// 생애주기: Core 가 Manifest 를 검증하고 Registry 에 등록한 뒤 Start 를 호출한다.
// Start 가 에러를 돌려주면 상태는 FAILED 가 되고 라우트는 등록되되 핸들러가 503 을 응답한다
// (조건부 라우트 등록은 프론트 404 를 만들기 때문에 하지 않는다).
//
// 구현 시 지킬 것:
//   - Start 는 오래 걸리는 작업을 블로킹하지 않는다(ctx 취소를 존중하고 백그라운드는 고루틴으로).
//   - Stop 은 여러 번 호출되어도 안전해야 한다(멱등).
//   - Health 는 빠르게 반환한다(외부 호출을 매번 하지 말고 캐시한다).
//   - Manifest 는 호출마다 동일한 값을 돌려준다(런타임에 권한·라우트를 바꿔 검증을 우회할 수 없다).
//   - 하나의 Extension 이 panic 하거나 FAILED 여도 Core 와 다른 Extension 은 정상 동작해야 한다.
type Extension interface {
	// Manifest 는 이 Extension 의 선언을 돌려준다(불변).
	Manifest() Manifest
	// Start 는 Extension 을 시작한다. host 의 capability 필드는 Manifest 선언분만 주입된다.
	Start(ctx context.Context, host HostContext) error
	// Stop 은 Extension 을 정지한다(멱등).
	Stop(ctx context.Context) error
	// Health 는 현재 건강 상태를 돌려준다.
	Health(ctx context.Context) Health
	// Routes 는 마운트할 HTTP 핸들러 목록을 돌려준다(Manifest routes[].path 기준 상대 경로).
	Routes() []RouteHandler
}
