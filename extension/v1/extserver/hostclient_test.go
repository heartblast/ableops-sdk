package extserver

// Host API 클라이언트(프로토콜 §4) 검증 — 가짜 Core 서버를 세워 실제 HTTP 로 호출한다.
//
// 고정하는 것:
//   - 경로·쿼리·Bearer 토큰이 프로토콜대로 나가는가
//   - 응답이 SDK DTO 로 해석되는가
//   - Core 의 한국어 오류 메시지와 상태코드가 보존되는가
//   - 재시도가 **멱등 GET 에만** 적용되는가(POST 재시도는 감사·신청 중복을 만든다)
//   - 미선언 capability 가 HostContext 에서 nil 인가(최소권한)

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// recordedRequest 는 가짜 Core 가 기록한 요청 1건이다.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   string
}

// fakeCore 는 Host API 를 흉내 내는 테스트 서버다.
type fakeCore struct {
	mu       sync.Mutex
	requests []recordedRequest
	// handlers 는 "METHOD /path" → 응답 함수다.
	handlers map[string]func(w http.ResponseWriter, r *http.Request)
	server   *httptest.Server
}

func newFakeCore(t *testing.T) *fakeCore {
	t.Helper()
	f := &fakeCore{handlers: map[string]func(http.ResponseWriter, *http.Request){}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Header: r.Header.Clone(), Body: string(body),
		})
		f.mu.Unlock()

		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		h := f.handlers[key]
		f.mu.Unlock()
		if h == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "확장 기능을 찾을 수 없습니다"})
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// on 은 경로 핸들러를 등록한다.
func (f *fakeCore) on(method, path string, h func(w http.ResponseWriter, r *http.Request)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method+" "+path] = h
}

// json 은 JSON 응답을 돌려주는 핸들러를 등록한다.
func (f *fakeCore) json(method, path string, status int, body any) {
	f.on(method, path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})
}

func (f *fakeCore) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

func (f *fakeCore) count(method, path string) int {
	n := 0
	for _, req := range f.recorded() {
		if req.Method == method && req.Path == path {
			n++
		}
	}
	return n
}

// newTestClient 는 가짜 Core 를 가리키는 클라이언트를 만든다.
func newTestClient(t *testing.T, f *fakeCore, caps ...extv1.Capability) (*hostClient, Environment) {
	t.Helper()
	env := Environment{
		ExtensionID:  "test-ext",
		Version:      "1.2.3",
		CallToken:    "call-token",
		HostURL:      f.server.URL + "/api/extensions/_host",
		HostToken:    "host-token",
		Capabilities: caps,
	}
	return newHostClient(env, newLogger(io.Discard, env.ExtensionID), 3*time.Second), env
}

// hostPath 는 Host API 절대 경로를 만든다.
func hostPath(sub string) string { return "/api/extensions/_host" + sub }

// TestHostClient_Ping 은 연결 확인 경로와 인증 헤더를 고정한다.
func TestHostClient_Ping(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/ping"), http.StatusOK, map[string]any{"ok": true, "extensionId": "test-ext"})

	c, _ := newTestClient(t, f)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping 이 실패했다: %v", err)
	}

	reqs := f.recorded()
	if len(reqs) != 1 {
		t.Fatalf("요청이 1건이어야 한다: %d", len(reqs))
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer host-token" {
		t.Errorf("Bearer 토큰이 다르다: %q", got)
	}
	if got := reqs[0].Header.Get(HeaderExtensionID); got != "test-ext" {
		t.Errorf("확장 ID 헤더가 다르다: %q", got)
	}
}

// TestHostClient_Ping_다른확장으로인식되면오류 는 토큰·확장 짝이 어긋난 배선을 잡아낸다.
func TestHostClient_Ping_다른확장으로인식되면오류(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/ping"), http.StatusOK, map[string]any{"ok": true, "extensionId": "other-ext"})

	c, _ := newTestClient(t, f)
	err := c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "other-ext") {
		t.Fatalf("확장 불일치를 알리지 않았다: %v", err)
	}
}

