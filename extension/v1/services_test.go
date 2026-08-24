package extensionv1

// ServiceResolver·RequireService 계약 테스트 — 특히 **typed-nil** 이 새지 않는지.

import (
	"context"
	"errors"
	"testing"
)

// nilKafka 는 typed-nil 을 만들기 위한 구현체다(메서드는 호출되지 않는다).
type nilKafka struct{}

func (*nilKafka) ListTopics(context.Context, string) ([]TopicInfo, error) { return nil, nil }
func (*nilKafka) DescribeTopic(context.Context, string, string) (TopicDetail, error) {
	return TopicDetail{}, nil
}
func (*nilKafka) ListACLs(context.Context, string, ACLFilter) ([]ACLEntry, error) { return nil, nil }
func (*nilKafka) ListConsumerGroups(context.Context, string) ([]ConsumerGroupInfo, error) {
	return nil, nil
}

// TestRequireService_Resolver경유 는 Resolver 로만 제공되는 capability 를 얻을 수 있는지 본다.
func TestRequireService_Resolver경유(t *testing.T) {
	writer := stubConfigWriter{}
	h := HostContext{
		ExtensionID: "sample",
		Services:    ServiceMap{CapConfigWrite: ConfigWriteService(writer)},
	}
	got, err := RequireService[ConfigWriteService](h, CapConfigWrite)
	if err != nil {
		t.Fatalf("Resolver 로 제공한 서비스를 얻지 못했다: %v", err)
	}
	if err := got.Set(context.Background(), map[string]any{"a": 1}); err != nil {
		t.Fatalf("서비스 호출이 실패했다: %v", err)
	}
	if !h.HasService(CapConfigWrite) {
		t.Error("HasService 가 false 다")
	}
}

// stubConfigWriter 는 최소 ConfigWriteService 구현이다.
type stubConfigWriter struct{}

func (stubConfigWriter) Set(context.Context, map[string]any) error { return nil }

// TestRequireService_미제공은CAPABILITY_UNAVAILABLE 는 미선언 capability 의 오류 코드를 고정한다.
func TestRequireService_미제공은CAPABILITY_UNAVAILABLE(t *testing.T) {
	h := HostContext{ExtensionID: "sample"}
	_, err := RequireService[ConfigWriteService](h, CapConfigWrite)
	if err == nil {
		t.Fatal("미제공 capability 가 성공했다")
	}
	if code := CodeOf(err); code != CodeCapabilityUnavailable {
		t.Errorf("오류 코드가 다르다: %q", code)
	}
	// 하위호환 — 기존 확장은 센티널로 판별한다.
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Error("ErrCapabilityUnavailable 센티널이 사라졌다(기존 확장의 errors.Is 가 깨진다)")
	}
}

// TestRequireService_typedNil은미제공으로본다 는 최소권한이 조용히 무력화되지 않음을 고정한다.
//
// ⚠ 이 테스트가 깨지면 "capability 를 선언하지 않았는데 nil 검사를 통과하고 실제 호출에서 죽는"
// 상태가 성립한다.
func TestRequireService_typedNil은미제공으로본다(t *testing.T) {
	var typedNil *nilKafka // nil 포인터
	for name, h := range map[string]HostContext{
		"필드로 주입":        {ExtensionID: "sample", Kafka: typedNil},
		"Resolver 로 주입": {ExtensionID: "sample", Services: ServiceMap{CapKafkaRead: KafkaReadService(typedNil)}},
	} {
		if h.HasService(CapKafkaRead) {
			t.Errorf("%s: typed-nil 이 사용 가능으로 판정됐다", name)
		}
		if _, err := h.RequireKafka(); err == nil {
			t.Errorf("%s: typed-nil 인데 RequireKafka 가 성공했다", name)
		} else if CodeOf(err) != CodeCapabilityUnavailable {
			t.Errorf("%s: 오류 코드가 다르다: %q", name, CodeOf(err))
		}
	}
}

// TestLookupService_필드폴백 은 Resolver 를 넘기지 않아도 기존 필드로 동작함을 고정한다
// (v1 하위호환 — 기존 확장의 테스트 코드가 HostContext 를 직접 만든다).
func TestLookupService_필드폴백(t *testing.T) {
	h := HostContext{ExtensionID: "sample", Kafka: &nilKafkaValue{}}
	if _, err := h.RequireKafka(); err != nil {
		t.Fatalf("필드로 주입한 서비스를 얻지 못했다: %v", err)
	}
	if _, err := h.RequireClusters(); err == nil {
		t.Error("주입하지 않은 capability 가 성공했다")
	}
}

// nilKafkaValue 는 non-nil 인 구현체다.
type nilKafkaValue struct{ nilKafka }

// TestRequireService_타입불일치 는 Core/SDK 버전이 어긋난 상황을 안전하게 처리하는지 본다.
func TestRequireService_타입불일치(t *testing.T) {
	h := HostContext{ExtensionID: "sample", Services: ServiceMap{CapKafkaRead: "문자열은 서비스가 아니다"}}
	_, err := RequireService[KafkaReadService](h, CapKafkaRead)
	if err == nil {
		t.Fatal("타입이 다른 서비스가 통과했다")
	}
	if CodeOf(err) != CodeCapabilityUnavailable {
		t.Errorf("오류 코드가 다르다: %q", CodeOf(err))
	}
}

// TestServiceMap_nil값은미등록 은 ServiceMap 이 nil 을 걸러 내는지 확인한다.
func TestServiceMap_nil값은미등록(t *testing.T) {
	m := ServiceMap{CapAuditWrite: nil}
	if _, ok := m.Lookup(CapAuditWrite); ok {
		t.Error("nil 값이 등록된 것으로 판정됐다")
	}
	var fn ServiceResolverFunc
	if _, ok := fn.Lookup(CapAuditWrite); ok {
		t.Error("nil ServiceResolverFunc 가 값을 돌려줬다")
	}
}
