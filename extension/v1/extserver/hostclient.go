package extserver

// Core Host API 클라이언트(프로토콜 §4) — HostContext 각 서비스의 실제 구현.
//
// 베이스 URL 은 ABLEOPS_HOST_URL, 인증은 `Authorization: Bearer <ABLEOPS_HOST_TOKEN>` 이다.
//
//	GET  /ping                                연결 확인
//	GET  /kafka/topics · /kafka/topics/{name} · /kafka/acls · /kafka/consumer-groups   kafka.read
//	GET  /clusters · /clusters/{id}           cluster.read
//	POST /audit                               audit.write
//	GET  /config                              config.read
//	PUT  /config                              config.write
//	POST /workflow/topic-create|topic-update|topic-delete|acl-grant|acl-revoke          workflow.submit
//
// 응답 본문은 SDK DTO 의 JSON 직렬화 그대로다(extensionv1.TopicInfo 등).
// 오류는 {"error":"한국어 메시지"} + 상태코드이며, 그 메시지를 그대로 보존해 전달한다
// (Core 가 왜 거부했는지가 확장 사용자에게 보여야 한다).
//
// ⚠ 토큰은 Authorization 헤더로만 나가며, 오류 메시지·로그·URL 어디에도 싣지 않는다.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

const (
	// defaultHostTimeout 은 Host API 호출 1건의 기본 타임아웃이다.
	// Kafka 메타데이터 조회가 브로커 응답을 기다릴 수 있으므로 넉넉히 잡되 무한 대기는 두지 않는다.
	defaultHostTimeout = 20 * time.Second
	// maxGetAttempts 는 멱등 GET 의 최대 시도 횟수다(최초 1회 + 재시도 2회).
	maxGetAttempts = 3
	// retryBaseDelay 는 재시도 간 대기의 기준값이다(시도마다 배증).
	retryBaseDelay = 120 * time.Millisecond
	// maxHostResponseBytes 는 응답 본문 읽기 상한이다(Core 응답이라도 무한히 읽지 않는다).
	maxHostResponseBytes = 16 << 20
)

// HostError 는 Core Host API 가 오류 상태코드로 응답했을 때의 오류다.
//
// 상태코드를 보존하는 이유: 확장이 401(토큰 문제)·403(capability 미선언)·404(미등록)를 구분해
// 자기 응답 상태코드를 정할 수 있어야 하기 때문이다. 네트워크 실패는 이 타입이 아니다.
type HostError struct {
	Method     string
	Path       string
	StatusCode int
	// Message 는 Core 가 준 한국어 오류 메시지다(없으면 빈 문자열).
	Message string
	// Code 는 Core 가 준 안정적인 오류 코드다(SDK Error Contract).
	//
	// 구버전 Core 는 코드를 보내지 않으므로, 그 경우 상태코드로부터 유추한다
	// (extensionv1.CodeForHTTPStatus). **분기는 Message 가 아니라 이 값으로 한다.**
	Code extv1.ErrorCode
	// RequestID 는 Core 가 이 요청에 붙인 추적 ID 다(있을 때만).
	RequestID string
}

// Error 는 한국어 오류 문자열을 만든다(토큰은 포함하지 않는다).
func (e *HostError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "Core 가 오류 사유를 제공하지 않았습니다"
	}
	suffix := ""
	if e.RequestID != "" {
		suffix = fmt.Sprintf(" (요청 %s)", e.RequestID)
	}
	return fmt.Sprintf("Core Host API 호출이 거부되었습니다(%s %s, 상태 %d, 코드 %s): %s%s",
		e.Method, e.Path, e.StatusCode, string(e.Code), msg, suffix)
}

// Is 는 extensionv1.Error 와 **코드로** 비교되게 한다.
//
// 덕분에 확장은 in-process 든 외부 프로세스든 같은 방식으로 분기할 수 있다:
//
//	if errors.Is(err, extensionv1.NewError(extensionv1.CodeNotFound, "")) { … }
func (e *HostError) Is(target error) bool {
	var t *extv1.Error
	if !errors.As(target, &t) || t == nil || e == nil {
		return false
	}
	return t.Code == e.Code
}

