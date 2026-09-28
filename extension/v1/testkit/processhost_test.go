package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
	"github.com/heartblast/ableops-sdk/extension/v1/extserver"
)

// TestMain keeps stdin open: extserver stops the extension when stdin closes
// (the Host's stop signal), and the test runner's stdin may already be closed.
func TestMain(m *testing.M) {
	if r, w, err := os.Pipe(); err == nil {
		os.Stdin = r
		keepStdin = w
	}
	os.Exit(m.Run())
}

var keepStdin *os.File

// probe is a minimal process extension whose routes exercise MCP and secret.use.
type probe struct {
	mu       sync.Mutex
	mcp      extv1.MCPService
	secret   extv1.SecretUseService
	upstream string
	leaked   chan context.Context
}

func (p *probe) Manifest() extv1.Manifest {
	return extv1.Manifest{
		APIVersion: extv1.APIVersion, ID: "probe", Name: "Probe", Version: "0.1.0",
		Capabilities: []extv1.Capability{extv1.CapMCPRead, extv1.CapMCPCall, extv1.CapSecretUse, extv1.CapAuditWrite},
		Backend:      extv1.BackendDecl{Enabled: true, Kind: extv1.BackendKindProcess},
	}
}

func (p *probe) Start(_ context.Context, host extv1.HostContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mcp, _ = host.RequireMCPCall()
	p.secret, _ = host.RequireSecretUse()
	return nil
}
func (p *probe) Stop(context.Context) error { return nil }
func (p *probe) Health(context.Context) extv1.Health {
	return extv1.Health{OK: true, CheckedAt: time.Now()}
}

