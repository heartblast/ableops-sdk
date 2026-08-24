package testkit_test

// testkit 자체의 계약 검증 — 특히 **관대해지지 않았는지**.
//
// testkit 이 주지 않은 capability 를 채워 주면 "테스트는 통과하는데 설치하면 죽는" 확장이
// 만들어진다. 그 회귀를 여기서 막는다.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
	"github.com/heartblast/kafka-control-portal/sdk/extension/v1/testkit"
)

// TestNewHost_미주입capability는nil 은 최소권한 재현을 고정한다.
func TestNewHost_미주입capability는nil(t *testing.T) {
	h := testkit.NewHost("demo", testkit.WithKafka(testkit.NewFakeKafka()))
	ctx := h.Context()

	if _, err := ctx.RequireKafka(); err != nil {
		t.Fatalf("주입한 capability 를 얻지 못했다: %v", err)
	}
	for name, get := range map[string]func() error{
		"cluster.read":    func() error { _, err := ctx.RequireClusters(); return err },
		"workflow.submit": func() error { _, err := ctx.RequireWorkflow(); return err },
		"audit.write":     func() error { _, err := ctx.RequireAudit(); return err },
		"config.read":     func() error { _, err := ctx.RequireConfig(); return err },
		"config.write":    func() error { _, err := ctx.RequireConfigWrite(); return err },
		"secret.ref":      func() error { _, err := ctx.RequireSecrets(); return err },
	} {
		err := get()
		if err == nil {
			t.Errorf("%s: 주입하지 않았는데 사용 가능하다(testkit 이 관대해졌다)", name)
			continue
		}
		if extv1.CodeOf(err) != extv1.CodeCapabilityUnavailable {
			t.Errorf("%s: 오류 코드가 다르다: %q", name, extv1.CodeOf(err))
		}
	}
}

// TestWithConfig_조회전용 은 config.read 와 config.write 가 실제로 갈리는지 고정한다.
func TestWithConfig_조회전용(t *testing.T) {
	ro := testkit.NewHost("demo", testkit.WithConfig(map[string]any{"greeting": "안녕하세요"})).Context()
	if _, err := ro.RequireConfig(); err != nil {
		t.Fatalf("config.read 를 얻지 못했다: %v", err)
	}
	if _, err := ro.RequireConfigWrite(); err == nil {
		t.Error("조회 전용인데 config.write 가 주어졌다")
	}
	cfg, err := ro.RequireConfig()
	if err != nil {
		t.Fatalf("조회 서비스 획득 실패: %v", err)
	}
	if err := cfg.Set(context.Background(), map[string]any{"greeting": "안녕"}); err == nil {
		t.Error("조회 전용인데 저장이 성공했다")
	} else if extv1.CodeOf(err) != extv1.CodePermissionDenied {
		t.Errorf("오류 코드가 다르다: %q", extv1.CodeOf(err))
	}

	rw := testkit.NewHost("demo", testkit.WithWritableConfig(nil)).Context()
	w, err := rw.RequireConfigWrite()
	if err != nil {
		t.Fatalf("config.write 를 얻지 못했다: %v", err)
	}
	if err := w.Set(context.Background(), map[string]any{"a": 1}); err != nil {
		t.Fatalf("쓰기 가능 설정에서 저장이 실패했다: %v", err)
	}
}

// TestFakeWorkflow_신청이력 은 확장이 실제로 신청을 냈는지 검증할 수 있음을 보인다.
func TestFakeWorkflow_신청이력(t *testing.T) {
	h := testkit.NewHost("demo",
		testkit.WithWorkflow(testkit.NewFakeWorkflow()),
		testkit.WithIdentity(extv1.Identity{UserID: "owner01"}),
	)
	wf, err := h.Context().RequireWorkflow()
	if err != nil {
		t.Fatalf("workflow.submit 을 얻지 못했다: %v", err)
	}
	ref, err := wf.SubmitTopicCreate(context.Background(), extv1.TopicCreateInput{
		ClusterID: "prod-01", Name: "raw.web.dig.order.prod", Partitions: 3, ReplicationFactor: 3,
	})
	if err != nil || ref.ID == "" {
		t.Fatalf("신청이 실패했다: %v %+v", err, ref)
	}
	if len(h.Workflow.Submitted) != 1 || h.Workflow.Submitted[0].Kind != "topic.create" {
		t.Fatalf("신청 이력이 기록되지 않았다: %+v", h.Workflow.Submitted)
	}
	id, err := h.Context().RequireIdentity(context.Background())
	if err != nil || id.UserID != "owner01" {
		t.Fatalf("신원 주입이 동작하지 않는다: %v %+v", err, id)
	}
}

// TestCheckExtension_계약위반탐지 는 계약 헬퍼가 실제 위반을 잡는지 확인한다.
func TestCheckExtension_계약위반탐지(t *testing.T) {
	host := testkit.NewHost("bad-ext").Context()

	issues := testkit.CheckExtension(&badExtension{}, host)
	joined := strings.Join(issues, "\n")
	if !strings.Contains(joined, "예약 서브경로") {
		t.Errorf("예약 서브경로 위반을 잡지 못했다:\n%s", joined)
	}
	if !strings.Contains(joined, "멱등") {
		t.Errorf("Stop 멱등 위반을 잡지 못했다:\n%s", joined)
	}

	// 정상 확장에는 지적이 없어야 한다(거짓 양성 방지).
	if issues := testkit.CheckExtension(&goodExtension{}, host); len(issues) != 0 {
		t.Errorf("정상 확장에 지적이 나왔다:\n%s", strings.Join(issues, "\n"))
	}
}

// badExtension 은 계약을 여러 군데 어기는 확장이다.
type badExtension struct{ stopped bool }

func (e *badExtension) Manifest() extv1.Manifest {
	return extv1.Manifest{
		APIVersion: extv1.APIVersion, ID: "bad-ext", Name: "나쁜 확장", Version: "1.0.0",
		Backend: extv1.BackendDecl{Enabled: true},
		Routes:  []extv1.RouteDecl{{Path: "/api/extensions/bad-ext/health"}}, // 예약 서브경로
	}
}
func (e *badExtension) Start(context.Context, extv1.HostContext) error { return nil }
func (e *badExtension) Stop(context.Context) error {
	if e.stopped {
		return extv1.NewError(extv1.CodeInternal, "이미 정지했습니다")
	}
	e.stopped = true
	return nil
}
func (e *badExtension) Health(context.Context) extv1.Health {
	return extv1.Health{OK: true, CheckedAt: time.Now()}
}
func (e *badExtension) Routes() []extv1.RouteHandler { return nil }

// goodExtension 은 계약을 지키는 확장이다.
type goodExtension struct{}

func (goodExtension) Manifest() extv1.Manifest {
	return extv1.Manifest{
		APIVersion: extv1.APIVersion, ID: "good-ext", Name: "좋은 확장", Version: "1.0.0",
		Backend: extv1.BackendDecl{Enabled: true},
		Routes:  []extv1.RouteDecl{{Path: "/api/extensions/good-ext/ping"}},
	}
}
func (goodExtension) Start(context.Context, extv1.HostContext) error { return nil }
func (goodExtension) Stop(context.Context) error                     { return nil }
func (goodExtension) Health(context.Context) extv1.Health {
	return extv1.Health{OK: true, Message: "정상", CheckedAt: time.Now()}
}
func (goodExtension) Routes() []extv1.RouteHandler {
	return []extv1.RouteHandler{{Method: http.MethodGet, Pattern: "/ping", Handler: http.NotFoundHandler()}}
}