// AsSDKError 는 SDK 표준 오류로 바꾼다(코드·메시지·추적 ID 보존).
func (e *HostError) AsSDKError() *extv1.Error {
	if e == nil {
		return nil
	}
	return (&extv1.Error{Code: e.Code, Message: e.Message}).WithRequestID(e.RequestID)
}

// HostErrorCode 는 오류에서 SDK 오류 코드를 꺼낸다(코드가 없으면 빈 문자열).
//
// 네트워크 실패처럼 Core 응답이 없는 오류는 TEMPORARY_FAILURE 로 본다 — 재시도할 가치가 있다.
func HostErrorCode(err error) extv1.ErrorCode {
	if err == nil {
		return ""
	}
	var he *HostError
	if errors.As(err, &he) {
		return he.Code
	}
	if c := extv1.CodeOf(err); c != "" {
		return c
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ""
	}
	return extv1.CodeTemporaryFailure
}

// HostStatusCode 는 오류에서 Host API 상태코드를 꺼낸다(Host 오류가 아니면 0).
func HostStatusCode(err error) int {
	var he *HostError
	if errors.As(err, &he) {
		return he.StatusCode
	}
	return 0
}

// hostClient 는 Host API HTTP 클라이언트다.
type hostClient struct {
	baseURL     string
	token       string
	extensionID string
	http        *http.Client
	log         extv1.Logger
	// apiPath 는 상대 경로에 Host API 버전 세그먼트를 붙이는 함수다(env.HostAPIPath).
	//
	// 함수로 들고 있는 이유: 구버전 Core 는 버전 없는 경로만 알고, 신버전은 /v1 을 쓴다.
	// 호출부(각 서비스)가 그 차이를 몰라도 되게 한 곳에서 결정한다.
	apiPath func(string) string
}

// newHostClient 는 클라이언트를 만든다(timeout 이 0 이하면 기본값).
func newHostClient(env Environment, log extv1.Logger, timeout time.Duration) *hostClient {
	if timeout <= 0 {
		timeout = defaultHostTimeout
	}
	return &hostClient{
		baseURL:     env.HostURL,
		token:       env.HostToken,
		extensionID: env.ExtensionID,
		http:        &http.Client{Timeout: timeout},
		log:         log,
		apiPath:     env.HostAPIPath,
	}
}

// pingResult 는 GET /ping 응답이다.
type pingResult struct {
	OK          bool   `json:"ok"`
	ExtensionID string `json:"extensionId"`
}

// Ping 은 Host API 연결을 확인한다(기동 직후 자기 진단용).
func (c *hostClient) Ping(ctx context.Context) error {
	var out pingResult
	if err := c.get(ctx, "/ping", nil, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("Core Host API 가 정상 응답을 주지 않았습니다(/ping)")
	}
	if out.ExtensionID != "" && c.extensionID != "" && out.ExtensionID != c.extensionID {
		return fmt.Errorf("Core Host API 가 다른 확장(%q)으로 인식하고 있습니다(기대 %q)", out.ExtensionID, c.extensionID)
	}
	return nil
}

// get 은 멱등 GET 호출이다(재시도 대상).
func (c *hostClient) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

// do 는 Host API 호출을 수행한다. 재시도는 **GET 만** 한다 — POST/PUT 을 재시도하면
// 감사 기록이나 신청서가 중복 생성될 수 있고, 그것은 조회 실패보다 훨씬 나쁜 결과다.
func (c *hostClient) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	endpoint := c.baseURL + c.versionedPath(path)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("Core Host API 요청 본문을 만들 수 없습니다(%s %s): %w", method, path, err)
		}
		payload = b
	}

	attempts := 1
	if method == http.MethodGet {
		attempts = maxGetAttempts
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			delay := retryBaseDelay * time.Duration(1<<(i-1))
			select {
			case <-ctx.Done():
				return fmt.Errorf("Core Host API 호출이 취소되었습니다(%s %s): %w", method, path, ctx.Err())
			case <-time.After(delay):
			}
		}
		err := c.attempt(ctx, method, endpoint, path, payload, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryableHostError(err) {
			return err
		}
	}
	return lastErr
}

