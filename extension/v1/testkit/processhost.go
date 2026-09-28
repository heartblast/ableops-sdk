package testkit

// ProcessHost — 외부 프로세스 Extension 을 실제 프로토콜로 검증하는 테스트 Host.
//
// NewHost 가 in-process HostContext 를 조립한다면, ProcessHost 는 같은 가짜 구현을
// **Host API v1(loopback HTTP)** 로 제공하고 extserver.Run 으로 만든 바이너리를 기동한다.
//
//	환경변수(ABLEOPS_*) → 기동 → READY 핸드셰이크 → /health(호출 토큰) → 라우트 프록시(Invoke)
//	→ Extension 이 Host API 호출(Bearer + 확장 ID + 요청 토큰) → 가짜 서비스 → 종료
//
// Invoke 는 실제 Host 처럼 요청마다 **단기 요청 토큰**을 발급해 신원에 묶고, 응답이 끝나면 폐기한다.
// MCP 호출은 그 토큰과 On-Behalf-Of 가 맞을 때만 가짜 서비스에 닿으며, 서비스가 받는 컨텍스트에는
// 요청 신원이 실린다(AuthorizeRequestIdentity). 요청 컨텍스트 없는 MCP 호출은 403 이다.
//
// 지원 경로: /ping · /config · /audit · /metadata · /metrics · /mcp/* · /secrets/egress.
// kafka.read · cluster.read · workflow.submit 경로는 제공하지 않는다(NewHost 로 검증한다).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
	"github.com/heartblast/ableops-sdk/extension/v1/extserver"
)

// ProcessHost 는 Host API v1 테스트 서버다. Close 로 정리한다.
type ProcessHost struct {
	host      *Host
	version   string
	granted   []extv1.Capability
	hostToken string
	callToken string
	srv       *httptest.Server
	client    *http.Client

	mu      sync.Mutex
	leases  map[string]processLease
	addr    string
	denials []string
}

type processLease struct {
	id  extv1.Identity
	ctx context.Context
}

// ProcessOption 은 ProcessHost 옵션이다.
type ProcessOption func(*ProcessHost)

// WithProcessVersion 은 Host 가 설치했다고 알릴 확장 버전이다(ABLEOPS_EXT_VERSION).
func WithProcessVersion(v string) ProcessOption {
	return func(p *ProcessHost) { p.version = v }
}

// WithGrantedCapabilities 는 Host 가 허용할 capability 를 명시한다.
// 주지 않으면 host 에 서비스가 주입된 capability 전부다.
func WithGrantedCapabilities(caps ...extv1.Capability) ProcessOption {
	return func(p *ProcessHost) { p.granted = append([]extv1.Capability(nil), caps...) }
}

// NewProcessHost 는 host 의 가짜 서비스를 Host API v1 로 제공하는 서버를 시작한다.
func NewProcessHost(host *Host, opts ...ProcessOption) *ProcessHost {
	if host == nil {
		host = NewHost("")
	}
	p := &ProcessHost{
		host:      host,
		hostToken: randomHex(),
		callToken: randomHex(),
		leases:    map[string]processLease{},
		client:    &http.Client{Transport: &http.Transport{Proxy: nil}},
	}
	for _, c := range extv1.AllCapabilities() {
		if host.ctx.HasService(c) {
			p.granted = append(p.granted, c)
		}
	}
	for _, o := range opts {
		if o != nil {
			o(p)
		}
	}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	return p
}

// Close 는 서버를 닫고 남은 요청 토큰을 폐기한다.
func (p *ProcessHost) Close() {
	p.srv.Close()
	p.mu.Lock()
	p.leases = map[string]processLease{}
	p.mu.Unlock()
}

// URL 은 Host API 베이스 URL 이다.
func (p *ProcessHost) URL() string { return p.srv.URL }

// Granted 는 허용한 capability 다.
func (p *ProcessHost) Granted() []extv1.Capability {
	return append([]extv1.Capability(nil), p.granted...)
}

// Denials 는 Host 가 거부한 호출의 사유 목록이다("capability:mcp.call", "principal" …).
func (p *ProcessHost) Denials() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.denials...)
}