// TestKafkaService_조회경로 는 kafka.read 4종의 경로·쿼리를 고정한다.
func TestKafkaService_조회경로(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/kafka/topics"), http.StatusOK, []extv1.TopicInfo{
		{Name: "orders.v1", Partitions: 3, ReplicationFactor: 3},
	})
	f.json(http.MethodGet, hostPath("/kafka/topics/orders.v1"), http.StatusOK, extv1.TopicDetail{
		Topic:   extv1.TopicInfo{Name: "orders.v1", Partitions: 3},
		Configs: map[string]string{"retention.ms": "604800000"},
	})
	f.json(http.MethodGet, hostPath("/kafka/acls"), http.StatusOK, []extv1.ACLEntry{
		{Principal: "User:svc-order", Operation: "Write", ResourceType: "Topic", PermissionType: "Allow"},
	})
	f.json(http.MethodGet, hostPath("/kafka/consumer-groups"), http.StatusOK, []extv1.ConsumerGroupInfo{
		{Name: "order-consumer", State: "Stable", Members: 2, TotalLag: 10},
	})

	c, _ := newTestClient(t, f)
	svc := kafkaService{c: c}
	ctx := context.Background()

	topics, err := svc.ListTopics(ctx, "prod-01")
	if err != nil || len(topics) != 1 || topics[0].Name != "orders.v1" {
		t.Fatalf("ListTopics 결과가 다르다: %v %v", topics, err)
	}

	detail, err := svc.DescribeTopic(ctx, "prod-01", "orders.v1")
	if err != nil || detail.Topic.Name != "orders.v1" || detail.Configs["retention.ms"] != "604800000" {
		t.Fatalf("DescribeTopic 결과가 다르다: %+v %v", detail, err)
	}

	acls, err := svc.ListACLs(ctx, "prod-01", extv1.ACLFilter{Principal: "User:svc-order", Operation: "Write"})
	if err != nil || len(acls) != 1 || acls[0].Operation != "Write" {
		t.Fatalf("ListACLs 결과가 다르다: %v %v", acls, err)
	}

	groups, err := svc.ListConsumerGroups(ctx, "")
	if err != nil || len(groups) != 1 || groups[0].TotalLag != 10 {
		t.Fatalf("ListConsumerGroups 결과가 다르다: %v %v", groups, err)
	}

	reqs := f.recorded()
	if !strings.Contains(reqs[0].Query, "clusterId=prod-01") {
		t.Errorf("clusterId 쿼리가 없다: %q", reqs[0].Query)
	}
	if !strings.Contains(reqs[2].Query, "principal=User%3Asvc-order") || !strings.Contains(reqs[2].Query, "operation=Write") {
		t.Errorf("ACL 필터 쿼리가 다르다: %q", reqs[2].Query)
	}
	// clusterId 를 비우면 아예 보내지 않는다(Core 가 "미지정 = 기본 클러스터"로 해석한다).
	if strings.Contains(reqs[3].Query, "clusterId") {
		t.Errorf("빈 clusterId 를 보냈다: %q", reqs[3].Query)
	}
}

// TestKafkaService_오류메시지보존 은 Core 의 한국어 오류와 상태코드가 그대로 전달되는지 고정한다.
func TestKafkaService_오류메시지보존(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/kafka/topics"), http.StatusForbidden,
		map[string]string{"error": "kafka.read capability 가 선언되지 않았습니다"})

	c, _ := newTestClient(t, f)
	_, err := kafkaService{c: c}.ListTopics(context.Background(), "prod-01")
	if err == nil {
		t.Fatal("403 인데 성공했다")
	}
	if !strings.Contains(err.Error(), "kafka.read capability 가 선언되지 않았습니다") {
		t.Errorf("Core 오류 메시지가 보존되지 않았다: %v", err)
	}
	if got := HostStatusCode(err); got != http.StatusForbidden {
		t.Errorf("상태코드가 보존되지 않았다: %d", got)
	}
	// 403 은 재시도해도 결과가 같다 — 재시도하면 안 된다.
	if n := f.count(http.MethodGet, hostPath("/kafka/topics")); n != 1 {
		t.Errorf("403 을 재시도했다: %d회", n)
	}
}

