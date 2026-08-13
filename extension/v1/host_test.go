package extensionv1

// HostContext 안전 접근자(미선언 capability = nil 주입) 테스트.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHostContext_미선언_capability는_한국어오류(t *testing.T) {
	h := HostContext{ExtensionID: "sample"} // 아무 capability 도 주입되지 않은 상태

	checks := []struct {
		name string
		call func() error
	}{
		{"kafka.read", func() error { _, err := h.RequireKafka(); return err }},
		{"cluster.read", func() error { _, err := h.RequireClusters(); return err }},
		{"workflow.submit", func() error { _, err := h.RequireWorkflow(); return err }},
		{"audit.write", func() error { _, err := h.RequireAudit(); return err }},
		{"config.read", func() error { _, err := h.RequireConfig(); return err }},
		{"secret.ref", func() error { _, err := h.RequireSecrets(); return err }},
	}
	for _, c := range checks {
		err := c.call()
		if err == nil {
			t.Errorf("%s: nil 서비스인데 오류가 없다", c.name)
			continue
		}
		if !errors.Is(err, ErrCapabilityUnavailable) {
			t.Errorf("%s: ErrCapabilityUnavailable 로 감싸지 않았다: %v", c.name, err)
		}
		if !strings.Contains(err.Error(), c.name) || !strings.Contains(err.Error(), "sample") {
			t.Errorf("%s: 오류 메시지에 capability/Extension ID 가 없다: %v", c.name, err)
		}
	}
}

func TestHostContext_주입되면_그대로_반환(t *testing.T) {
	svc := fakeKafka{}
	h := HostContext{ExtensionID: "sample", Kafka: svc}
	got, err := h.RequireKafka()
	if err != nil {
		t.Fatalf("주입된 서비스에 오류가 났다: %v", err)
	}
	if got == nil {
		t.Fatal("주입된 서비스를 돌려주지 않았다")
	}
}

func TestHostContext_Identity(t *testing.T) {
	h := HostContext{ExtensionID: "sample"}
	if _, err := h.RequireIdentity(context.Background()); err == nil {
		t.Error("Identity 미주입인데 오류가 없다")
	}

	h.Identity = func(context.Context) (Identity, bool) { return Identity{}, false }
	if _, err := h.RequireIdentity(context.Background()); err == nil {
		t.Error("인증 정보 없음(ok=false)인데 오류가 없다")
	}

	want := Identity{UserID: "operator01", Permissions: []string{"ext.sample.view"}}
	h.Identity = func(context.Context) (Identity, bool) { return want, true }
	got, err := h.RequireIdentity(context.Background())
	if err != nil {
		t.Fatalf("Identity 조회 실패: %v", err)
	}
	if got.UserID != want.UserID {
		t.Errorf("Identity 불일치: %+v", got)
	}
	if !got.HasPermission("ext.sample.view") || got.HasPermission("ext.sample.manage") || got.HasPermission("") {
		t.Error("HasPermission 판정 오류")
	}
}

func TestHostContext_Logger_nil안전(t *testing.T) {
	h := HostContext{ExtensionID: "sample"}
	// Log 가 nil 이어도 패닉하지 않아야 한다.
	h.Logger().Info("메시지", "key", "value")
	h.Logger().Warn("경고")
	h.Logger().Error("오류")
}

// fakeKafka 는 읽기 전용 계약을 만족하는 테스트용 구현이다.
type fakeKafka struct{}

func (fakeKafka) ListTopics(context.Context, string) ([]TopicInfo, error) { return nil, nil }
func (fakeKafka) DescribeTopic(context.Context, string, string) (TopicDetail, error) {
	return TopicDetail{}, nil
}
func (fakeKafka) ListACLs(context.Context, string, ACLFilter) ([]ACLEntry, error) { return nil, nil }
func (fakeKafka) ListConsumerGroups(context.Context, string) ([]ConsumerGroupInfo, error) {
	return nil, nil
}

// 컴파일 타임 계약 확인 — 인터페이스 시그니처가 바뀌면 여기서 먼저 깨진다.
var _ KafkaReadService = fakeKafka{}
