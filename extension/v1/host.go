package extensionv1

// HostContext — Core 가 Extension 에 주입하는 실행 컨텍스트(capability 주입 지점).

import (
	"context"
	"errors"
	"fmt"
)

// ErrCapabilityUnavailable 은 Manifest 가 선언하지 않아(또는 Core 가 제공하지 않아)
// 해당 capability 를 쓸 수 없을 때의 센티널 오류다. errors.Is 로 판별한다.
var ErrCapabilityUnavailable = errors.New("capability 미제공")

// Logger 는 Extension 이 사용하는 구조화 로깅 계약이다.
//
// 시그니처는 log/slog 와 호환되므로 Core 는 *slog.Logger 를 그대로 넘길 수 있다.
// 그럼에도 log/slog 를 import 하지 않고 인터페이스로 두는 이유는, SDK 가 특정 로깅 구현에
// 묶이지 않아야 하고 Extension 이 테스트에서 가짜 Logger 를 끼울 수 있어야 하기 때문이다.
// 로그 인자에 시크릿·메시지 본문을 담지 않는다.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// HostContext 는 Core 가 Extension 시작 시점에 주입하는 기능 묶음이다.
//
// ⚠ **Manifest 가 선언하지 않은 capability 에 해당하는 필드는 nil 로 주입된다.**
// 예를 들어 capabilities 에 kafka.read 가 없으면 Kafka 는 nil 이다.
// 필드를 직접 역참조하면 nil 포인터 패닉으로 Extension 이 죽고 원인도 알기 어렵다.
// 따라서 **항상 Require* 접근자를 통해 사용**한다 — 미선언 시 한국어 오류를 돌려준다.
//
//	svc, err := host.RequireKafka()
//	if err != nil { return err }   // "[CAPABILITY_UNAVAILABLE] 확장 기능 sample: kafka.read ..."
//
// ⚠ **직접 생성하지 않는다.** 운영 경로에서는 Core 가 만들어 Start 로 넘겨 준다.
// 테스트에서 필요하면 extension/v1/testkit 의 testkit.NewHost(...) 를 쓴다 —
// 이 구조체는 v1 이 사는 동안 필드가 **추가**될 수 있고, 그때 unkeyed composite literal
// (HostContext{a, b, c})은 컴파일이 깨진다. keyed literal 이나 testkit 은 깨지지 않는다.
type HostContext struct {
	// ExtensionID 는 이 컨텍스트를 받는 Extension 의 ID 다(설정·감사·로그 스코프의 기준).
	ExtensionID string
	// Identity 는 현재 요청 사용자를 돌려준다. 인증 컨텍스트가 없으면 ok=false.
	Identity func(ctx context.Context) (Identity, bool)

	Kafka    KafkaReadService      // capability: kafka.read
	Clusters ClusterRegistry       // capability: cluster.read
	Workflow WorkflowSubmitService // capability: workflow.submit
	Audit    AuditService          // capability: audit.write
	Config   ConfigService         // capability: config.read(Get) + config.write(Set)
	Secrets  SecretRefService      // capability: secret.ref

	// Services 는 **신규 capability 의 주입 지점**이다(services.go).
	//
	// 위 필드는 v1 초기 계약이라 그대로 유지하지만, 앞으로 추가되는 capability 는 필드를 늘리지
	// 않고 이 Resolver 로만 제공한다. 그래야 capability 가 늘어도 공개 구조체가 커지지 않는다.
	//
	//	w, err := extensionv1.RequireService[extensionv1.ConfigWriteService](host, extensionv1.CapConfigWrite)
	//
	// nil 이어도 된다 — 그 경우 조회는 위 필드로만 이뤄진다(테스트에서 흔한 형태다).
	Services ServiceResolver

	Log Logger
}

// extensionIDForError 는 오류 문구에 쓸 확장 ID 다(미주입 시 자리표시자).
func (h HostContext) extensionIDForError() string {
	if h.ExtensionID == "" {
		return "(알 수 없음)"
	}
	return h.ExtensionID
}