// TestHostClient_GET만재시도 는 재시도 정책을 고정한다.
//
// POST 를 재시도하면 감사 기록·신청서가 중복 생성된다 — 조회 실패보다 훨씬 나쁘다.
func TestHostClient_GET만재시도(t *testing.T) {
	f := newFakeCore(t)
	var getCalls int
	f.on(http.MethodGet, hostPath("/kafka/topics"), func(w http.ResponseWriter, r *http.Request) {
		getCalls++
		if getCalls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "잠시 후 다시 시도하세요"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]extv1.TopicInfo{{Name: "orders.v1"}})
	})
	f.json(http.MethodPost, hostPath("/audit"), http.StatusServiceUnavailable, map[string]string{"error": "일시 오류"})

	c, _ := newTestClient(t, f)

	topics, err := kafkaService{c: c}.ListTopics(context.Background(), "")
	if err != nil {
		t.Fatalf("재시도 후에도 실패했다: %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("재시도 성공 결과가 다르다: %v", topics)
	}
	if getCalls != 3 {
		t.Errorf("GET 재시도 횟수가 다르다: %d", getCalls)
	}

	auditService{c: c}.Record(context.Background(), extv1.AuditEntry{Action: "test", Result: extv1.AuditResultSuccess})
	if n := f.count(http.MethodPost, hostPath("/audit")); n != 1 {
		t.Errorf("POST 를 재시도했다: %d회 (감사 기록이 중복된다)", n)
	}
}

// TestClusterService_조회 는 cluster.read 경로와 "오류를 ok=false 로 축약"하는 계약을 고정한다.
func TestClusterService_조회(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/clusters"), http.StatusOK, []extv1.ClusterInfo{
		{ID: "prod-01", Name: "운영", Environment: "prod", Active: true},
	})
	f.json(http.MethodGet, hostPath("/clusters/prod-01"), http.StatusOK,
		extv1.ClusterInfo{ID: "prod-01", Name: "운영", Environment: "prod", Active: true})

	c, _ := newTestClient(t, f)
	svc := clusterService{c: c}

	list, err := svc.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != "prod-01" {
		t.Fatalf("List 결과가 다르다: %v %v", list, err)
	}

	got, ok := svc.Get(context.Background(), "prod-01")
	if !ok || got.Name != "운영" {
		t.Fatalf("Get 결과가 다르다: %+v ok=%v", got, ok)
	}

	// 없는 클러스터는 404 → ok=false (오류를 돌려주지 않는 SDK 계약).
	if _, ok := svc.Get(context.Background(), "없는클러스터"); ok {
		t.Error("없는 클러스터가 존재로 판정되었다")
	}
	if _, ok := svc.Get(context.Background(), "  "); ok {
		t.Error("빈 ID 가 존재로 판정되었다")
	}
}

// TestConfigService_조회저장 은 config.read 경로와 저장 본문을 고정한다.
func TestConfigService_조회저장(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/config"), http.StatusOK, map[string]any{"greeting": "반갑습니다"})
	f.on(http.MethodPut, hostPath("/config"), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	c, _ := newTestClient(t, f)
	svc := &configService{c: c, canWrite: true}

	values, err := svc.Get(context.Background())
	if err != nil || values["greeting"] != "반갑습니다" {
		t.Fatalf("설정 조회 결과가 다르다: %v %v", values, err)
	}
	if err := svc.Set(context.Background(), map[string]any{"greeting": "안녕히"}); err != nil {
		t.Fatalf("설정 저장이 실패했다: %v", err)
	}
	reqs := f.recorded()
	last := reqs[len(reqs)-1]
	if last.Method != http.MethodPut || !strings.Contains(last.Body, "안녕히") {
		t.Errorf("설정 저장 요청이 다르다: %+v", last)
	}
}

// TestConfigService_조회실패시기동스냅샷폴백 은 Host API 장애가 확장 전체를 멈추지 않음을 고정한다.
func TestConfigService_조회실패시기동스냅샷폴백(t *testing.T) {
	f := newFakeCore(t) // /config 핸들러를 등록하지 않아 404 가 난다
	c, _ := newTestClient(t, f)

	svc := &configService{c: c, fallback: map[string]any{"greeting": "기동시값"}}
	values, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("폴백이 동작하지 않았다: %v", err)
	}
	if values["greeting"] != "기동시값" {
		t.Errorf("기동 스냅샷이 쓰이지 않았다: %v", values)
	}
	// 반환값을 바꿔도 원본 스냅샷은 유지되어야 한다.
	values["greeting"] = "변경"
	again, _ := svc.Get(context.Background())
	if again["greeting"] != "기동시값" {
		t.Errorf("폴백 스냅샷이 호출자에 의해 오염되었다: %v", again)
	}

	// 스냅샷이 없으면 오류를 숨기지 않는다.
	bare := &configService{c: c}
	if _, err := bare.Get(context.Background()); err == nil {
		t.Error("스냅샷이 없는데 오류를 숨겼다")
	}
}