// versionedPath 는 상대 경로에 Host API 버전 세그먼트를 붙인다(미설정이면 그대로).
func (c *hostClient) versionedPath(path string) string {
	if c.apiPath == nil {
		return path
	}
	return c.apiPath(path)
}

// attempt 는 실제 HTTP 요청 1회다.
func (c *hostClient) attempt(ctx context.Context, method, endpoint, path string, payload []byte, out any) error {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("Core Host API 요청을 만들 수 없습니다(%s %s): %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderExtensionID, c.extensionID)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	// 요청 사용자 컨텍스트 전달(§4).
	//
	// workflow.submit 은 "누구 명의로 신청하는가"가 필요하다. Extension 이 임의 사용자를 적어
	// 사칭하는 것을 막기 위해, Core 가 프록시 때 발급한 값을 **그대로 되돌려 준다**:
	//   - On-Behalf-Of: 지금 처리 중인 요청의 사용자 ID(헤더에서 복원한 값 — 확장이 지어내지 않는다)
	//   - Request-Id / Request-Token: Core 가 그 요청에 붙인 식별자·단기 토큰
	// Core 는 (사용자, 요청식별자)가 방금 그 확장에 프록시한 요청과 일치하는지 확인할 수 있다.
	// 백그라운드 작업처럼 요청 컨텍스트가 없는 호출에는 이 헤더들이 붙지 않으므로,
	// Core 는 그런 호출의 workflow.submit 을 거부할 수 있다.
	//
	// ⚠ 두 헤더는 **함께** 보낸다. Core 는 짝이 맞지 않는 요청을 거부하므로(한쪽만 오면
	// 감사 주체가 사라진 채 처리되기 때문), 토큰이 없는 상황에서 On-Behalf-Of 만 보내면
	// 조회 API 까지 전부 400 이 된다.
	//
	// ⚠ On-Behalf-Of 값은 **URL 인코딩하지 않는다**. Core 는 이 값을 단기 요청 토큰 안에
	// 서명해 넣은 원본 사용자 ID 와 그대로 비교한다 — 다시 인코딩하면 그 비교가 어긋난다.
	// (여기서 쓰는 id.UserID 는 프록시 헤더를 디코딩해 얻은 원본이므로 왕복이 정확히 맞는다.)
	id, hasIdentity := IdentityFrom(ctx)
	token, hasToken := requestTokenFrom(ctx)
	if hasIdentity && hasToken {
		req.Header.Set(HeaderOnBehalfOf, id.UserID)
		req.Header.Set(HeaderRequestToken, token)
	}
	// 추적 ID 는 **신원과 무관하게** 항상 전달한다(요구 §16).
	// 신원이 있을 때만 붙이면 익명·백그라운드 호출의 상관관계가 끊긴다.
	if rid := RequestIDFrom(ctx); rid != "" {
		req.Header.Set(HeaderRequestID, rid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error 는 요청 URL 을 포함하지만 토큰은 헤더에만 있으므로 노출되지 않는다.
		return fmt.Errorf("Core Host API 에 연결하지 못했습니다(%s %s): %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxHostResponseBytes))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, code, rid := errorEnvelope(data)
		if code == "" {
			// 구버전 Core 는 코드를 보내지 않는다 — 상태코드로 유추한다.
			code = extv1.CodeForHTTPStatus(resp.StatusCode)
		}
		if rid == "" {
			rid = strings.TrimSpace(resp.Header.Get(HeaderRequestID))
		}
		return &HostError{Method: method, Path: path, StatusCode: resp.StatusCode, Message: msg, Code: code, RequestID: rid}
	}
	if readErr != nil {
		return fmt.Errorf("Core Host API 응답을 읽지 못했습니다(%s %s): %w", method, path, readErr)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("Core Host API 응답을 해석하지 못했습니다(%s %s): %w", method, path, err)
	}
	return nil
}

// errorEnvelope 는 오류 본문에서 메시지·코드·추적 ID 를 꺼낸다.
//
// Core 오류 본문은 하위호환을 위해 `error`(구 형식)와 `code`/`message`/`requestId`(SDK Error
// Contract)를 **함께** 싣는다. 어느 한쪽만 있는 응답도 해석한다.
// JSON 이 아니면 본문 앞부분을 메시지로 그대로 쓴다(원문 보존 — 원인 추적에 필요하다).
func errorEnvelope(data []byte) (message string, code extv1.ErrorCode, requestID string) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", "", ""
	}
	var body struct {
		Error     string `json:"error"`
		Message   string `json:"message"`
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(trimmed, &body); err == nil {
		msg := strings.TrimSpace(body.Error)
		if msg == "" {
			msg = strings.TrimSpace(body.Message)
		}
		if msg != "" || strings.TrimSpace(body.Code) != "" {
			return msg, extv1.ErrorCode(strings.TrimSpace(body.Code)), strings.TrimSpace(body.RequestID)
		}
	}
	const maxRaw = 300
	raw := string(trimmed)
	if len(raw) > maxRaw {
		raw = raw[:maxRaw] + "…"
	}
	return raw, "", ""
}

// retryableHostError 는 재시도해도 되는 실패인지 판정한다.
// 네트워크 실패와 일시적 서버 오류(502/503/504)만 재시도한다 — 401/403/404 는 재시도해도 같다.
func retryableHostError(err error) bool {
	var he *HostError
	if errors.As(err, &he) {
		// 코드가 있으면 코드로 판정한다(SDK Error Contract — TEMPORARY_FAILURE 만 재시도).
		if he.Code != "" {
			if extv1.RetryableCode(he.Code) {
				return true
			}
			// HOST_UNAVAILABLE 은 "지금은 안 되지만 곧 될 수 있다" — Core 기동 직후가 대표적이다.
			return he.Code == extv1.CodeHostUnavailable
		}
		switch he.StatusCode {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	// 컨텍스트 취소는 재시도 대상이 아니다.
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// addIfNotEmpty 는 값이 있을 때만 쿼리에 추가한다.
// 빈 clusterId 는 아예 보내지 않는다 — Core 가 "미지정 = 기본 클러스터"로 해석한다(SDK kafka.go 계약).
func addIfNotEmpty(q url.Values, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		q.Set(key, v)
	}
}

// ── kafka.read ───────────────────────────────────────────────────────────────

// kafkaService 는 KafkaReadService 의 Host API 구현이다.
type kafkaService struct{ c *hostClient }

var _ extv1.KafkaReadService = kafkaService{}

func (s kafkaService) ListTopics(ctx context.Context, clusterID string) ([]extv1.TopicInfo, error) {
	q := url.Values{}
	addIfNotEmpty(q, "clusterId", clusterID)
	var out []extv1.TopicInfo
	if err := s.c.get(ctx, "/kafka/topics", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s kafkaService) DescribeTopic(ctx context.Context, clusterID, topic string) (extv1.TopicDetail, error) {
	name := strings.TrimSpace(topic)
	if name == "" {
		return extv1.TopicDetail{}, errors.New("Topic 이름이 비어 있습니다")
	}
	q := url.Values{}
	addIfNotEmpty(q, "clusterId", clusterID)
	var out extv1.TopicDetail
	if err := s.c.get(ctx, "/kafka/topics/"+url.PathEscape(name), q, &out); err != nil {
		return extv1.TopicDetail{}, err
	}
	return out, nil
}

func (s kafkaService) ListACLs(ctx context.Context, clusterID string, filter extv1.ACLFilter) ([]extv1.ACLEntry, error) {
	q := url.Values{}
	addIfNotEmpty(q, "clusterId", clusterID)
	addIfNotEmpty(q, "principal", filter.Principal)
	addIfNotEmpty(q, "resourceType", filter.ResourceType)
	addIfNotEmpty(q, "resourceName", filter.ResourceName)
	addIfNotEmpty(q, "patternType", filter.PatternType)
	addIfNotEmpty(q, "operation", filter.Operation)
	addIfNotEmpty(q, "permissionType", filter.PermissionType)
	addIfNotEmpty(q, "host", filter.Host)
	var out []extv1.ACLEntry
	if err := s.c.get(ctx, "/kafka/acls", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s kafkaService) ListConsumerGroups(ctx context.Context, clusterID string) ([]extv1.ConsumerGroupInfo, error) {
	q := url.Values{}
	addIfNotEmpty(q, "clusterId", clusterID)
	var out []extv1.ConsumerGroupInfo
	if err := s.c.get(ctx, "/kafka/consumer-groups", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ── cluster.read ─────────────────────────────────────────────────────────────

// clusterService 는 ClusterRegistry 의 Host API 구현이다.
type clusterService struct{ c *hostClient }

var _ extv1.ClusterRegistry = clusterService{}

func (s clusterService) List(ctx context.Context) ([]extv1.ClusterInfo, error) {
	var out []extv1.ClusterInfo
	if err := s.c.get(ctx, "/clusters", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get 은 SDK 계약상 오류를 돌려주지 않는다(존재 여부만 알린다).
// 따라서 조회 실패는 ok=false 로 축약하되, 원인을 잃지 않도록 경고 로그를 남긴다.
func (s clusterService) Get(ctx context.Context, id string) (extv1.ClusterInfo, bool) {
	name := strings.TrimSpace(id)
	if name == "" {
		return extv1.ClusterInfo{}, false
	}
	var out extv1.ClusterInfo
	if err := s.c.get(ctx, "/clusters/"+url.PathEscape(name), nil, &out); err != nil {
		if HostStatusCode(err) != http.StatusNotFound {
			s.c.log.Warn("클러스터 조회에 실패했습니다", "clusterId", name, "error", err.Error())
		}
		return extv1.ClusterInfo{}, false
	}
	if out.ID == "" {
		return extv1.ClusterInfo{}, false
	}
	return out, true
}

// ── audit.write ──────────────────────────────────────────────────────────────

// auditService 는 AuditService 의 Host API 구현이다.
type auditService struct{ c *hostClient }

var _ extv1.AuditService = auditService{}

// Record 는 감사 기록을 남긴다.
//
// SDK 계약상 오류를 돌려주지 않는다 — 감사 저장 실패로 사용자의 요청을 실패시키지 않기 위함이다.
// 대신 실패를 삼키지 않고 로그로 남긴다(기록 누락은 통제 실패이므로 흔적이 있어야 한다).
func (s auditService) Record(ctx context.Context, entry extv1.AuditEntry) {
	if err := s.c.do(ctx, http.MethodPost, "/audit", nil, entry, nil); err != nil {
		s.c.log.Error("감사 기록 전송에 실패했습니다", "action", entry.Action, "error", err.Error())
	}
}

// ── config.read ──────────────────────────────────────────────────────────────

// configService 는 ConfigService 의 Host API 구현이다.
//
// fallback 은 기동 시 Core 가 환경변수로 넘긴 설정 스냅샷이다. Host API 조회가 실패했을 때
// 확장이 통째로 멈추지 않도록 이 스냅샷으로 폴백한다(경고 로그를 반드시 남긴다).
type configService struct {
	c        *hostClient
	fallback map[string]any
	// canWrite 는 config.write capability 선언 여부다.
	//
	// ⚠ 인터페이스에서 Set 을 빼지 않고 런타임에서 막는 이유: ConfigService 는 v1 공개 계약이라
	// 메서드를 제거하면 기존 확장이 컴파일되지 않는다. 대신 여기서 PERMISSION_DENIED 를 돌려주고,
	// 새 코드는 RequireService[ConfigWriteService](host, CapConfigWrite) 로 **호출 전에** 확인한다.
	canWrite bool
}

var _ extv1.ConfigService = (*configService)(nil)

// configWriter 는 config.write 전용 서비스다(Resolver 가 돌려주는 타입).
type configWriter struct{ svc *configService }

var _ extv1.ConfigWriteService = configWriter{}

func (w configWriter) Set(ctx context.Context, values map[string]any) error {
	return w.svc.Set(ctx, values)
}

func (s *configService) Get(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := s.c.get(ctx, "/config", nil, &out); err != nil {
		if s.fallback == nil {
			return nil, err
		}
		s.c.log.Warn("확장 설정 조회에 실패해 기동 시 스냅샷을 사용합니다", "error", err.Error())
		return copyValues(s.fallback), nil
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (s *configService) Set(ctx context.Context, values map[string]any) error {
	if !s.canWrite {
		return extv1.Errorf(extv1.CodePermissionDenied,
			"확장 기능 %s: 설정 저장에는 %s capability 가 필요합니다(Manifest capabilities 에 추가하세요)",
			s.c.extensionID, string(extv1.CapConfigWrite))
	}
	if values == nil {
		values = map[string]any{}
	}
	return s.c.do(ctx, http.MethodPut, "/config", nil, values, nil)
}

// copyValues 는 맵 사본을 만든다(폴백 스냅샷이 호출자에 의해 변경되지 않도록).
func copyValues(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ── workflow.submit ──────────────────────────────────────────────────────────

// workflowService 는 WorkflowSubmitService 의 Host API 구현이다.
//
// ⚠ 제출(Submit)만 있다. Approve/Apply/Reject 는 SDK 인터페이스에 존재하지 않으므로
// 이 클라이언트에도 대응 경로가 없다(신청자≠승인자 원칙을 타입 수준에서 지킨다).
type workflowService struct{ c *hostClient }

var _ extv1.WorkflowSubmitService = workflowService{}

// submit 은 신청 제출 공통 경로다.
//
// 요청 컨텍스트에 사용자 신원과 Core 가 발급한 단기 요청 토큰이 **둘 다** 없으면 호출하지 않는다.
// 신원 없는 신청은 Core 가 어차피 거부하며(감사 주체가 없는 신청은 통제가 성립하지 않는다),
// 여기서 먼저 막으면 원인을 한국어로 정확히 알려줄 수 있다.
func (s workflowService) submit(ctx context.Context, path string, in any) (extv1.RequestRef, error) {
	_, hasIdentity := IdentityFrom(ctx)
	_, hasToken := requestTokenFrom(ctx)
	if !hasIdentity || !hasToken {
		return extv1.RequestRef{}, errors.New("사용자 컨텍스트가 없는 호출에서는 신청서를 제출할 수 없습니다(Core 가 프록시한 요청을 처리하는 중에만 제출할 수 있습니다)")
	}
	var out extv1.RequestRef
	if err := s.c.do(ctx, http.MethodPost, path, nil, in, &out); err != nil {
		return extv1.RequestRef{}, err
	}
	return out, nil
}

func (s workflowService) SubmitTopicCreate(ctx context.Context, in extv1.TopicCreateInput) (extv1.RequestRef, error) {
	return s.submit(ctx, "/workflow/topic-create", in)
}

func (s workflowService) SubmitTopicUpdate(ctx context.Context, in extv1.TopicUpdateInput) (extv1.RequestRef, error) {
	return s.submit(ctx, "/workflow/topic-update", in)
}

func (s workflowService) SubmitTopicDelete(ctx context.Context, in extv1.TopicDeleteInput) (extv1.RequestRef, error) {
	return s.submit(ctx, "/workflow/topic-delete", in)
}

func (s workflowService) SubmitACLGrant(ctx context.Context, in extv1.ACLGrantInput) (extv1.RequestRef, error) {
	return s.submit(ctx, "/workflow/acl-grant", in)
}

func (s workflowService) SubmitACLRevoke(ctx context.Context, in extv1.ACLRevokeInput) (extv1.RequestRef, error) {
	return s.submit(ctx, "/workflow/acl-revoke", in)
}

// ── HostContext 조립 ─────────────────────────────────────────────────────────

// newHostContext 는 선언된 capability 만 채운 HostContext 를 만든다.
//
// ⚠ **조건부 대입만 한다(typed-nil 금지).** Go 에서 nil 포인터를 인터페이스에 담으면
// 인터페이스 자체는 non-nil 이 되어 `if host.Kafka != nil` 이 통과하고, 최소권한이 무력화된 채로
// 실제 호출에서 죽는다. 미선언 capability 는 필드에 **아무것도 대입하지 않는다**.
func newHostContext(env Environment, log extv1.Logger, timeout time.Duration) (extv1.HostContext, *hostClient) {
	client := newHostClient(env, log, timeout)

	host := extv1.HostContext{
		ExtensionID: env.ExtensionID,
		Identity:    IdentityFrom,
		Log:         log,
	}
	// ⚠ Resolver 에도 **실제로 주입한 것만** 넣는다. 빈 항목을 넣어 두면 HasService 가 true 가 되어
	// 최소권한 검사가 통과한다(services.go 의 typed-nil 주석과 같은 이유).
	services := extv1.ServiceMap{}

	canWriteConfig := env.HasCapability(extv1.CapConfigWrite)
	if env.HasCapability(extv1.CapKafkaRead) {
		svc := kafkaService{c: client}
		host.Kafka = svc
		services[extv1.CapKafkaRead] = svc
	}
	if env.HasCapability(extv1.CapClusterRead) {
		svc := clusterService{c: client}
		host.Clusters = svc
		services[extv1.CapClusterRead] = svc
	}
	if env.HasCapability(extv1.CapWorkflowSubmit) {
		svc := workflowService{c: client}
		host.Workflow = svc
		services[extv1.CapWorkflowSubmit] = svc
	}
	if env.HasCapability(extv1.CapAuditWrite) {
		svc := auditService{c: client}
		host.Audit = svc
		services[extv1.CapAuditWrite] = svc
	}
	if env.HasCapability(extv1.CapConfigRead) || canWriteConfig {
		svc := &configService{c: client, fallback: env.Config, canWrite: canWriteConfig}
		// config.read 없이 config.write 만 선언한 확장도 성립한다(쓰기 전용). 그 경우
		// HostContext.Config 는 채우되 Get 은 Core 가 403 으로 막는다.
		host.Config = svc
		if env.HasCapability(extv1.CapConfigRead) {
			services[extv1.CapConfigRead] = extv1.ConfigReadService(svc)
		}
		if canWriteConfig {
			services[extv1.CapConfigWrite] = extv1.ConfigWriteService(configWriter{svc: svc})
		}
	}
	if env.HasCapability(extv1.CapSecretRef) {
		// 외부 프로세스 Host 프로토콜 v1 에는 secret.ref 경로가 없다(§4 엔드포인트 목록).
		// 임의로 경로를 만들면 Core 와 계약이 어긋나므로 **nil 로 남기고 사유를 알린다** —
		// 확장은 RequireSecrets() 에서 "capability 미제공" 안내를 받는다.
		log.Warn("secret.ref 는 외부 프로세스 Extension 에서 아직 제공되지 않습니다(Host API 경로 미정의)",
			"capability", string(extv1.CapSecretRef))
	}
	host.Services = services
	return host, client
}