// Env 는 Host 가 자식 프로세스에 주입하는 ABLEOPS_* 환경변수다("KEY=value").
func (p *ProcessHost) Env() []string {
	names := make([]string, len(p.granted))
	for i, c := range p.granted {
		names[i] = string(c)
	}
	cfg := "{}"
	if p.host.Config != nil {
		if values, err := p.host.Config.Get(context.Background()); err == nil {
			if b, err := json.Marshal(values); err == nil {
				cfg = string(b)
			}
		}
	}
	env := []string{
		extserver.EnvExtensionID + "=" + p.host.ctx.ExtensionID,
		extserver.EnvCallToken + "=" + p.callToken,
		extserver.EnvHostURL + "=" + p.srv.URL,
		extserver.EnvHostToken + "=" + p.hostToken,
		extserver.EnvCapabilities + "=" + strings.Join(names, ","),
		extserver.EnvConfig + "=" + cfg,
		extserver.EnvProtocolVersion + "=1",
		extserver.EnvHostAPIVersion + "=" + extv1.HostAPIVersion,
	}
	if p.version != "" {
		env = append(env, extserver.EnvExtensionVersion+"="+p.version)
	}
	return env
}

// Launch 는 cmd 에 Env 를 더해 기동하고 READY·/health 까지 기다린다.
//
// cmd.Env 가 nil 이면 현재 프로세스 환경을 바탕으로 한다. stdin 은 열린 파이프로 두며
// (Host 는 stdin 을 닫아 정지시킨다) 돌려주는 WriteCloser 를 닫으면 확장이 종료된다.
func (p *ProcessHost) Launch(ctx context.Context, cmd *exec.Cmd) (extv1.ReadyMessage, io.WriteCloser, error) {
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, p.Env()...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return extv1.ReadyMessage{}, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return extv1.ReadyMessage{}, nil, err
	}
	if err := cmd.Start(); err != nil {
		return extv1.ReadyMessage{}, nil, err
	}
	ready, err := p.WaitReady(ctx, stdout)
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		return extv1.ReadyMessage{}, nil, err
	}
	return ready, stdin, nil
}

