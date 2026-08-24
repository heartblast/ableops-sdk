package extensionv1

// ServiceResolver — capability 가 늘어도 HostContext 구조체가 함께 커지지 않게 하는 확장 지점.
//
// # 왜 필요한가
//
// v1 초기 설계는 capability 하나당 HostContext 필드 하나였다(Kafka·Clusters·Workflow…).
// 그 구조에서는 capability 를 추가할 때마다 **공개 구조체가 커진다**. 공개 구조체에 필드를
// 추가하면 외부 코드의 unkeyed composite literal(`HostContext{a, b, c}`)이 깨지고,
// 필드가 20개가 되는 시점에는 "이 확장이 실제로 무엇을 쓰는가"를 읽어 낼 수도 없다.
//
// 그래서 **조회 계층**을 하나 둔다. 신규 capability 는 필드를 늘리지 않고 Resolver 로만 제공한다.
// 기존 필드는 하위호환을 위해 그대로 유지한다(제거하지 않는다).
//
//	svc, err := extensionv1.RequireService[extensionv1.ConfigWriteService](host, extensionv1.CapConfigWrite)
//
// # 구현은 Core 가 한다
//
// Resolver 구현은 Core(internal/extensionhost)와 외부 프로세스 런타임(extserver)이 각각 제공한다.
// Extension 은 Core 구현 타입을 알 필요가 없고 알 방법도 없다 — Lookup 이 돌려주는 값은 항상
// **이 패키지가 선언한 인터페이스**로만 타입 단언된다.
//
// # typed-nil 을 절대 흘리지 않는다
//
// Go 에서 nil 포인터를 인터페이스에 담으면 인터페이스 자체는 non-nil 이 된다. Resolver 가
// (*someAdapter)(nil) 을 (any, true) 로 돌려주면 `RequireService` 는 성공을 반환하고 확장은
// 실제 호출에서 nil 역참조로 죽는다 — 최소권한 검사가 통과해 버리는 것이다.
// 그래서 조회 결과는 반드시 isNilService 로 한 번 더 거른다.

import "reflect"

// ServiceResolver 는 capability 이름으로 Host 서비스를 찾아 주는 계약이다.
//
// 반환 타입이 any 인 이유: capability 마다 서비스 타입이 다르고, Go 의 메서드는 타입 파라미터를
// 가질 수 없다. 타입 안전성은 이 인터페이스가 아니라 RequireService[T] 가 책임진다.
//
// 구현 규칙:
//   - 제공하지 않는 capability 는 (nil, false) 를 돌려준다. 빈 구조체·typed-nil 금지.
//   - 같은 capability 에는 항상 같은 의미의 서비스를 돌려준다(호출마다 달라지지 않는다).
//   - Lookup 은 부작용이 없어야 한다(조회일 뿐 활성화가 아니다).
type ServiceResolver interface {
	Lookup(capability Capability) (any, bool)
}

// ServiceResolverFunc 는 함수를 ServiceResolver 로 쓰기 위한 어댑터다.
type ServiceResolverFunc func(capability Capability) (any, bool)

// Lookup 은 ServiceResolver 를 구현한다.
func (f ServiceResolverFunc) Lookup(capability Capability) (any, bool) {
	if f == nil {
		return nil, false
	}
	return f(capability)
}

// ServiceMap 은 map 기반 ServiceResolver 다(테스트·간단한 Host 구현용).
//
// nil 값이 들어 있어도 Lookup 이 걸러 낸다.
type ServiceMap map[Capability]any

// Lookup 은 ServiceResolver 를 구현한다.
func (m ServiceMap) Lookup(capability Capability) (any, bool) {
	v, ok := m[capability]
	if !ok || isNilService(v) {
		return nil, false
	}
	return v, true
}

// isNilService 는 값이 실질적으로 nil 인지 판정한다(typed-nil 포함).
//
// reflect 를 쓰는 이유는 typed-nil 이 == nil 비교로는 잡히지 않기 때문이다.
// 조회 경로에서만 호출되므로 비용은 문제가 되지 않는다.
func isNilService(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return rv.IsNil()
	}
	return false
}

