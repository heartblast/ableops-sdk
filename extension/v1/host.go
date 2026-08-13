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
//	if err != nil { return err }   // "확장 기능 sample: kafka.read capability 미선언 ..."
type HostContext struct {
	// ExtensionID 는 이 컨텍스트를 받는 Extension 의 ID 다(설정·감사·로그 스코프의 기준).
	ExtensionID string
	// Identity 는 현재 요청 사용자를 돌려준다. 인증 컨텍스트가 없으면 ok=false.
	Identity func(ctx context.Context) (Identity, bool)

	Kafka    KafkaReadService      // capability: kafka.read
	Clusters ClusterRegistry       // capability: cluster.read
	Workflow WorkflowSubmitService // capability: workflow.submit
	Audit    AuditService          // capability: audit.write
	Config   ConfigService         // capability: config.read
	Secrets  SecretRefService      // capability: secret.ref

	Log Logger
}

// capabilityError 는 미제공 capability 접근 시의 한국어 오류를 만든다.
func (h HostContext) capabilityError(c Capability) error {
	id := h.ExtensionID
	if id == "" {
		id = "(알 수 없음)"
	}
	return fmt.Errorf("확장 기능 %s: %s capability 를 사용할 수 없습니다(Manifest capabilities 에 선언했는지 확인하세요): %w",
		id, string(c), ErrCapabilityUnavailable)
}

// RequireKafka 는 Kafka 조회 서비스를 돌려준다(kafka.read 미선언 시 오류).
func (h HostContext) RequireKafka() (KafkaReadService, error) {
	if h.Kafka == nil {
		return nil, h.capabilityError(CapKafkaRead)
	}
	return h.Kafka, nil
}

// RequireClusters 는 클러스터 레지스트리를 돌려준다(cluster.read 미선언 시 오류).
func (h HostContext) RequireClusters() (ClusterRegistry, error) {
	if h.Clusters == nil {
		return nil, h.capabilityError(CapClusterRead)
	}
	return h.Clusters, nil
}

// RequireWorkflow 는 신청서 제출 서비스를 돌려준다(workflow.submit 미선언 시 오류).
func (h HostContext) RequireWorkflow() (WorkflowSubmitService, error) {
	if h.Workflow == nil {
		return nil, h.capabilityError(CapWorkflowSubmit)
	}
	return h.Workflow, nil
}

// RequireAudit 는 감사 서비스를 돌려준다(audit.write 미선언 시 오류).
func (h HostContext) RequireAudit() (AuditService, error) {
	if h.Audit == nil {
		return nil, h.capabilityError(CapAuditWrite)
	}
	return h.Audit, nil
}

// RequireConfig 는 설정 서비스를 돌려준다(config.read 미선언 시 오류).
func (h HostContext) RequireConfig() (ConfigService, error) {
	if h.Config == nil {
		return nil, h.capabilityError(CapConfigRead)
	}
	return h.Config, nil
}

// RequireSecrets 는 시크릿 참조 서비스를 돌려준다(secret.ref 미선언 시 오류).
func (h HostContext) RequireSecrets() (SecretRefService, error) {
	if h.Secrets == nil {
		return nil, h.capabilityError(CapSecretRef)
	}
	return h.Secrets, nil
}

// RequireIdentity 는 현재 요청 사용자를 돌려준다.
// 인증 컨텍스트가 없으면(익명·백그라운드 작업) 오류다 — 사용자를 알 수 없는데 사용자 행위로
// 처리하면 감사 기록이 거짓이 된다.
func (h HostContext) RequireIdentity(ctx context.Context) (Identity, error) {
	if h.Identity == nil {
		return Identity{}, errors.New("사용자 컨텍스트가 주입되지 않았습니다(Core 배선 오류)")
	}
	id, ok := h.Identity(ctx)
	if !ok {
		return Identity{}, errors.New("요청에 인증된 사용자 정보가 없습니다")
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
