package testkit

// capability 서비스 6종의 가짜 구현.
//
// 전부 **메모리에만** 동작하며 Kafka·DB·HTTP 를 건드리지 않는다. 호출 기록을 남겨 확장이
// "무엇을 요청했는가"를 검증할 수 있게 한다 — 반환값만 흉내 내면 "신청서를 냈다고 주장하지만
// 실제로는 아무것도 하지 않는" 확장을 잡아내지 못한다.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	extv1 "github.com/heartblast/kafka-control-portal/sdk/extension/v1"
)

// --- kafka.read ---

// FakeKafka 는 KafkaReadService 의 가짜 구현이다(클러스터별 고정 응답).
//
// ⚠ 변경 메서드는 존재하지 않는다 — SDK 인터페이스에 없기 때문이다. 이 사실 자체가
// "Extension 은 Kafka 를 직접 바꿀 수 없다"는 계약의 증거다.
type FakeKafka struct {
	mu sync.Mutex

	topics   map[string][]extv1.TopicInfo
	details  map[string]extv1.TopicDetail
	acls     map[string][]extv1.ACLEntry
	groups   map[string][]extv1.ConsumerGroupInfo
	failWith error

	// Calls 는 호출 이력이다("ListTopics:prod-01" 형식).
	Calls []string
}

var _ extv1.KafkaReadService = (*FakeKafka)(nil)

// NewFakeKafka 는 빈 FakeKafka 를 만든다.
func NewFakeKafka() *FakeKafka {
	return &FakeKafka{
		topics:  map[string][]extv1.TopicInfo{},
		details: map[string]extv1.TopicDetail{},
		acls:    map[string][]extv1.ACLEntry{},
		groups:  map[string][]extv1.ConsumerGroupInfo{},
	}
}

// WithTopics 는 클러스터의 Topic 목록을 설정한다.
func (f *FakeKafka) WithTopics(clusterID string, topics ...extv1.TopicInfo) *FakeKafka {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.topics[clusterID] = append([]extv1.TopicInfo(nil), topics...)
	return f
}

// WithTopicDetail 은 특정 Topic 의 상세를 설정한다.
func (f *FakeKafka) WithTopicDetail(clusterID, topic string, detail extv1.TopicDetail) *FakeKafka {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.details[clusterID+"/"+topic] = detail
	return f
}

// WithACLs 는 클러스터의 ACL 목록을 설정한다.
func (f *FakeKafka) WithACLs(clusterID string, acls ...extv1.ACLEntry) *FakeKafka {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acls[clusterID] = append([]extv1.ACLEntry(nil), acls...)
	return f
}

// WithConsumerGroups 는 클러스터의 Consumer Group 목록을 설정한다.
func (f *FakeKafka) WithConsumerGroups(clusterID string, groups ...extv1.ConsumerGroupInfo) *FakeKafka {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups[clusterID] = append([]extv1.ConsumerGroupInfo(nil), groups...)
	return f
}

// FailWith 는 모든 조회가 이 오류를 돌려주게 한다(장애 경로 테스트용).
func (f *FakeKafka) FailWith(err error) *FakeKafka {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failWith = err
	return f
}

func (f *FakeKafka) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, call)
	return f.failWith
}

// ListTopics 는 설정된 Topic 목록을 돌려준다.
func (f *FakeKafka) ListTopics(_ context.Context, clusterID string) ([]extv1.TopicInfo, error) {
	if err := f.record("ListTopics:" + clusterID); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.TopicInfo(nil), f.topics[clusterID]...), nil
}