func (p *probe) Routes() []extv1.RouteHandler {
	fail := func(w http.ResponseWriter, err error) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": string(extserver.HostErrorCode(err)), "error": err.Error(), "deadline": boolText(errors.Is(err, context.DeadlineExceeded))})
	}
	return []extv1.RouteHandler{
		{Method: http.MethodPost, Pattern: "/mcp", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var call extv1.MCPToolCall
			_ = json.NewDecoder(r.Body).Decode(&call)
			ctx := r.Context()
			if call.Name == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			servers, err := p.mcp.ListMCPServers(ctx)
			if err != nil {
				fail(w, err)
				return
			}
			tools, err := p.mcp.ListMCPTools(ctx, servers[0].ID)
			if err != nil {
				fail(w, err)
				return
			}
			call.ServerID = servers[0].ID
			if call.Name == "" {
				call.Name = tools[0].Name
			}
			out, err := p.mcp.CallMCPTool(ctx, call)
			if err != nil {
				fail(w, err)
				return
			}
			if p.leaked != nil {
				p.leaked <- r.Context()
			}
			_ = json.NewEncoder(w).Encode(out)
		})},
		{Method: http.MethodPost, Pattern: "/llm", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := r.URL.Query().Get("url")
			client := &http.Client{Transport: extv1.SecretTransport(p.secret, extv1.SecretRef{Ref: r.URL.Query().Get("ref")})}
			req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, target, strings.NewReader(`{"prompt":"hi"}`))
			req.Header.Set("Authorization", "Bearer forged-by-extension")
			resp, err := client.Do(req)
			if err != nil {
				fail(w, err)
				return
			}
			defer resp.Body.Close()
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, resp.Body)
		})},
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// startInProcess runs ext through extserver.RunContext with the ProcessHost env.
func startInProcess(t *testing.T, ph *ProcessHost, ext extv1.Extension) *lockedBuffer {
	t.Helper()
	for _, kv := range ph.Env() {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	pr, pw := io.Pipe()
	logs := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- extserver.RunContext(ctx, ext, extserver.WithStdout(pw), extserver.WithLogWriter(logs))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("extension did not stop")
		}
		_ = pw.Close()
	})
	wctx, wcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer wcancel()
	if _, err := ph.WaitReady(wctx, pr); err != nil {
		t.Fatalf("handshake: %v\n%s", err, logs.String())
	}
	return logs
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func TestProcessHostMCPRequestScope(t *testing.T) {
	block := make(chan struct{})
	mcp := &FakeMCP{
		Servers:   []extv1.MCPServer{{ID: "gw"}},
		Tools:     map[string][]extv1.MCPTool{"gw": {{Name: "read_topics"}}},
		Result:    extv1.MCPToolResult{StructuredContent: json.RawMessage(`{"ok":true}`)},
		Authorize: AuthorizeRequestIdentity,
		CallFunc: func(ctx context.Context, call extv1.MCPToolCall) (extv1.MCPToolResult, error) {
			if id, _ := requestIdentity(ctx); id.UserID != "alice" {
				return extv1.MCPToolResult{}, extv1.NewError(extv1.CodePermissionDenied, "wrong principal")
			}
			switch call.Name {
			case "timeout":
				select {
				case <-ctx.Done():
					close(block)
					return extv1.MCPToolResult{}, ctx.Err()
				case <-time.After(5 * time.Second):
				}
			case "missing":
				return extv1.MCPToolResult{}, extv1.NewError(extv1.CodeNotFound, "no such tool")
			}
			return extv1.MCPToolResult{StructuredContent: json.RawMessage(`{"ok":true}`)}, nil
		},
	}
	audit := NewFakeAudit()
	ph := NewProcessHost(NewHost("probe", WithMCPRead(mcp), WithMCPCall(mcp), WithAudit(audit)), WithProcessVersion("0.1.0"))
	defer ph.Close()
	ext := &probe{leaked: make(chan context.Context, 1)}
	startInProcess(t, ph, ext)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	alice := extv1.Identity{UserID: "alice", Roles: []string{"admin"}}
	status, body, err := ph.Invoke(ctx, alice, http.MethodPost, "/mcp", []byte(`{}`))
	if err != nil || status != http.StatusOK || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("call: %d %s %v", status, body, err)
	}
	if calls := mcp.CallHistory(); len(calls) != 1 || calls[0].ServerID != "gw" || calls[0].Name != "read_topics" {
		t.Fatalf("calls: %+v", calls)
	}

	// The request context outlives the lease: a later call with it is refused.
	stale := <-ext.leaked
	if _, err := ext.mcp.ListMCPServers(context.WithoutCancel(stale)); extserver.HostErrorCode(err) != extv1.CodePermissionDenied {
		t.Fatalf("stale lease: %v", err)
	}

	// Host side error mapping and the extension's own deadline.
	status, body, _ = ph.Invoke(ctx, alice, http.MethodPost, "/mcp", []byte(`{"name":"missing"}`))
	if status != http.StatusBadGateway || !strings.Contains(string(body), `"code":"NOT_FOUND"`) {
		t.Fatalf("not found: %d %s", status, body)
	}
	status, body, _ = ph.Invoke(ctx, alice, http.MethodPost, "/mcp", []byte(`{"name":"timeout"}`))
	if status != http.StatusBadGateway || !strings.Contains(string(body), `"deadline":"true"`) {
		t.Fatalf("timeout: %d %s", status, body)
	}
	select {
	case <-block:
	case <-time.After(3 * time.Second):
		t.Fatal("Host did not cancel the tool when the extension's deadline expired")
	}

	// A forged request token never reaches the service.
	req, _ := http.NewRequest(http.MethodGet, ph.URL()+"/v1/mcp/servers", nil)
	for _, kv := range ph.Env() {
		if v, ok := strings.CutPrefix(kv, extserver.EnvHostToken+"="); ok {
			req.Header.Set("Authorization", "Bearer "+v)
		}
	}
	req.Header.Set(extserver.HeaderExtensionID, "probe")
	req.Header.Set(extserver.HeaderOnBehalfOf, "alice")
	req.Header.Set(extserver.HeaderRequestToken, "forged")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forged token: %v %v", resp, err)
	}
	_ = resp.Body.Close()
	if d := ph.Denials(); len(d) < 2 || d[len(d)-1] != "principal" {
		t.Fatalf("denials: %v", d)
	}
}