// TestWorkflowService_신원없으면제출불가 는 사칭 방지 장치를 고정한다.
//
// 사용자 컨텍스트가 없는 호출(백그라운드 작업 등)에서는 신청서를 만들 수 없다.
// 이것을 허용하면 확장이 임의 사용자 명의로 신청서를 만들 수 있게 된다.
func TestWorkflowService_신원없으면제출불가(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodPost, hostPath("/workflow/topic-create"), http.StatusOK,
		extv1.RequestRef{ID: "REQ-1", State: "REQUESTED"})

	c, _ := newTestClient(t, f)
	svc := workflowService{c: c}

	if _, err := svc.SubmitTopicCreate(context.Background(), extv1.TopicCreateInput{Name: "a"}); err == nil {
		t.Fatal("신원 없는 제출이 성공했다")
	}
	// 신원만 있고 Core 가 발급한 단기 요청 토큰이 없으면 역시 제출할 수 없다
	// (Core 는 토큰 없는 On-Behalf-Of 를 사칭 시도로 보고 거부한다).
	onlyID := WithIdentity(context.Background(), extv1.Identity{UserID: "requester01"})
	if _, err := svc.SubmitTopicCreate(onlyID, extv1.TopicCreateInput{Name: "a"}); err == nil {
		t.Fatal("요청 토큰 없는 제출이 성공했다")
	}
	if n := f.count(http.MethodPost, hostPath("/workflow/topic-create")); n != 0 {
		t.Errorf("신원 없는 제출이 Core 로 나갔다: %d회", n)
	}
}

// TestHostClient_OnBehalfOf는원문그대로 는 Core 와의 사용자 ID 대조 규약을 고정한다.
//
// Core 는 On-Behalf-Of 헤더 값을 **단기 요청 토큰에 서명해 넣은 원본 사용자 ID 와 그대로** 비교한다
// (internal/extensionhost/token.go 의 verifyRequestToken). 여기서 다시 URL 인코딩하면
// '@' 나 공백이 든 사용자 ID 에서 비교가 어긋나 신청이 통째로 거부된다.
func TestHostClient_OnBehalfOf는원문그대로(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/kafka/topics"), http.StatusOK, []extv1.TopicInfo{})

	c, _ := newTestClient(t, f)
	const rawID = "user@corp.example" // QueryEscape 하면 user%40corp.example 이 된다
	ctx := WithIdentity(context.Background(), extv1.Identity{UserID: rawID})
	ctx = withRequestToken(ctx, "rt-1")

	if _, err := (kafkaService{c: c}).ListTopics(ctx, ""); err != nil {
		t.Fatalf("조회가 실패했다: %v", err)
	}
	reqs := f.recorded()
	if got := reqs[len(reqs)-1].Header.Get(HeaderOnBehalfOf); got != rawID {
		t.Errorf("On-Behalf-Of 가 원문이 아니다: got %q want %q", got, rawID)
	}
}

// TestHostClient_토큰없으면OnBehalfOf미전송 은 짝이 맞지 않는 헤더를 보내지 않음을 고정한다.
//
// Core 는 한쪽만 온 요청을 400 으로 거부하므로, 토큰이 없는 상황에서 On-Behalf-Of 를 보내면
// 신청뿐 아니라 **조회 API 까지 전부** 실패한다.
func TestHostClient_토큰없으면OnBehalfOf미전송(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodGet, hostPath("/kafka/topics"), http.StatusOK, []extv1.TopicInfo{})

	c, _ := newTestClient(t, f)
	ctx := WithIdentity(context.Background(), extv1.Identity{UserID: "requester01", RequestID: "req-1"})

	if _, err := (kafkaService{c: c}).ListTopics(ctx, ""); err != nil {
		t.Fatalf("조회가 실패했다: %v", err)
	}
	last := f.recorded()[0]
	if got := last.Header.Get(HeaderOnBehalfOf); got != "" {
		t.Errorf("요청 토큰 없이 On-Behalf-Of 를 보냈다: %q", got)
	}
	if got := last.Header.Get(HeaderRequestToken); got != "" {
		t.Errorf("없는 요청 토큰이 전송되었다: %q", got)
	}
	// 요청 ID 는 로그 상관관계용이므로 토큰과 무관하게 전달한다.
	if got := last.Header.Get(HeaderRequestID); got != "req-1" {
		t.Errorf("요청 ID 가 전달되지 않았다: %q", got)
	}
}

