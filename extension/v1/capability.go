package extensionv1

// Capability — Extension 이 Manifest 로 **명시 선언**해야만 얻는 최소권한 단위.
//
// Core 는 Manifest 가 선언하지 않은 capability 에 해당하는 HostContext 필드를 **nil 로 주입**한다.
// 즉 선언하지 않은 기능은 "권한 오류"가 아니라 **사용 자체가 불가능**하다(host.go 의 Require* 참조).

// Capability 는 Extension 이 요구하는 Host 기능 단위다.
type Capability string

const (
	// CapKafkaRead 는 Kafka 메타데이터 **조회만** 허용한다(KafkaReadService).
	// 허용: Topic 목록/상세, ACL 목록, Consumer Group 목록.
	// 불허: Topic/ACL/SCRAM 의 생성·수정·삭제, 파티션 증설, Consumer Group Offset 변경,
	//       메시지 본문 조회(PeekMessages). Kafka 변경은 Core Governance 경로로만 한다.
	CapKafkaRead Capability = "kafka.read"

	// CapClusterRead 는 클러스터 목록·메타데이터 조회를 허용한다(ClusterRegistry).
	// 허용: 클러스터 ID/이름/환경/Kafka 버전/활성 여부.
	// 불허: 브로커 주소·SASL 계정·비밀번호·TLS 인증서 경로 등 **접속정보 일체**(ClusterInfo 에 필드 자체가 없다).
	CapClusterRead Capability = "cluster.read"

	// CapWorkflowSubmit 은 Core 워크플로에 **신청서 제출만** 허용한다(WorkflowSubmitService).
	// 허용: Topic 생성/변경/삭제·ACL 부여/회수 신청 생성.
	// 불허: 승인(Approve)·반영(Apply)·반려(Reject). 신청자≠승인자 원칙과 고위험 승인 절차를 우회할 수 없다.
	CapWorkflowSubmit Capability = "workflow.submit"

	// CapAuditWrite 는 감사로그 기록을 허용한다(AuditService).
	// 허용: 자기 Extension 이 수행한 행위의 감사 기록 추가.
	// 불허: 감사로그 조회·수정·삭제(감사 기록은 추가 전용이다).
	CapAuditWrite Capability = "audit.write"

	// CapConfigRead 는 **자기 Extension 설정** 조회/저장을 허용한다(ConfigService).
	// 허용: Manifest 가 선언한 config 스키마 범위의 값 읽기·쓰기.
	// 불허: 다른 Extension 의 설정, Core 실행 설정(internal/config.Config) 접근.
	CapConfigRead Capability = "config.read"

	// CapSecretRef 는 시크릿 **참조(SecretRef)** 획득만 허용한다(SecretRefService).
	// 허용: 참조 문자열 해석·유효성 확인.
	// 불허: 평문 시크릿 값 획득. 어떤 메서드도 평문을 반환하지 않는다.
	CapSecretRef Capability = "secret.ref"
)

// AllCapabilities 는 v1 이 정의하는 capability 전체를 선언 순서대로 돌려준다.
func AllCapabilities() []Capability {
	return []Capability{
		CapKafkaRead,
		CapClusterRead,
		CapWorkflowSubmit,
		CapAuditWrite,
		CapConfigRead,
		CapSecretRef,
	}
}

// KnownCapability 는 v1 이 아는 capability 인지 판정한다.
// 모르는 값은 Manifest 검증에서 거부한다 — 오타를 "권한 없음"으로 조용히 넘기면
// Extension 이 런타임에 nil 역참조로 죽는다.
func KnownCapability(c Capability) bool {
	switch c {
	case CapKafkaRead, CapClusterRead, CapWorkflowSubmit, CapAuditWrite, CapConfigRead, CapSecretRef:
		return true
	}
	return false
}

// HasCapability 는 Manifest 가 해당 capability 를 선언했는지 확인한다.
func (m Manifest) HasCapability(c Capability) bool {
	for _, x := range m.Capabilities {
		if x == c {
			return true
		}
	}
	return false
}