// DescribeTopic 은 설정된 상세를 돌려준다(없으면 NOT_FOUND).
func (f *FakeKafka) DescribeTopic(_ context.Context, clusterID, topic string) (extv1.TopicDetail, error) {
	if err := f.record("DescribeTopic:" + clusterID + "/" + topic); err != nil {
		return extv1.TopicDetail{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.details[clusterID+"/"+topic]
	if !ok {
		return extv1.TopicDetail{}, extv1.Errorf(extv1.CodeNotFound, "Topic 을 찾을 수 없습니다: %s", topic)
	}
	return d, nil
}

// ListACLs 는 설정된 ACL 을 필터링해 돌려준다(Principal 만 걸러 낸다 — 최소 재현).
func (f *FakeKafka) ListACLs(_ context.Context, clusterID string, filter extv1.ACLFilter) ([]extv1.ACLEntry, error) {
	if err := f.record("ListACLs:" + clusterID); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]extv1.ACLEntry, 0, len(f.acls[clusterID]))
	for _, a := range f.acls[clusterID] {
		if filter.Principal != "" && a.Principal != filter.Principal {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// ListConsumerGroups 는 설정된 Consumer Group 을 돌려준다.
func (f *FakeKafka) ListConsumerGroups(_ context.Context, clusterID string) ([]extv1.ConsumerGroupInfo, error) {
	if err := f.record("ListConsumerGroups:" + clusterID); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.ConsumerGroupInfo(nil), f.groups[clusterID]...), nil
}

// --- cluster.read ---

// FakeClusters 는 ClusterRegistry 의 가짜 구현이다.
//
// ⚠ ClusterInfo 에는 브로커 주소·SASL 계정 필드가 **존재하지 않는다**. 가짜 구현이라도
// 접속정보를 담을 자리가 없다는 사실이 그대로 드러나야 한다(최소권한).
type FakeClusters struct {
	mu       sync.Mutex
	clusters []extv1.ClusterInfo
}

var _ extv1.ClusterRegistry = (*FakeClusters)(nil)

// NewFakeClusters 는 주어진 클러스터 목록으로 가짜 레지스트리를 만든다.
func NewFakeClusters(clusters ...extv1.ClusterInfo) *FakeClusters {
	return &FakeClusters{clusters: append([]extv1.ClusterInfo(nil), clusters...)}
}

// List 는 클러스터 목록을 돌려준다.
func (f *FakeClusters) List(context.Context) ([]extv1.ClusterInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.ClusterInfo(nil), f.clusters...), nil
}

// Get 은 ID 로 클러스터를 찾는다.
func (f *FakeClusters) Get(_ context.Context, id string) (extv1.ClusterInfo, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.clusters {
		if c.ID == id {
			return c, true
		}
	}
	return extv1.ClusterInfo{}, false
}

// --- workflow.submit ---

// SubmittedRequest 는 FakeWorkflow 가 기록한 신청 1건이다.
type SubmittedRequest struct {
	// Kind 는 신청 유형이다("topic.create" · "acl.grant" 등).
	Kind string
	// Input 은 확장이 넘긴 입력 DTO 원본이다(타입 단언해 검증한다).
	Input any
}

// FakeWorkflow 는 WorkflowSubmitService 의 가짜 구현이다.
//
// ⚠ Approve/Apply/Reject 는 **없다** — SDK 인터페이스에 존재하지 않기 때문이다.
// 확장은 신청까지만 할 수 있고 승인·반영은 Core 의 몫이라는 계약이 여기에도 그대로 드러난다.
type FakeWorkflow struct {
	mu sync.Mutex
	// Submitted 는 제출된 신청 이력이다.
	Submitted []SubmittedRequest
	// NextRef 는 다음 제출이 돌려줄 참조다(비우면 순번으로 자동 생성).
	NextRef extv1.RequestRef
	// FailWith 가 설정되면 모든 제출이 실패한다.
	FailWith error
}

var _ extv1.WorkflowSubmitService = (*FakeWorkflow)(nil)

// NewFakeWorkflow 는 빈 FakeWorkflow 를 만든다.
func NewFakeWorkflow() *FakeWorkflow { return &FakeWorkflow{} }

func (f *FakeWorkflow) submit(kind string, in any) (extv1.RequestRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailWith != nil {
		return extv1.RequestRef{}, f.FailWith
	}
	f.Submitted = append(f.Submitted, SubmittedRequest{Kind: kind, Input: in})
	if f.NextRef.ID != "" {
		return f.NextRef, nil
	}
	return extv1.RequestRef{ID: fmt.Sprintf("REQ-%03d", len(f.Submitted)), State: "SUBMITTED"}, nil
}

// SubmitTopicCreate 는 Topic 생성 신청을 기록한다.
func (f *FakeWorkflow) SubmitTopicCreate(_ context.Context, in extv1.TopicCreateInput) (extv1.RequestRef, error) {
	return f.submit("topic.create", in)
}

// SubmitTopicUpdate 는 Topic 변경 신청을 기록한다.
func (f *FakeWorkflow) SubmitTopicUpdate(_ context.Context, in extv1.TopicUpdateInput) (extv1.RequestRef, error) {
	return f.submit("topic.update", in)
}

// SubmitTopicDelete 는 Topic 삭제 신청을 기록한다.
func (f *FakeWorkflow) SubmitTopicDelete(_ context.Context, in extv1.TopicDeleteInput) (extv1.RequestRef, error) {
	return f.submit("topic.delete", in)
}

// SubmitACLGrant 는 ACL 부여 신청을 기록한다.
func (f *FakeWorkflow) SubmitACLGrant(_ context.Context, in extv1.ACLGrantInput) (extv1.RequestRef, error) {
	return f.submit("acl.grant", in)
}

// SubmitACLRevoke 는 ACL 회수 신청을 기록한다.
func (f *FakeWorkflow) SubmitACLRevoke(_ context.Context, in extv1.ACLRevokeInput) (extv1.RequestRef, error) {
	return f.submit("acl.revoke", in)
}

// --- audit.write ---

// FakeAudit 는 AuditService 의 가짜 구현이다(기록만 모은다).
type FakeAudit struct {
	mu sync.Mutex
	// Entries 는 기록된 감사 항목이다.
	Entries []extv1.AuditEntry
}

var _ extv1.AuditService = (*FakeAudit)(nil)

// NewFakeAudit 는 빈 FakeAudit 를 만든다.
func NewFakeAudit() *FakeAudit { return &FakeAudit{} }

// Record 는 감사 항목을 기록한다(SDK 계약대로 오류를 돌려주지 않는다).
func (f *FakeAudit) Record(_ context.Context, entry extv1.AuditEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Entries = append(f.Entries, entry)
}

// Actions 는 기록된 행위 이름 목록이다(검증 편의).
func (f *FakeAudit) Actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.Entries))
	for _, e := range f.Entries {
		out = append(out, e.Action)
	}
	return out
}