// WaitReady 는 stdout 에서 READY 줄을 읽고(INCOMPATIBLE 이면 오류) /health 가 OK 일 때까지 기다린다.
// 확장을 같은 프로세스에서 extserver.RunContext 로 돌릴 때도 쓴다(WithStdout 파이프를 넘긴다).
func (p *ProcessHost) WaitReady(ctx context.Context, stdout io.Reader) (extv1.ReadyMessage, error) {
	type result struct {
		msg extv1.ReadyMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			if body, ok := strings.CutPrefix(line, extv1.ReadyPrefix+" "); ok {
				var m extv1.ReadyMessage
				err := json.Unmarshal([]byte(body), &m)
				ch <- result{msg: m, err: err}
				_, _ = io.Copy(io.Discard, stdout)
				return
			}
			if strings.HasPrefix(line, extv1.IncompatiblePrefix) {
				ch <- result{err: errors.New(line)}
				return
			}
		}
		ch <- result{err: errors.New("testkit: stdout closed before READY")}
	}()
	var ready extv1.ReadyMessage
	select {
	case r := <-ch:
		if r.err != nil {
			return extv1.ReadyMessage{}, r.err
		}
		ready = r.msg
	case <-ctx.Done():
		return extv1.ReadyMessage{}, ctx.Err()
	}
	if ready.APIVersion != "" && ready.APIVersion != extv1.APIVersion {
		return ready, fmt.Errorf("testkit: READY apiVersion %q", ready.APIVersion)
	}
	if !extv1.SupportsProtocolVersion(ready.EffectiveProtocolVersion()) {
		return ready, fmt.Errorf("testkit: READY protocol %d", ready.EffectiveProtocolVersion())
	}
	p.mu.Lock()
	p.addr = ready.Addr
	p.mu.Unlock()
	for {
		code, h, err := p.Health(ctx)
		if err == nil && code == http.StatusOK && h.OK {
			return ready, nil
		}
		select {
		case <-ctx.Done():
			return ready, fmt.Errorf("testkit: /health not OK (status %d): %w", code, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Health 는 Host 처럼 호출 토큰을 붙여 /health 를 부른다.
func (p *ProcessHost) Health(ctx context.Context) (int, extv1.Health, error) {
	status, body, err := p.send(ctx, http.MethodGet, "/health", nil, nil)
	var h extv1.Health
	if err == nil {
		_ = json.Unmarshal(body, &h)
	}
	return status, h, err
}

// Invoke 는 사용자 요청 하나를 확장 라우트로 프록시한다(Host 의 /api/extensions/<id> 프록시 역할).
//
// 요청 토큰을 발급해 id 에 묶고, 신원 헤더를 Host 가 계산한 값으로 채운다. 응답을 다 읽으면
// (또는 ctx 가 끝나면) 토큰은 폐기된다 — 이후 그 토큰으로 온 MCP 호출은 거부된다.
func (p *ProcessHost) Invoke(ctx context.Context, id extv1.Identity, method, path string, body []byte) (int, []byte, error) {
	if strings.TrimSpace(id.UserID) == "" {
		return 0, nil, errors.New("testkit: Invoke needs a user identity")
	}
	leaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	token := randomHex()
	p.mu.Lock()
	p.leases[token] = processLease{id: id, ctx: leaseCtx}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.leases, token)
		p.mu.Unlock()
	}()
	h := http.Header{}
	extserver.SetIdentityHeaders(h, id)
	h.Set(extserver.HeaderRequestToken, token)
	return p.send(leaseCtx, method, path, body, h)
}

func (p *ProcessHost) send(ctx context.Context, method, path string, body []byte, h http.Header) (int, []byte, error) {
	p.mu.Lock()
	addr := p.addr
	p.mu.Unlock()
	if addr == "" {
		return 0, nil, errors.New("testkit: extension process is not ready")
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for k, v := range h {
		req.Header[k] = v
	}
	req.Header.Set(extserver.HeaderCallToken, p.callToken)
	req.Header.Set(extserver.HeaderExtensionID, p.host.ctx.ExtensionID)
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return resp.StatusCode, data, err
}

// ── Host API v1 ──────────────────────────────────────────────────────────────

type hostIdentityKey struct{}

// requestIdentity 는 ProcessHost 가 서비스에 넘긴 컨텍스트의 요청 신원이다.
func requestIdentity(ctx context.Context) (extv1.Identity, bool) {
	id, ok := ctx.Value(hostIdentityKey{}).(extv1.Identity)
	return id, ok
}

func (p *ProcessHost) grantedCap(c extv1.Capability) bool {
	for _, g := range p.granted {
		if g == c {
			return true
		}
	}
	return false
}

func (p *ProcessHost) deny(w http.ResponseWriter, reason string) {
	p.mu.Lock()
	p.denials = append(p.denials, reason)
	p.mu.Unlock()
	writeHostError(w, extv1.NewError(extv1.CodePermissionDenied, "denied: "+reason))
}

// principal 은 요청 토큰·On-Behalf-Of 를 검증하고 서비스에 넘길 컨텍스트를 만든다.
// 컨텍스트는 Host API 요청(확장이 취소하면 끝남)과 사용자 요청(lease) 중 먼저 끝나는 쪽에서 끝난다.
func (p *ProcessHost) principal(r *http.Request) (context.Context, context.CancelFunc, bool) {
	token := r.Header.Get(extserver.HeaderRequestToken)
	p.mu.Lock()
	lease, ok := p.leases[token]
	p.mu.Unlock()
	if token == "" || !ok || lease.ctx.Err() != nil || r.Header.Get(extserver.HeaderOnBehalfOf) != lease.id.UserID {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), hostIdentityKey{}, lease.id))
	stop := context.AfterFunc(lease.ctx, cancel)
	return ctx, func() { stop(); cancel() }, true
}