// LookupService 는 capability 에 해당하는 Host 서비스를 찾는다(타입 단언 없이).
//
// 조회 순서:
//  1. HostContext.Services(Resolver) — Core 가 제공하는 정식 경로
//  2. v1 초기 필드(Kafka·Clusters·…) — Resolver 를 넘기지 않는 구현·테스트를 위한 폴백
//
// 어느 쪽이든 typed-nil 은 걸러 낸다.
func (h HostContext) LookupService(capability Capability) (any, bool) {
	if h.Services != nil {
		if v, ok := h.Services.Lookup(capability); ok && !isNilService(v) {
			return v, true
		}
	}
	if v, ok := h.legacyService(capability); ok && !isNilService(v) {
		return v, true
	}
	return nil, false
}

// legacyService 는 v1 초기 HostContext 필드를 capability 로 조회한다.
//
// ⚠ 여기에 **신규 capability 를 추가하지 않는다**. 새 기능은 Resolver 로만 제공한다 —
// 그렇지 않으면 이 파일을 만든 이유(구조체 팽창 방지)가 사라진다.
func (h HostContext) legacyService(capability Capability) (any, bool) {
	switch capability {
	case CapKafkaRead:
		return h.Kafka, h.Kafka != nil
	case CapClusterRead:
		return h.Clusters, h.Clusters != nil
	case CapWorkflowSubmit:
		return h.Workflow, h.Workflow != nil
	case CapAuditWrite:
		return h.Audit, h.Audit != nil
	case CapConfigRead:
		return h.Config, h.Config != nil
	case CapSecretRef:
		return h.Secrets, h.Secrets != nil
	}
	return nil, false
}

// HasService 는 해당 capability 의 서비스를 실제로 사용할 수 있는지 판정한다.
//
// Manifest 선언 여부가 아니라 **주입 여부**를 본다 — 선언했더라도 Core 가 그 기능을 구성하지
// 않았으면(예: 외부 프로세스에서 secret.ref) 사용할 수 없다.
func (h HostContext) HasService(capability Capability) bool {
	_, ok := h.LookupService(capability)
	return ok
}

// RequireService 는 capability 에 해당하는 Host 서비스를 **타입 안전하게** 돌려준다.
//
//	cfg, err := extensionv1.RequireService[extensionv1.ConfigWriteService](host, extensionv1.CapConfigWrite)
//	if err != nil {
//	    return err // [CAPABILITY_UNAVAILABLE] …
//	}
//
// 실패는 두 가지이며 둘 다 CAPABILITY_UNAVAILABLE 코드를 쓴다:
//   - 서비스가 없다(미선언·미제공)
//   - 서비스는 있으나 요청한 타입이 아니다(Core 와 SDK 버전이 어긋난 상황)
//
// 두 번째를 별도 코드로 나누지 않는 이유: 확장이 취할 조치가 같다(그 기능을 쓰지 않는다).
// 대신 메시지에 실제 타입을 남겨 원인을 추적할 수 있게 한다.
func RequireService[T any](h HostContext, capability Capability) (T, error) {
	var zero T
	v, ok := h.LookupService(capability)
	if !ok {
		return zero, h.capabilityError(capability)
	}
	typed, ok := v.(T)
	if !ok {
		return zero, Errorf(CodeCapabilityUnavailable,
			"확장 기능 %s: %s capability 의 Host 서비스 타입이 예상과 다릅니다(Core 와 SDK 버전을 확인하세요, 실제 %T)",
			h.extensionIDForError(), string(capability), v)
	}
	// Resolver 가 걸렀더라도 인터페이스 → 인터페이스 단언 과정에서 typed-nil 이 살아남을 수 있다.
	if isNilService(typed) {
		return zero, h.capabilityError(capability)
	}
	return typed, nil
}