// capabilityError 는 미제공 capability 접근 시의 오류를 만든다.
//
// CAPABILITY_UNAVAILABLE 코드가 붙고, 하위호환을 위해 센티널 ErrCapabilityUnavailable 도
// 감싼다 — 기존 확장의 errors.Is(err, ErrCapabilityUnavailable) 가 계속 참이어야 한다.
func (h HostContext) capabilityError(c Capability) error {
	return WrapError(CodeCapabilityUnavailable,
		fmt.Sprintf("확장 기능 %s: %s capability 를 사용할 수 없습니다(Manifest capabilities 에 선언했는지 확인하세요)",
			h.extensionIDForError(), string(c)),
		ErrCapabilityUnavailable)
}

// RequireKafka 는 Kafka 조회 서비스를 돌려준다(kafka.read 미선언 시 오류).
func (h HostContext) RequireKafka() (KafkaReadService, error) {
	return RequireService[KafkaReadService](h, CapKafkaRead)
}

// RequireClusters 는 클러스터 레지스트리를 돌려준다(cluster.read 미선언 시 오류).
func (h HostContext) RequireClusters() (ClusterRegistry, error) {
	return RequireService[ClusterRegistry](h, CapClusterRead)
}

// RequireWorkflow 는 신청서 제출 서비스를 돌려준다(workflow.submit 미선언 시 오류).
func (h HostContext) RequireWorkflow() (WorkflowSubmitService, error) {
	return RequireService[WorkflowSubmitService](h, CapWorkflowSubmit)
}

// RequireAudit 는 감사 서비스를 돌려준다(audit.write 미선언 시 오류).
func (h HostContext) RequireAudit() (AuditService, error) {
	return RequireService[AuditService](h, CapAuditWrite)
}

// RequireConfig 는 설정 서비스를 돌려준다(config.read 미선언 시 오류).
func (h HostContext) RequireConfig() (ConfigService, error) {
	return RequireService[ConfigService](h, CapConfigRead)
}

// RequireConfigWrite 는 설정 **저장** 서비스를 돌려준다(config.write 미선언 시 오류).
//
// RequireConfig().Set(...) 도 같은 판정을 받지만, 이쪽은 **호출 전에** 권한을 확인할 수 있다.
func (h HostContext) RequireConfigWrite() (ConfigWriteService, error) {
	return RequireService[ConfigWriteService](h, CapConfigWrite)
}

// RequireSecrets 는 시크릿 참조 서비스를 돌려준다(secret.ref 미선언 시 오류).
func (h HostContext) RequireSecrets() (SecretRefService, error) {
	return RequireService[SecretRefService](h, CapSecretRef)
}

// RequireIdentity 는 현재 요청 사용자를 돌려준다.
// 인증 컨텍스트가 없으면(익명·백그라운드 작업) 오류다 — 사용자를 알 수 없는데 사용자 행위로
// 처리하면 감사 기록이 거짓이 된다.
func (h HostContext) RequireIdentity(ctx context.Context) (Identity, error) {
	if h.Identity == nil {
		return Identity{}, NewError(CodeHostUnavailable, "사용자 컨텍스트가 주입되지 않았습니다(Core 배선 오류)")
	}
	id, ok := h.Identity(ctx)
	if !ok {
		return Identity{}, NewError(CodePermissionDenied, "요청에 인증된 사용자 정보가 없습니다")
	}
	return id, nil
}

// Logger 는 안전한 로거를 돌려준다. Log 가 nil 이면 아무것도 하지 않는 로거를 돌려주므로
// Extension 은 nil 검사 없이 host.Logger().Info(...) 를 호출할 수 있다.
func (h HostContext) Logger() Logger {
	if h.Log == nil {
		return nopLogger{}
	}
	return h.Log
}

// nopLogger 는 아무것도 기록하지 않는 로거다(Log 미주입 시 nil 역참조 방지).
type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