func (p *ProcessHost) serve(w http.ResponseWriter, r *http.Request) {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(p.hostToken)) != 1 || r.Header.Get(extserver.HeaderExtensionID) != p.host.ctx.ExtensionID {
		p.mu.Lock()
		p.denials = append(p.denials, "unauthenticated")
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(extv1.NewError(extv1.CodePermissionDenied, "unauthenticated"))
		return
	}
	ctx := r.Context()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/ping":
		writeHostJSON(w, map[string]any{"ok": true, "extensionId": p.host.ctx.ExtensionID})
	case r.URL.Path == "/v1/config":
		p.serveConfig(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/audit":
		if !p.grantedCap(extv1.CapAuditWrite) || p.host.Audit == nil {
			p.deny(w, "capability:"+string(extv1.CapAuditWrite))
			return
		}
		var e extv1.AuditEntry
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&e) != nil {
			writeHostError(w, extv1.NewError(extv1.CodeInvalidArgument, "invalid audit entry"))
			return
		}
		p.host.Audit.Record(ctx, e)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/metadata":
		if !p.grantedCap(extv1.CapHostMetadata) {
			p.deny(w, "capability:"+string(extv1.CapHostMetadata))
			return
		}
		if svc, err := p.host.ctx.RequireHostMetadata(); err == nil {
			md, err := svc.GetHostMetadata(ctx)
			if err != nil {
				writeHostError(w, err)
				return
			}
			writeHostJSON(w, md)
			return
		}
		caps := p.Granted()
		sort.Slice(caps, func(i, j int) bool { return caps[i] < caps[j] })
		writeHostJSON(w, extv1.HostMetadata{HostVersion: "testkit", APIVersion: extv1.APIVersion, ProtocolVersion: extv1.ProtocolVersion, ExtensionID: p.host.ctx.ExtensionID, GrantedCapabilities: caps})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/metrics":
		svc, err := p.host.ctx.RequireMetrics()
		if err != nil || !p.grantedCap(extv1.CapMetricsWrite) {
			p.deny(w, "capability:"+string(extv1.CapMetricsWrite))
			return
		}
		var m extv1.Metric
		if json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&m) != nil {
			writeHostError(w, extv1.NewError(extv1.CodeInvalidArgument, "invalid metric"))
			return
		}
		if err := svc.RecordMetric(ctx, m); err != nil {
			writeHostError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(r.URL.Path, "/v1/mcp/"):
		p.serveMCP(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/secrets/egress":
		p.serveEgress(w, r)
	default:
		writeHostError(w, extv1.NewError(extv1.CodeNotFound, "no such Host API path"))
	}
}

func (p *ProcessHost) serveConfig(w http.ResponseWriter, r *http.Request) {
	cfg := p.host.Config
	switch r.Method {
	case http.MethodGet:
		if !p.grantedCap(extv1.CapConfigRead) || cfg == nil {
			p.deny(w, "capability:"+string(extv1.CapConfigRead))
			return
		}
		values, err := cfg.Get(r.Context())
		if err != nil {
			writeHostError(w, err)
			return
		}
		writeHostJSON(w, values)
	case http.MethodPut:
		if !p.grantedCap(extv1.CapConfigWrite) || cfg == nil {
			p.deny(w, "capability:"+string(extv1.CapConfigWrite))
			return
		}
		var values map[string]any
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&values) != nil {
			writeHostError(w, extv1.NewError(extv1.CodeInvalidArgument, "invalid config"))
			return
		}
		if err := cfg.Set(r.Context(), values); err != nil {
			writeHostError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeHostError(w, extv1.NewError(extv1.CodeNotFound, "no such Host API path"))
	}
}

