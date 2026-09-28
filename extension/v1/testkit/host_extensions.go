package testkit

import (
	"context"
	"net/http"
	"strings"
	"sync"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// FakeMCP records calls and returns a fixed inventory. It requires a request
// context marker supplied by the test so background calls fail closed.
//
// Failure injection: Err fails every method with that error. CallFunc, when set,
// replaces Result for CallMCPTool (return a context error to model a slow tool;
// it receives the caller's context, so deadlines and cancellation propagate).
// Every method returns the context error first when ctx is already done.
type FakeMCP struct {
	mu        sync.Mutex
	Servers   []extv1.MCPServer
	Tools     map[string][]extv1.MCPTool
	Calls     []extv1.MCPToolCall
	Result    extv1.MCPToolResult
	Authorize func(context.Context) bool
	Err       error
	CallFunc  func(context.Context, extv1.MCPToolCall) (extv1.MCPToolResult, error)
}

func (f *FakeMCP) allowed(ctx context.Context) bool { return f.Authorize != nil && f.Authorize(ctx) }

func (f *FakeMCP) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !f.allowed(ctx) {
		return extv1.NewError(extv1.CodePermissionDenied, "request principal required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Err
}

func (f *FakeMCP) ListMCPServers(ctx context.Context) ([]extv1.MCPServer, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.MCPServer(nil), f.Servers...), nil
}
func (f *FakeMCP) ListMCPTools(ctx context.Context, id string) ([]extv1.MCPTool, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	tools, ok := f.Tools[id]
	if !ok {
		return nil, extv1.NewError(extv1.CodeNotFound, "MCP server not found")
	}
	return append([]extv1.MCPTool(nil), tools...), nil
}
func (f *FakeMCP) CallMCPTool(ctx context.Context, call extv1.MCPToolCall) (extv1.MCPToolResult, error) {
	if err := f.check(ctx); err != nil {
		return extv1.MCPToolResult{}, err
	}
	f.mu.Lock()
	f.Calls = append(f.Calls, call)
	fn, result := f.CallFunc, f.Result
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, call)
	}
	return result, nil
}

// CallHistory returns a copy of the recorded calls (safe while calls run).
func (f *FakeMCP) CallHistory() []extv1.MCPToolCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.MCPToolCall(nil), f.Calls...)
}

// mcpReadOnly is what an mcp.read grant exposes: listing works, calls do not.
type mcpReadOnly struct{ *FakeMCP }

func (m mcpReadOnly) CallMCPTool(context.Context, extv1.MCPToolCall) (extv1.MCPToolResult, error) {
	return extv1.MCPToolResult{}, extv1.NewError(extv1.CodeCapabilityUnavailable, "mcp.call is not granted")
}

type FakeMetrics struct {
	mu      sync.Mutex
	Records []extv1.Metric
}

func (f *FakeMetrics) RecordMetric(_ context.Context, m extv1.Metric) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Records = append(f.Records, m)
	return nil
}

type FakeHostMetadata struct{ Value extv1.HostMetadata }

func (f FakeHostMetadata) GetHostMetadata(context.Context) (extv1.HostMetadata, error) {
	return f.Value, nil
}

// WithMCPRead grants mcp.read. Unless WithMCPCall is also applied, CallMCPTool
// on the returned service fails with CAPABILITY_UNAVAILABLE, as on a real Host.
func WithMCPRead(f *FakeMCP) Option {
	return WithService(extv1.CapMCPRead, extv1.MCPService(mcpReadOnly{f}))
}

// WithMCPCall grants mcp.call. A Host that grants mcp.call serves listing on the
// same service, so RequireMCPRead then also returns the full service.
func WithMCPCall(f *FakeMCP) Option {
	return func(h *Host) {
		WithService(extv1.CapMCPCall, extv1.MCPService(f))(h)
		h.mcpCall = f
	}
}
func WithMetrics(f *FakeMetrics) Option {
	return WithService(extv1.CapMetricsWrite, extv1.MetricsService(f))
}
func WithHostMetadata(f FakeHostMetadata) Option {
	return WithService(extv1.CapHostMetadata, extv1.HostMetadataService(f))
}

// ── secret.use ───────────────────────────────────────────────────────────────

// SecretBinding is one Host side secret.use binding: which secret value a ref
// resolves to, where it may be sent and in which header. The value exists only
// in the fake (the test plays the Host); the extension never receives it.
type SecretBinding struct {
	// Value is the plaintext the fake injects. Tests assert it never reaches logs.
	Value string
	// Origins are the allowed request origins ("https://api.example.com").
	Origins []string
	// Header receives the credential ("Authorization" by default).
	Header string
	// Scheme is prepended with a space ("Bearer" by default, "-" for none).
	Scheme string
}

