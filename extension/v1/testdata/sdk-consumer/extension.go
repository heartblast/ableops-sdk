// Package consumer 는 저장소 밖 개발자가 작성했다고 가정한 최소 Extension 이다.
//
// SDK 하나만 import 해서 built-in·외부 프로세스 양쪽 계약을 모두 구현할 수 있는지 확인한다.
package consumer

import (
	"context"
	"net/http"
	"time"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
	"github.com/heartblast/kafka-control-portal/sdk/extension/v1/extserver"
	"github.com/heartblast/kafka-control-portal/sdk/extension/v1/testkit"
)

// Extension 은 SDK 계약만으로 구현한 예제 확장이다.
type Extension struct {
	host extv1.HostContext
}

// 컴파일 시점에 계약 준수를 강제한다.
var _ extv1.Extension = (*Extension)(nil)

// New 는 확장 인스턴스를 만든다.
func New() *Extension { return &Extension{} }

// Manifest 는 확장 선언을 돌려준다.
func (e *Extension) Manifest() extv1.Manifest {
	return extv1.Manifest{
		APIVersion:   extv1.APIVersion,
		ID:           "consumer-demo",
		Name:         "외부 Consumer 데모",
		Version:      "1.0.0",
		Capabilities: []extv1.Capability{extv1.CapKafkaRead, extv1.CapConfigRead, extv1.CapConfigWrite},
		Permissions: []extv1.PermissionDecl{
			{Key: extv1.PermissionKey("consumer-demo", "view"), Label: "조회", Roles: []string{"SystemAdmin"}},
		},
		Backend: extv1.BackendDecl{Enabled: true, Kind: extv1.BackendKindProcess},
		Routes: []extv1.RouteDecl{
			{Path: "/api/extensions/consumer-demo/ping", Permission: extv1.PermissionKey("consumer-demo", "view")},
		},
	}
}

// Start 는 HostContext 를 보관한다.
func (e *Extension) Start(_ context.Context, host extv1.HostContext) error {
	e.host = host
	return nil
}

// Stop 은 정리한다(멱등).
func (e *Extension) Stop(context.Context) error { return nil }

// Health 는 건강 상태를 돌려준다.
func (e *Extension) Health(context.Context) extv1.Health {
	return extv1.Health{OK: true, Message: "정상", CheckedAt: time.Now()}
}

// Routes 는 HTTP 핸들러를 돌려준다.
func (e *Extension) Routes() []extv1.RouteHandler {
	return []extv1.RouteHandler{{
		Method:     http.MethodGet,
		Pattern:    "/ping",
		Permission: extv1.PermissionKey("consumer-demo", "view"),
		Handler:    http.HandlerFunc(e.ping),
	}}
}

// ping 은 capability 접근 경로를 전부 밟아 본다(Require* · Resolver · Error Contract).
func (e *Extension) ping(w http.ResponseWriter, r *http.Request) {
	if _, err := e.host.RequireKafka(); err != nil {
		http.Error(w, err.Error(), extv1.HTTPStatusForCode(extv1.CodeOf(err)))
		return
	}
	if _, err := extv1.RequireService[extv1.ConfigWriteService](e.host, extv1.CapConfigWrite); err != nil {
		http.Error(w, err.Error(), extv1.HTTPStatusForCode(extv1.CodeOf(err)))
		return
	}
	if id := extserver.RequestIDFrom(r.Context()); id != "" {
		w.Header().Set("X-Demo-Request-Id", id)
	}
	w.WriteHeader(http.StatusOK)
}

// SelfCheck 는 testkit 만으로 계약을 점검할 수 있는지 확인한다(외부 개발자의 테스트 경로).
func SelfCheck() []string {
	host := testkit.NewHost("consumer-demo",
		testkit.WithKafka(testkit.NewFakeKafka()),
		testkit.WithWritableConfig(map[string]any{"greeting": "안녕하세요"}),
		testkit.WithIdentity(extv1.Identity{UserID: "admin01", Roles: []string{"SystemAdmin"}}),
	)
	return testkit.CheckExtension(New(), host.Context())
}