func (p *ProcessHost) serveMCP(w http.ResponseWriter, r *http.Request) {
	need := extv1.CapMCPRead
	if r.URL.Path == "/v1/mcp/call" {
		need = extv1.CapMCPCall
	}
	svc, err := extv1.RequireService[extv1.MCPService](p.host.ctx, need)
	if err != nil || !p.grantedCap(need) {
		p.deny(w, "capability:"+string(need))
		return
	}
	ctx, done, ok := p.principal(r)
	if !ok {
		p.deny(w, "principal")
		return
	}
	defer done()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/mcp/servers":
		servers, err := svc.ListMCPServers(ctx)
		if err != nil {
			writeHostError(w, err)
			return
		}
		writeHostJSON(w, map[string]any{"servers": servers})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/mcp/tools":
		tools, err := svc.ListMCPTools(ctx, r.URL.Query().Get("serverId"))
		if err != nil {
			writeHostError(w, err)
			return
		}
		writeHostJSON(w, map[string]any{"tools": tools})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/mcp/call":
		var call extv1.MCPToolCall
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&call) != nil || call.ServerID == "" || call.Name == "" {
			writeHostError(w, extv1.NewError(extv1.CodeInvalidArgument, "serverId and name required"))
			return
		}
		out, err := svc.CallMCPTool(ctx, call)
		if err != nil {
			writeHostError(w, err)
			return
		}
		writeHostJSON(w, out)
	default:
		writeHostError(w, extv1.NewError(extv1.CodeNotFound, "no such Host API path"))
	}
}

func (p *ProcessHost) serveEgress(w http.ResponseWriter, r *http.Request) {
	svc, err := p.host.ctx.RequireSecretUse()
	if err != nil || !p.grantedCap(extv1.CapSecretUse) {
		p.deny(w, "capability:"+string(extv1.CapSecretUse))
		return
	}
	var in extv1.SecretEgressRequest
	if json.NewDecoder(io.LimitReader(r.Body, 2*extv1.MaxSecretEgressRequestBytes)).Decode(&in) != nil || len(in.Body) > extv1.MaxSecretEgressRequestBytes {
		writeHostError(w, extv1.NewError(extv1.CodeInvalidArgument, "invalid egress request"))
		return
	}
	ctx := r.Context()
	if pctx, done, ok := p.principal(r); ok {
		defer done()
		ctx = pctx
	}
	req, err := http.NewRequestWithContext(ctx, in.Method, in.URL, bytes.NewReader(in.Body))
	if err != nil {
		writeHostError(w, extv1.NewError(extv1.CodeInvalidArgument, "invalid egress request"))
		return
	}
	req.Header = in.Header
	if req.Header == nil {
		req.Header = http.Header{}
	}
	resp, err := svc.Do(ctx, extv1.SecretRef{Ref: in.Ref}, req)
	if err != nil {
		writeHostError(w, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	CopyEgressResponse(w, resp)
}

// CopyEgressResponse 는 상류 응답을 secret.use 응답으로 스트리밍한다(Host 구현 참고용).
//
// hop-by-hop·Set-Cookie 헤더는 빼고, extserver.HeaderEgressUpstream 을 붙이며, 청크마다 flush 한다
// (LLM SSE 가 버퍼에 갇히지 않게).
func CopyEgressResponse(w http.ResponseWriter, resp *http.Response) {
	for k, v := range resp.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Connection", "Keep-Alive", "Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Set-Cookie", "Content-Length":
			continue
		}
		if strings.HasPrefix(http.CanonicalHeaderKey(k), extserver.HeaderPrefix) {
			continue
		}
		w.Header()[k] = append([]string(nil), v...)
	}
	w.Header().Set(extserver.HeaderEgressUpstream, "1")
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func writeHostJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeHostError 는 SDK Error Contract 본문으로 거부를 쓴다.
// 코드 없는 오류의 원문은 싣지 않는다(내부 사정이 경계 밖으로 새지 않게).
func writeHostError(w http.ResponseWriter, err error) {
	var e *extv1.Error
	switch {
	case errors.As(err, &e) && e != nil:
	case errors.Is(err, context.DeadlineExceeded):
		e = extv1.NewError(extv1.CodeTemporaryFailure, "deadline exceeded")
	case errors.Is(err, context.Canceled):
		e = extv1.NewError(extv1.CodeTemporaryFailure, "canceled")
	default:
		e = extv1.NewError(extv1.CodeInternal, "internal error")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(extv1.HTTPStatusForCode(e.Code))
	_ = json.NewEncoder(w).Encode(e)
}

func randomHex() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