// --- config.read / config.write ---

// FakeConfig 는 ConfigService 의 가짜 구현이다(메모리 맵).
//
// 쓰기 권한은 NewHost 의 옵션이 결정한다: WithConfig 만 주면 조회 전용이고,
// WithWritableConfig 를 주면 저장까지 된다. 실제 Core 와 같은 판정을 재현하기 위한 것이다.
type FakeConfig struct {
	mu       sync.Mutex
	values   map[string]any
	writable bool
	// Writes 는 Set 호출 이력이다.
	Writes []map[string]any
}

var _ extv1.ConfigService = (*FakeConfig)(nil)

// NewFakeConfig 는 초기값을 가진 가짜 설정을 만든다(조회 전용).
func NewFakeConfig(values map[string]any) *FakeConfig {
	return &FakeConfig{values: copyValues(values)}
}

// Get 은 현재 설정을 돌려준다.
func (f *FakeConfig) Get(context.Context) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return copyValues(f.values), nil
}

// Set 은 설정을 저장한다(쓰기 권한이 없으면 PERMISSION_DENIED).
func (f *FakeConfig) Set(_ context.Context, values map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.writable {
		return extv1.Errorf(extv1.CodePermissionDenied,
			"설정 저장에는 %s capability 가 필요합니다", string(extv1.CapConfigWrite))
	}
	f.Writes = append(f.Writes, copyValues(values))
	for k, v := range values {
		f.values[k] = v
	}
	return nil
}

// copyValues 는 맵 사본을 만든다(가짜 구현이 호출자 맵을 공유하지 않게).
func copyValues(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// --- secret.ref ---

// FakeSecrets 는 SecretRefService 의 가짜 구현이다.
//
// ⚠ **평문을 돌려주는 메서드는 없다.** SDK 인터페이스가 그렇게 설계되어 있으며, 테스트
// 편의를 이유로도 추가하지 않는다 — 추가하는 순간 확장 코드가 평문을 다루는 습관이 생긴다.
type FakeSecrets struct {
	mu    sync.Mutex
	known map[string]bool
}

var _ extv1.SecretRefService = (*FakeSecrets)(nil)

// NewFakeSecrets 는 존재한다고 볼 참조 이름 목록으로 가짜 서비스를 만든다.
func NewFakeSecrets(known ...string) *FakeSecrets {
	m := make(map[string]bool, len(known))
	for _, k := range known {
		m[k] = true
	}
	return &FakeSecrets{known: m}
}

// Resolve 는 참조를 해석한다(존재하지 않으면 NOT_FOUND).
func (f *FakeSecrets) Resolve(_ context.Context, ref string) (extv1.SecretRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.TrimSpace(ref)
	if name == "" {
		return extv1.SecretRef{}, extv1.NewError(extv1.CodeInvalidArgument, "시크릿 참조가 비어 있습니다")
	}
	if !f.known[name] {
		return extv1.SecretRef{}, extv1.Errorf(extv1.CodeNotFound, "시크릿 참조를 찾을 수 없습니다: %s", name)
	}
	return extv1.SecretRef{Ref: name}, nil
}

// --- Logger ---

// RecordingLogger 는 로그를 메모리에 모으는 Logger 다.
//
// 시크릿이 로그로 새는지 검증할 때 쓴다(Lines 를 문자열로 훑는다).
type RecordingLogger struct {
	mu sync.Mutex
	// Lines 는 "LEVEL msg key=value …" 형식의 기록이다.
	Lines []string
}

var _ extv1.Logger = (*RecordingLogger)(nil)

// NewRecordingLogger 는 빈 로거를 만든다.
func NewRecordingLogger() *RecordingLogger { return &RecordingLogger{} }

func (l *RecordingLogger) write(level, msg string, args []any) {
	var b strings.Builder
	b.WriteString(level)
	b.WriteString(" ")
	b.WriteString(msg)
	for i := 0; i+1 < len(args); i += 2 {
		fmt.Fprintf(&b, " %v=%v", args[i], args[i+1])
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Lines = append(l.Lines, b.String())
}

// Info 는 정보 로그를 기록한다.
func (l *RecordingLogger) Info(msg string, args ...any) { l.write("INFO", msg, args) }

// Warn 은 경고 로그를 기록한다.
func (l *RecordingLogger) Warn(msg string, args ...any) { l.write("WARN", msg, args) }

// Error 는 오류 로그를 기록한다.
func (l *RecordingLogger) Error(msg string, args ...any) { l.write("ERROR", msg, args) }

// Contains 는 기록된 로그에 특정 문자열이 있는지 본다.
func (l *RecordingLogger) Contains(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.Lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

// nowFunc 는 Health 헬퍼가 쓰는 시각 함수다(테스트에서 고정할 수 있게 변수로 둔다).
var nowFunc = time.Now