// TestWorkflowService_신원전달 은 on-behalf-of·요청 토큰이 함께 나가는지 고정한다.
func TestWorkflowService_신원전달(t *testing.T) {
	f := newFakeCore(t)
	f.json(http.MethodPost, hostPath("/workflow/acl-grant"), http.StatusOK,
		extv1.RequestRef{ID: "REQ-9", State: "REQUESTED"})

	c, _ := newTestClient(t, f)
	ctx := WithIdentity(context.Background(), extv1.Identity{UserID: "requester01", Name: "신청자", RequestID: "req-77"})
	ctx = withRequestToken(ctx, "rt-secret-ref")

	ref, err := workflowService{c: c}.SubmitACLGrant(ctx, extv1.ACLGrantInput{ClusterID: "prod-01"})
	if err != nil {
		t.Fatalf("제출이 실패했다: %v", err)
	}
	if ref.ID != "REQ-9" || ref.State != "REQUESTED" {
		t.Errorf("신청 참조가 다르다: %+v", ref)
	}

	reqs := f.recorded()
	last := reqs[len(reqs)-1]
	if got := last.Header.Get(HeaderOnBehalfOf); got != "requester01" {
		t.Errorf("on-behalf-of 헤더가 다르다: %q", got)
	}
	if got := last.Header.Get(HeaderRequestID); got != "req-77" {
		t.Errorf("요청 ID 가 전달되지 않았다: %q", got)
	}
	if got := last.Header.Get(HeaderRequestToken); got != "rt-secret-ref" {
		t.Errorf("단기 요청 토큰이 전달되지 않았다: %q", got)
	}
}

// TestHostContext_미선언capability는nil 은 **최소권한의 핵심**을 고정한다.
//
// typed-nil 을 넣으면 인터페이스가 non-nil 이 되어 `!= nil` 검사를 통과한다(그리고 호출에서 죽는다).
// 조건부 대입만 하므로 미선언 capability 필드는 진짜 nil 이어야 한다.
func TestHostContext_미선언capability는nil(t *testing.T) {
	f := newFakeCore(t)
	_, env := newTestClient(t, f, extv1.CapKafkaRead, extv1.CapAuditWrite)

	host, _ := newHostContext(env, newLogger(io.Discard, env.ExtensionID), time.Second)

	if host.Kafka == nil {
		t.Error("선언한 kafka.read 가 주입되지 않았다")
	}
	if host.Audit == nil {
		t.Error("선언한 audit.write 가 주입되지 않았다")
	}
	if host.Clusters != nil {
		t.Error("미선언 cluster.read 가 주입되었다")
	}
	if host.Workflow != nil {
		t.Error("미선언 workflow.submit 이 주입되었다")
	}
	if host.Config != nil {
		t.Error("미선언 config.read 가 주입되었다")
	}
	if host.Secrets != nil {
		t.Error("미선언 secret.ref 가 주입되었다")
	}
	if host.ExtensionID != "test-ext" {
		t.Errorf("ExtensionID 가 다르다: %q", host.ExtensionID)
	}

	// Require* 는 한국어 안내를 돌려준다.
	if _, err := host.RequireClusters(); err == nil {
		t.Error("미선언 capability 접근이 성공했다")
	}
}

// TestHostContext_secretRef는아직미제공 은 프로토콜 v1 의 한계를 계약으로 고정한다.
//
// 외부 프로세스 Host 프로토콜 §4 에는 secret.ref 경로가 없다. 임의 경로를 만들면 Core 와
// 계약이 어긋나므로, 선언되어 있어도 nil 로 두고 확장이 안내를 받게 한다.
func TestHostContext_secretRef는아직미제공(t *testing.T) {
	f := newFakeCore(t)
	_, env := newTestClient(t, f, extv1.CapSecretRef)
	host, _ := newHostContext(env, newLogger(io.Discard, env.ExtensionID), time.Second)
	if host.Secrets != nil {
		t.Fatal("정의되지 않은 Host API 경로로 secret.ref 를 주입했다")
	}
}