func TestProcessHostSecretUseKeepsPlaintextInHost(t *testing.T) {
	const secret = "sk-live-0123456789"
	var seenAuth []string
	var mu sync.Mutex
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenAuth = append(seenAuth, r.Header.Values("Authorization")...)
		mu.Unlock()
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://evil.example.com/steal", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: hello\n\n")
	}))
	defer upstream.Close()
	uses := NewFakeSecretUse().Bind("llm/key", SecretBinding{Value: secret, Origins: []string{upstream.URL}})
	ph := NewProcessHost(NewHost("probe", WithSecretUse(uses)))
	defer ph.Close()
	logs := startInProcess(t, ph, &probe{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	alice := extv1.Identity{UserID: "alice"}
	status, body, err := ph.Invoke(ctx, alice, http.MethodPost, "/llm?ref=llm/key&url="+upstream.URL+"/v1/chat", nil)
	if err != nil || status != http.StatusOK || string(body) != "data: hello\n\n" {
		t.Fatalf("egress: %d %q %v", status, body, err)
	}
	mu.Lock()
	if len(seenAuth) != 1 || seenAuth[0] != "Bearer "+secret {
		t.Fatalf("upstream Authorization: %q", seenAuth)
	}
	mu.Unlock()

	// The Host returns the upstream 302 instead of following it. The extension's
	// http.Client follows it through the transport, and the Host refuses the new origin.
	status, body, _ = ph.Invoke(ctx, alice, http.MethodPost, "/llm?ref=llm/key&url="+upstream.URL+"/redirect", nil)
	if !strings.Contains(string(body), `"code":"PERMISSION_DENIED"`) {
		t.Fatalf("redirect to another origin: %d %s", status, body)
	}
	if h := uses.History(); h[len(h)-1].Origin != "https://evil.example.com" || h[len(h)-1].Code != extv1.CodePermissionDenied {
		t.Fatalf("redirect hop: %+v", h[len(h)-1])
	}
	status, body, _ = ph.Invoke(ctx, alice, http.MethodPost, "/llm?ref=llm/key&url=https://other.example.com/", nil)
	if !strings.Contains(string(body), `"code":"PERMISSION_DENIED"`) {
		t.Fatalf("other origin: %d %s", status, body)
	}
	status, body, _ = ph.Invoke(ctx, alice, http.MethodPost, "/llm?ref=llm/unknown&url="+upstream.URL, nil)
	if !strings.Contains(string(body), `"code":"NOT_FOUND"`) {
		t.Fatalf("unbound ref: %d %s", status, body)
	}
	uses.mu.Lock()
	uses.Err = extv1.NewError(extv1.CodeHostUnavailable, "secret store unavailable")
	uses.mu.Unlock()
	status, body, _ = ph.Invoke(ctx, alice, http.MethodPost, "/llm?ref=llm/key&url="+upstream.URL, nil)
	if !strings.Contains(string(body), `"code":"HOST_UNAVAILABLE"`) {
		t.Fatalf("store failure: %d %s", status, body)
	}
	for _, u := range uses.History() {
		if strings.Contains(u.Ref+u.Origin+string(u.Code), secret) {
			t.Fatal("secret in use history")
		}
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(string(body), secret) {
		t.Fatal("secret reached extension logs or responses")
	}
}

func TestProcessHostDeniesUngrantedCapabilities(t *testing.T) {
	mcp := &FakeMCP{Servers: []extv1.MCPServer{{ID: "gw"}}, Tools: map[string][]extv1.MCPTool{"gw": {{Name: "t"}}}, Authorize: AuthorizeRequestIdentity}
	// mcp.read only: listing works, the call is refused by the SDK client before the Host.
	ph := NewProcessHost(NewHost("probe", WithMCPRead(mcp)))
	defer ph.Close()
	ext := &probe{}
	startInProcess(t, ph, ext)
	if ext.mcp != nil {
		t.Fatal("mcp.call must not be available")
	}
	if got := ph.Granted(); len(got) != 1 || got[0] != extv1.CapMCPRead {
		t.Fatalf("granted: %v", got)
	}
}
