package extserver

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

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// HeaderEgressUpstream 은 secret.use 응답이 **상류가 준 응답**임을 표시하는 헤더다.
//
// Host 는 상류 응답(상태코드 무관)에 이 헤더를 "1" 로 붙여 그대로 스트리밍한다. 이 헤더가 없는
// 오류 응답은 Host 자신의 거부이며 SDK Error Contract 본문을 갖는다. 표시가 없으면 상류의 401 과
// Host 의 401 을 구분할 수 없다.
const HeaderEgressUpstream = "X-Ableops-Egress-Upstream"

// requestScoped 는 요청 컨텍스트(신원 + Host 가 발급한 요청 토큰)가 있는지 확인한다.
//
// MCP 는 요청 사용자의 권한으로만 동작한다. 백그라운드 컨텍스트로 부르면 Host 가 어차피 거부하므로,
// 네트워크를 타기 전에 같은 코드로 막아 원인을 정확히 알려 준다.
func requestScoped(ctx context.Context) error {
	_, hasIdentity := IdentityFrom(ctx)
	_, hasToken := requestTokenFrom(ctx)
	if !hasIdentity || !hasToken {
		return extv1.NewError(extv1.CodePermissionDenied,
			"MCP 호출에는 Host 가 프록시한 요청의 사용자 컨텍스트가 필요합니다(백그라운드 호출은 허용되지 않습니다)")
	}
	return nil
}

type mcpService struct {
	c *hostClient
	// canCall 은 mcp.call 허용 여부다. mcp.read 만 허용된 확장의 CallMCPTool 은 호출 전에 막는다.
	canCall bool
}

func (s mcpService) ListMCPServers(ctx context.Context) ([]extv1.MCPServer, error) {
	if err := requestScoped(ctx); err != nil {
		return nil, err
	}
	var out struct {
		Servers []extv1.MCPServer `json:"servers"`
	}
	err := s.c.get(ctx, "/mcp/servers", nil, &out)
	return out.Servers, err
}

func (s mcpService) ListMCPTools(ctx context.Context, serverID string) ([]extv1.MCPTool, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, extv1.NewError(extv1.CodeInvalidArgument, "MCP 서버 ID 가 비어 있습니다")
	}
	if err := requestScoped(ctx); err != nil {
		return nil, err
	}
	var out struct {
		Tools []extv1.MCPTool `json:"tools"`
	}
	err := s.c.get(ctx, "/mcp/tools", url.Values{"serverId": {serverID}}, &out)
	return out.Tools, err
}

// CallMCPTool 은 도구를 호출한다. 재시도하지 않는다(도구는 부작용이 있을 수 있다).
// 전체 기한은 호출 컨텍스트가 정한다(없으면 Host 클라이언트 기본값).
func (s mcpService) CallMCPTool(ctx context.Context, call extv1.MCPToolCall) (extv1.MCPToolResult, error) {
	if !s.canCall {
		return extv1.MCPToolResult{}, extv1.Errorf(extv1.CodeCapabilityUnavailable,
			"확장 기능 %s: MCP 도구 호출에는 %s capability 가 필요합니다", s.c.extensionID, string(extv1.CapMCPCall))
	}
	if strings.TrimSpace(call.ServerID) == "" || strings.TrimSpace(call.Name) == "" {
		return extv1.MCPToolResult{}, extv1.NewError(extv1.CodeInvalidArgument, "MCP 서버 ID 와 도구 이름이 필요합니다")
	}
	if err := requestScoped(ctx); err != nil {
		return extv1.MCPToolResult{}, err
	}
	var out extv1.MCPToolResult
	err := s.c.call(ctx, http.MethodPost, "/mcp/call", call, &out)
	return out, err
}

type metricsService struct{ c *hostClient }

func (s metricsService) RecordMetric(ctx context.Context, metric extv1.Metric) error {
	return s.c.do(ctx, http.MethodPost, "/metrics", nil, metric, nil)
}

type metadataService struct{ c *hostClient }

func (s metadataService) GetHostMetadata(ctx context.Context) (extv1.HostMetadata, error) {
	var out extv1.HostMetadata
	err := s.c.get(ctx, "/metadata", nil, &out)
	return out, err
}

// ── secret.use ───────────────────────────────────────────────────────────────

// secretUseService 는 SecretUseService 의 Host API 구현이다(POST /secrets/egress).
//
// 요청 본문을 SecretEgressRequest 로 감싸 보내고, 상류 응답은 **버퍼링 없이** 그대로 돌려준다
// (LLM SSE 스트리밍). 평문 시크릿은 이 경로 어디에도 없다 — Host 가 상류 요청에만 넣는다.
type secretUseService struct{ c *hostClient }

var _ extv1.SecretUseService = secretUseService{}

func (s secretUseService) Do(ctx context.Context, ref extv1.SecretRef, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, extv1.NewError(extv1.CodeInvalidArgument, "요청이 비어 있습니다")
	}
	body, err := readEgressBody(req)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ref.Ref) == "" {
		return nil, extv1.NewError(extv1.CodeInvalidArgument, "시크릿 참조가 비어 있습니다")
	}
	if req.URL == nil {
		return nil, extv1.NewError(extv1.CodeInvalidArgument, "egress URL 이 비어 있습니다")
	}
	if _, err := extv1.SecretEgressOrigin(req.URL.String()); err != nil {
		return nil, err
	}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	payload, err := jsonMarshal(extv1.SecretEgressRequest{
		Ref:    ref.Ref,
		Method: method,
		URL:    req.URL.String(),
		Header: extv1.SanitizeSecretEgressHeader(req.Header, ""),
		Body:   body,
	})
	if err != nil {
		return nil, err
	}

	// 기한 없는 컨텍스트에는 기본 기한을 건다. 응답 본문을 다 읽을 때까지 유지해야 하므로
	// cancel 은 본문 Close 에 묶는다.
	ctx, cancel := s.c.callContext(ctx)
	const path = "/secrets/egress"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.c.baseURL+s.c.versionedPath(path), bytes.NewReader(payload))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("Core Host API 요청을 만들 수 없습니다(POST %s): %w", path, err)
	}
	s.c.setHeaders(ctx, hreq, true)
	hreq.Header.Set("Accept", "*/*")
	resp, err := s.c.stream.Do(hreq)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("Core Host API 에 연결하지 못했습니다(POST %s): %w", path, err)
	}
	if resp.Header.Get(HeaderEgressUpstream) != "1" {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		cancel()
		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			return nil, extv1.NewError(extv1.CodeIncompatible, "Host 가 secret.use 상류 응답 표시를 보내지 않았습니다")
		}
		return nil, hostErrorFrom(http.MethodPost, path, resp, data)
	}
	resp.Header.Del(HeaderEgressUpstream)
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	resp.Request = req
	return resp, nil
}

// readEgressBody 는 요청 본문을 상한까지 읽고 닫는다(RoundTripper 계약: 본문은 항상 닫는다).
func readEgressBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	defer func() { _ = req.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(req.Body, extv1.MaxSecretEgressRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("egress 요청 본문을 읽지 못했습니다: %w", err)
	}
	if len(data) > extv1.MaxSecretEgressRequestBytes {
		return nil, extv1.Errorf(extv1.CodeInvalidArgument, "egress 요청 본문이 상한(%d바이트)을 넘었습니다", extv1.MaxSecretEgressRequestBytes)
	}
	return data, nil
}

// cancelOnClose 는 응답 본문을 닫을 때 호출 컨텍스트를 정리한다.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func jsonMarshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, errors.New("egress 요청을 만들 수 없습니다")
	}
	return b, nil
}
