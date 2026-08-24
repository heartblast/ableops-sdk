package extensionv1

// Capability — Extension 이 Manifest 로 **명시 선언**해야만 얻는 최소권한 단위.
//
// Core 는 Manifest 가 선언하지 않은 capability 에 해당하는 HostContext 필드를 **nil 로 주입**한다.
// 즉 선언하지 않은 기능은 "권한 오류"가 아니라 **사용 자체가 불가능**하다(host.go 의 Require* 참조).

// Capability 는 Extension 이 요구하는 Host 기능 단위다.
//
// # 이름 규칙 — 이름이 곧 권한의 상한이다
//
//	*.read    조회만 한다(상태를 바꾸지 않는다)
//	*.write   변경한다
//	*.submit  신청·요청만 한다(승인·반영은 Core 몫이다)
//	*.ref     참조만 얻는다(실제 값은 받지 못한다)
//
// 이 규칙을 어기면(read 인데 쓰기까지 되면) 설치 관리자가 Manifest 만 보고 위험도를 판단할 수
// 없게 된다. **기존 capability 의 의미를 조용히 넓히지 않는다** — 넓혀야 한다면 새 이름을 만든다.
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

	// CapConfigRead 는 **자기 Extension 설정 조회**를 허용한다(ConfigService.Get).
	// 허용: Manifest 가 선언한 config 스키마 범위의 값 읽기.
	// 불허: 값 저장(→ config.write), 다른 Extension 의 설정, Core 실행 설정(internal/config.Config) 접근.
	//
	// ⚠ v1.6.2 까지는 이 capability 하나로 저장(Set)까지 되었다. 이름이 read 인데 쓰기까지
	// 열려 있으면 설치 관리자가 Manifest 만 보고 위험도를 판단할 수 없다 — 그래서 분리했다.
	// 저장이 필요한 확장은 config.write 를 **추가로** 선언해야 한다(마이그레이션 가이드 참조).
	CapConfigRead Capability = "config.read"

	// CapConfigWrite 는 **자기 Extension 설정 저장**을 허용한다(ConfigService.Set).
	// 허용: Manifest 가 선언한 config 스키마 범위의 값 쓰기.
	// 불허: 스키마 밖의 키, 다른 Extension 의 설정, Core 실행 설정.
	//
	// 조회까지 함께 필요하면 config.read 도 선언한다(write 가 read 를 포함하지 않는다 —
	// 포함시키면 "쓰기만 허용" 이라는 선언이 표현 불가능해진다).
	CapConfigWrite Capability = "config.write"

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
		CapConfigWrite,
		CapSecretRef,
	}
}

// KnownCapability 는 v1 이 아는 capability 인지 판정한다.
// 모르는 값은 Manifest 검증에서 거부한다 — 오타를 "권한 없음"으로 조용히 넘기면
// Extension 이 런타임에 nil 역참조로 죽는다.
func KnownCapability(c Capability) bool {
	switch c {
	case CapKafkaRead, CapClusterRead, CapWorkflowSubmit, CapAuditWrite, CapConfigRead, CapConfigWrite, CapSecretRef:
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
