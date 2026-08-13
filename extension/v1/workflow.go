package extensionv1

// WorkflowSubmitService — Extension 이 Core 워크플로에 **신청서를 제출**하는 유일한 통로
// (capability: workflow.submit).
//
// ⚠ 이 인터페이스에는 Approve / Apply / Reject 가 **존재하지 않으며 절대 추가하지 않는다**.
//
//   - 신청자 ≠ 승인자: Extension 이 자기가 낸 신청을 스스로 승인할 수 있으면 통제가 아니라 우회다.
//   - 고위험 신청의 정보보안 승인 필수 규칙도 같은 이유로 Extension 에 노출하지 않는다.
//   - 실제 Kafka 반영(Apply)은 Core 만 수행한다 — 정책 검증·감사 기록이 붙어 있는 경로이기 때문이다.
//
// Extension 이 할 수 있는 일은 "신청서를 만드는 것"까지이며, 그 뒤의 정책 검증·승인·반영·감사는
// 전부 Core 가 기존 화면과 동일한 절차로 처리한다.

import "context"

// RequestRef 는 생성된 신청서의 참조다.
// State 는 Core 워크플로 상태 문자열이다(예: "REQUESTED", "POLICY_CHECKED", "POLICY_REJECTED").
// Extension 은 이 값을 표시·기록용으로만 쓰고, 상태를 직접 바꾸려 시도하지 않는다.
type RequestRef struct {
	ID    string `json:"requestId"`
	State string `json:"state"`
}

// TopicCreateInput 은 Topic 생성 신청 입력이다.
type TopicCreateInput struct {
	ClusterID         string            `json:"clusterId"`
	Name              string            `json:"name"`
	Partitions        int               `json:"partitions"`
	ReplicationFactor int               `json:"replicationFactor"`
	RetentionMs       int64             `json:"retentionMs,omitempty"`
	CleanupPolicy     string            `json:"cleanupPolicy,omitempty"` // delete | compact
	Configs           map[string]string `json:"configs,omitempty"`
	Reason            string            `json:"reason,omitempty"` // 신청 사유(감사·승인 판단 근거)
}

// TopicUpdateInput 은 Topic 설정 변경 신청 입력이다.
// 포인터 필드는 nil = 변경하지 않음을 뜻한다(0 과 "미지정"을 구분하기 위함).
type TopicUpdateInput struct {
	ClusterID   string            `json:"clusterId"`
	Name        string            `json:"name"`
	Partitions  *int              `json:"partitions,omitempty"` // 증설만 가능(축소는 Core 가 거부)
	RetentionMs *int64            `json:"retentionMs,omitempty"`
	Configs     map[string]string `json:"configs,omitempty"`
	Reason      string            `json:"reason,omitempty"`
}

// TopicDeleteInput 은 Topic 삭제 신청 입력이다.
type TopicDeleteInput struct {
	ClusterID string `json:"clusterId"`
	Name      string `json:"name"`
	Reason    string `json:"reason,omitempty"`
}

// ACLGrantInput 은 ACL 부여 신청 입력이다.
type ACLGrantInput struct {
	ClusterID string     `json:"clusterId"`
	Entries   []ACLEntry `json:"entries"`
	Reason    string     `json:"reason,omitempty"`
}

// ACLRevokeInput 은 ACL 회수 신청 입력이다.
type ACLRevokeInput struct {
	ClusterID string     `json:"clusterId"`
	Entries   []ACLEntry `json:"entries"`
	Reason    string     `json:"reason,omitempty"`
}

// WorkflowSubmitService 는 신청서 제출 계약이다(제출만 — 승인·반영·반려 없음).
type WorkflowSubmitService interface {
	SubmitTopicCreate(ctx context.Context, in TopicCreateInput) (RequestRef, error)
	SubmitTopicUpdate(ctx context.Context, in TopicUpdateInput) (RequestRef, error)
	SubmitTopicDelete(ctx context.Context, in TopicDeleteInput) (RequestRef, error)
	SubmitACLGrant(ctx context.Context, in ACLGrantInput) (RequestRef, error)
	SubmitACLRevoke(ctx context.Context, in ACLRevokeInput) (RequestRef, error)
}