// SecretUse is one recorded delegation. It holds no secret value.
type SecretUse struct {
	Ref    string
	Method string
	Origin string
	Status int
	Code   extv1.ErrorCode
}

// FakeSecretUse implements SecretUseService the way a Host must: unbound refs
// are NOT_FOUND, other origins PERMISSION_DENIED, caller credential headers are
// dropped, the credential goes only into the bound header, and redirects are
// never followed. Transport performs the upstream call (http.DefaultTransport
// when nil). Err fails every call with that error before anything is sent.
type FakeSecretUse struct {
	mu        sync.Mutex
	Bindings  map[string]SecretBinding
	Transport http.RoundTripper
	Err       error
	Uses      []SecretUse
}

var _ extv1.SecretUseService = (*FakeSecretUse)(nil)

// NewFakeSecretUse returns a fake with no bindings.
func NewFakeSecretUse() *FakeSecretUse {
	return &FakeSecretUse{Bindings: map[string]SecretBinding{}}
}

// Bind adds a binding and returns the fake for chaining.
func (f *FakeSecretUse) Bind(ref string, b SecretBinding) *FakeSecretUse {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Bindings == nil {
		f.Bindings = map[string]SecretBinding{}
	}
	f.Bindings[ref] = b
	return f
}

// History returns a copy of the recorded uses.
func (f *FakeSecretUse) History() []SecretUse {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SecretUse(nil), f.Uses...)
}

func (f *FakeSecretUse) record(u SecretUse) {
	f.mu.Lock()
	f.Uses = append(f.Uses, u)
	f.mu.Unlock()
}

func (f *FakeSecretUse) Do(ctx context.Context, ref extv1.SecretRef, req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, extv1.NewError(extv1.CodeInvalidArgument, "request required")
	}
	closeBody := func() {
		if req.Body != nil {
			_ = req.Body.Close()
		}
	}
	origin, err := extv1.SecretEgressOrigin(req.URL.String())
	if err != nil {
		closeBody()
		return nil, err
	}
	f.mu.Lock()
	injected, transport := f.Err, f.Transport
	binding, ok := f.Bindings[ref.Ref]
	f.mu.Unlock()
	use := SecretUse{Ref: ref.Ref, Method: req.Method, Origin: origin}
	fail := func(e error) (*http.Response, error) {
		closeBody()
		use.Code = extv1.CodeOf(e)
		f.record(use)
		return nil, e
	}
	if injected != nil {
		return fail(injected)
	}
	if !ok {
		return fail(extv1.NewError(extv1.CodeNotFound, "secret reference is not bound to this extension"))
	}
	allowed := false
	for _, o := range binding.Origins {
		if norm, err := extv1.SecretEgressOrigin(o); err == nil && norm == origin {
			allowed = true
		}
	}
	if !allowed {
		return fail(extv1.NewError(extv1.CodePermissionDenied, "origin is not allowed for this secret reference"))
	}
	header := binding.Header
	if header == "" {
		header = "Authorization"
	}
	value := binding.Value
	switch binding.Scheme {
	case "":
		value = "Bearer " + value
	case "-":
	default:
		value = binding.Scheme + " " + value
	}
	out := req.Clone(ctx)
	out.Header = extv1.SanitizeSecretEgressHeader(req.Header, header)
	out.Header.Set(header, value)
	out.Host = ""
	out.RequestURI = ""
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(out)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fail(ctxErr)
		}
		// The upstream error text is dropped: it could echo request details.
		return fail(extv1.NewError(extv1.CodeTemporaryFailure, "upstream request failed"))
	}
	use.Status = resp.StatusCode
	f.record(use)
	return resp, nil
}

// WithSecretUse grants secret.use backed by f.
func WithSecretUse(f *FakeSecretUse) Option {
	return func(h *Host) {
		if f == nil {
			return
		}
		WithService(extv1.CapSecretUse, extv1.SecretUseService(f))(h)
		h.SecretUse = f
	}
}

// AuthorizeRequestIdentity is a FakeMCP.Authorize for ProcessHost: it accepts
// contexts that carry the identity of a Host-proxied request.
func AuthorizeRequestIdentity(ctx context.Context) bool {
	id, ok := requestIdentity(ctx)
	return ok && strings.TrimSpace(id.UserID) != ""
}
