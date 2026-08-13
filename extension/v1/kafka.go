package extensionv1

// KafkaReadService — Extension 에 제공되는 **읽기 전용** Kafka 조회 계약(capability: kafka.read).
//
// ⚠ 이 인터페이스에는 다음 메서드가 **존재하지 않는다**:
//
//	CreateTopic · DeleteTopic · CreateACLs · DeleteACLs ·
//	UpsertScramCredential · DeleteScramCredential ·
//	AlterTopicConfigs · IncreasePartitions · CommitConsumerGroupOffsets ·
//	PeekMessages
//
// Kafka 변경은 **Core Governance 경로(Workflow → Policy → 승인 → Audit → Adapter)를 통해서만** 한다.
// Extension 이 변경을 원하면 WorkflowSubmitService 로 신청서를 제출하고 승인·반영은 Core 가 수행한다.
// 메서드를 "없앤" 것이 아니라 **처음부터 넣지 않는다** — 인터페이스에 없으면 우회할 방법도 없다.
// PeekMessages(메시지 본문 조회)도 제외한다: 토픽 본문에는 개인정보·거래정보가 그대로 들어 있어
// 조회 자체가 데이터 유출 경로가 된다.
//
// 모든 메서드는 clusterID 를 받는다. Extension 은 자신이 접근 가능한 클러스터를
// ClusterRegistry 로 확인하고, 실제 접근 허용 여부는 Core 가 클러스터 RBAC 으로 재검사한다.

import "context"

// TopicInfo 는 Topic 목록 항목이다.
type TopicInfo struct {
	Name              string `json:"name"`
	Partitions        int    `json:"partitions"`
	ReplicationFactor int    `json:"replicationFactor"`
	Internal          bool   `json:"internal,omitempty"` // __consumer_offsets 등 내부 토픽
}

// TopicPartitionInfo 는 파티션 1개의 복제 상태다.
type TopicPartitionInfo struct {
	Partition int32   `json:"partition"`
	Leader    int32   `json:"leader"` // -1 = 리더 없음
	Replicas  []int32 `json:"replicas,omitempty"`
	ISR       []int32 `json:"isr,omitempty"`
}

// TopicDetail 은 Topic 상세다(설정·파티션 복제 상태 포함).
type TopicDetail struct {
	Topic      TopicInfo            `json:"topic"`
	Configs    map[string]string    `json:"configs,omitempty"`
	Partitions []TopicPartitionInfo `json:"partitions,omitempty"`
}

// ACLEntry 는 Kafka ACL 1건이다.
//
// enum 표기는 포털 규약인 PascalCase 다("Read"/"Write"/"All", "Topic"/"Group", "Literal"/"Prefixed",
// "Allow"/"Deny"). 실 Kafka 의 대문자 enum 은 Core 어댑터 경계에서 정규화되어 전달된다 —
// Extension 이 대문자/소문자 표기를 다시 변환할 필요가 없고, 해서도 안 된다.
type ACLEntry struct {
	Principal      string `json:"principal"`      // 예: User:svc-order-prod
	ResourceType   string `json:"resourceType"`   // Topic | Group | Cluster | TransactionalId
	ResourceName   string `json:"resourceName"`   //
	PatternType    string `json:"patternType"`    // Literal | Prefixed
	Operation      string `json:"operation"`      // Read | Write | Describe | All ...
	PermissionType string `json:"permissionType"` // Allow | Deny
	Host           string `json:"host"`           // IP 또는 *
}

// ACLFilter 는 ACL 조회 필터다. 빈 필드는 "전체"를 의미한다(AND 결합).
type ACLFilter struct {
	Principal      string `json:"principal,omitempty"`
	ResourceType   string `json:"resourceType,omitempty"`
	ResourceName   string `json:"resourceName,omitempty"`
	PatternType    string `json:"patternType,omitempty"`
	Operation      string `json:"operation,omitempty"`
	PermissionType string `json:"permissionType,omitempty"`
	Host           string `json:"host,omitempty"`
}

// ConsumerGroupInfo 는 Consumer Group 요약이다.
type ConsumerGroupInfo struct {
	Name     string           `json:"name"`
	State    string           `json:"state"` // Stable | Empty | Rebalancing ...
	Members  int              `json:"members"`
	TotalLag int64            `json:"totalLag"`
	TopicLag map[string]int64 `json:"topicLag,omitempty"`
}

// KafkaReadService 는 Kafka 메타데이터 조회 계약이다(읽기 전용).
type KafkaReadService interface {
	// ListTopics 는 클러스터의 Topic 목록을 돌려준다.
	ListTopics(ctx context.Context, clusterID string) ([]TopicInfo, error)
	// DescribeTopic 은 Topic 1개의 상세를 돌려준다.
	DescribeTopic(ctx context.Context, clusterID, topic string) (TopicDetail, error)
	// ListACLs 는 필터에 맞는 ACL 목록을 돌려준다(filter 의 빈 필드는 전체).
	ListACLs(ctx context.Context, clusterID string, filter ACLFilter) ([]ACLEntry, error)
	// ListConsumerGroups 는 Consumer Group 요약 목록을 돌려준다.
	ListConsumerGroups(ctx context.Context, clusterID string) ([]ConsumerGroupInfo, error)
}
