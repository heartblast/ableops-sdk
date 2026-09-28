package extserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// requestCtx is the context of a Host-proxied request (identity + request token).
func requestCtx(parent context.Context) context.Context {
	return withRequestToken(WithIdentity(parent, extv1.Identity{UserID: "alice"}), "lease-1")
}

func hostFor(t *testing.T, h http.HandlerFunc, timeout time.Duration, caps ...extv1.Capability) (extv1.HostContext, *bytes.Buffer) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var logs bytes.Buffer
	env := Environment{ExtensionID: "demo", HostURL: srv.URL, HostToken: "host-secret-token", HostAPIVersion: "v1", Capabilities: caps}
	host, _ := newHostContext(env, newLogger(&logs, "demo"), timeout)
	return host, &logs
}

func TestMCPReadGrantCannotCallTools(t *testing.T) {
	var hits atomic.Int32
	host, _ := hostFor(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"servers": []extv1.MCPServer{{ID: "one"}}})
	}, 0, extv1.CapMCPRead)
	if _, err := host.RequireMCPCall(); !errors.Is(err, extv1.ErrCapabilityUnavailable) {
		t.Fatalf("mcp.call must be unavailable: %v", err)
	}
	svc, err := host.RequireMCPRead()
	if err != nil {
		t.Fatal(err)
	}
	if servers, err := svc.ListMCPServers(requestCtx(context.Background())); err != nil || len(servers) != 1 {
		t.Fatalf("list: %v %v", servers, err)
	}
	_, err = svc.CallMCPTool(requestCtx(context.Background()), extv1.MCPToolCall{ServerID: "one", Name: "x"})
	if extv1.CodeOf(err) != extv1.CodeCapabilityUnavailable || hits.Load() != 1 {
		t.Fatalf("read-only call: code=%s hits=%d", extv1.CodeOf(err), hits.Load())
	}
}

func TestMCPRequiresRequestScopeBeforeNetwork(t *testing.T) {
	var hits atomic.Int32
	host, _ := hostFor(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }, 0, extv1.CapMCPRead, extv1.CapMCPCall)
	svc, _ := host.RequireMCPCall()
	identityOnly := WithIdentity(context.Background(), extv1.Identity{UserID: "alice"})
	for name, ctx := range map[string]context.Context{"background": context.Background(), "identity without token": identityOnly} {
		if _, err := svc.ListMCPServers(ctx); extv1.CodeOf(err) != extv1.CodePermissionDenied {
			t.Errorf("%s list: %v", name, err)
		}
		if _, err := svc.ListMCPTools(ctx, "one"); extv1.CodeOf(err) != extv1.CodePermissionDenied {
			t.Errorf("%s tools: %v", name, err)
		}
		if _, err := svc.CallMCPTool(ctx, extv1.MCPToolCall{ServerID: "one", Name: "x"}); extv1.CodeOf(err) != extv1.CodePermissionDenied {
			t.Errorf("%s call: %v", name, err)
		}
	}
	if _, err := svc.CallMCPTool(requestCtx(context.Background()), extv1.MCPToolCall{ServerID: "", Name: "x"}); extv1.CodeOf(err) != extv1.CodeInvalidArgument {
		t.Errorf("empty server: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("host was contacted %d times", hits.Load())
	}
}

func TestMCPCallTimeoutCancelAndErrors(t *testing.T) {
	release := make(chan struct{})
	serverSawCancel := make(chan struct{}, 4)
	host, logs := hostFor(t, func(w http.ResponseWriter, r *http.Request) {
		var call extv1.MCPToolCall
		_ = json.NewDecoder(r.Body).Decode(&call)
		switch call.Name {
		case "slow":
			select {
			case <-r.Context().Done():
				serverSawCancel <- struct{}{}
			case <-release:
			}
		case "long":
			// Longer than the Host client default (50ms), shorter than the caller deadline.
			time.Sleep(150 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(extv1.MCPToolResult{StructuredContent: json.RawMessage(`{"ok":true}`)})
		case "missing":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(extv1.NewError(extv1.CodeNotFound, "tool not found"))
		case "limited":
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(extv1.NewError(extv1.CodeRateLimited, "slow down"))
		case "failed":
			_ = json.NewEncoder(w).Encode(extv1.MCPToolResult{IsError: true, Content: json.RawMessage(`[{"type":"text","text":"boom"}]`)})
		}
	}, 50*time.Millisecond, extv1.CapMCPCall)
	defer close(release)
	svc, _ := host.RequireMCPCall()

	ctx, cancel := context.WithTimeout(requestCtx(context.Background()), 2*time.Second)
	defer cancel()
	if out, err := svc.CallMCPTool(ctx, extv1.MCPToolCall{ServerID: "s", Name: "long"}); err != nil || string(out.StructuredContent) != `{"ok":true}` {
		t.Fatalf("caller deadline must override the client default: %+v %v", out, err)
	}

	short, cancelShort := context.WithTimeout(requestCtx(context.Background()), 80*time.Millisecond)
	defer cancelShort()
	if _, err := svc.CallMCPTool(short, extv1.MCPToolCall{ServerID: "s", Name: "slow"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}

	cctx, cancelNow := context.WithCancel(requestCtx(context.Background()))
	go func() { time.Sleep(30 * time.Millisecond); cancelNow() }()
	if _, err := svc.CallMCPTool(cctx, extv1.MCPToolCall{ServerID: "s", Name: "slow"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-serverSawCancel:
		case <-time.After(2 * time.Second):
			t.Fatal("host request was not cancelled")
		}
	}

	// Without a caller deadline the client default bounds the call.
	if _, err := svc.CallMCPTool(requestCtx(context.Background()), extv1.MCPToolCall{ServerID: "s", Name: "slow"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("default deadline: %v", err)
	}

	_, err := svc.CallMCPTool(ctx, extv1.MCPToolCall{ServerID: "s", Name: "missing"})
	if HostErrorCode(err) != extv1.CodeNotFound || !errors.Is(err, extv1.NewError(extv1.CodeNotFound, "")) {
		t.Fatalf("not found mapping: %v", err)
	}
	if _, err := svc.CallMCPTool(ctx, extv1.MCPToolCall{ServerID: "s", Name: "limited"}); HostErrorCode(err) != extv1.CodeRateLimited {
		t.Fatalf("rate limit mapping: %v", err)
	}
	if out, err := svc.CallMCPTool(ctx, extv1.MCPToolCall{ServerID: "s", Name: "failed"}); err != nil || !out.IsError {
		t.Fatalf("tool failure is a result, not an error: %+v %v", out, err)
	}
	if strings.Contains(logs.String(), "host-secret-token") {
		t.Fatal("host token leaked to logs")
	}
}

func TestSecretUseDelegatesWithoutPlaintext(t *testing.T) {
	var got extv1.SecretEgressRequest
	var gotHeader http.Header
	host, logs := hostFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/secrets/egress" || r.Method != http.MethodPost {
			t.Errorf("path %s %s", r.Method, r.URL.Path)
		}
		gotHeader = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&got)
		switch {
		case strings.Contains(got.URL, "denied"):
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(extv1.NewError(extv1.CodePermissionDenied, "origin not allowed"))
		case strings.Contains(got.URL, "unbound"):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(extv1.NewError(extv1.CodeNotFound, "not bound"))
		default:
			w.Header().Set(HeaderEgressUpstream, "1")
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusUnauthorized) // upstream status passes through unchanged
			_, _ = io.WriteString(w, "data: one\n\ndata: two\n\n")
		}
	}, 0, extv1.CapSecretUse)
	svc, err := host.RequireSecretUse()
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: extv1.SecretTransport(svc, extv1.SecretRef{Ref: "llm/key"})}
	req, _ := http.NewRequest(http.MethodPost, "https://api.example.com/v1/chat", strings.NewReader(`{"q":1}`))
	req.Header.Set("Authorization", "Bearer extension-supplied")
	req.Header.Set("X-Ableops-User-Id", "mallory")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || string(body) != "data: one\n\ndata: two\n\n" || resp.Header.Get(HeaderEgressUpstream) != "" {
		t.Fatalf("upstream response: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	if got.Ref != "llm/key" || got.Method != http.MethodPost || got.URL != "https://api.example.com/v1/chat" || string(got.Body) != `{"q":1}` {
		t.Fatalf("egress request: %+v", got)
	}
	if got.Header.Get("Authorization") != "" || got.Header.Get("X-Ableops-User-Id") != "" || got.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("forwarded headers: %v", got.Header)
	}
	if gotHeader.Get("Authorization") != "Bearer host-secret-token" || gotHeader.Get(HeaderRequestToken) != "" {
		t.Fatalf("host auth headers: %v", gotHeader)
	}

	for url, code := range map[string]extv1.ErrorCode{"https://denied.example.com/": extv1.CodePermissionDenied, "https://unbound.example.com/": extv1.CodeNotFound} {
		r, _ := http.NewRequest(http.MethodGet, url, nil)
		_, err := client.Do(r)
		if HostErrorCode(err) != code || strings.Contains(err.Error(), "host-secret-token") {
			t.Errorf("%s: %v", url, err)
		}
	}
	for _, bad := range []string{"ftp://x.example.com/", "https://user:pw@x.example.com/"} {
		r, _ := http.NewRequest(http.MethodGet, bad, nil)
		if _, err := svc.Do(context.Background(), extv1.SecretRef{Ref: "llm/key"}, r); extv1.CodeOf(err) != extv1.CodeInvalidArgument {
			t.Errorf("%s: %v", bad, err)
		}
	}
	big, _ := http.NewRequest(http.MethodPost, "https://api.example.com/", bytes.NewReader(make([]byte, extv1.MaxSecretEgressRequestBytes+1)))
	if _, err := svc.Do(context.Background(), extv1.SecretRef{Ref: "llm/key"}, big); extv1.CodeOf(err) != extv1.CodeInvalidArgument {
		t.Errorf("oversized body: %v", err)
	}
	if strings.Contains(logs.String(), "host-secret-token") {
		t.Fatal("host token leaked to logs")
	}

	// Not granted: no service and the transport fails closed.
	none, _ := hostFor(t, func(http.ResponseWriter, *http.Request) {}, 0)
	if _, err := none.RequireSecretUse(); !errors.Is(err, extv1.ErrCapabilityUnavailable) {
		t.Fatalf("ungranted: %v", err)
	}
	r, _ := http.NewRequest(http.MethodGet, "https://api.example.com/", nil)
	if _, err := extv1.SecretTransport(nil, extv1.SecretRef{Ref: "x"}).RoundTrip(r); extv1.CodeOf(err) != extv1.CodeCapabilityUnavailable {
		t.Fatalf("nil transport: %v", err)
	}
}

func TestSecretUseOldHostWithoutMarkerIsIncompatible(t *testing.T) {
	host, _ := hostFor(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "{}") }, 0, extv1.CapSecretUse)
	svc, _ := host.RequireSecretUse()
	r, _ := http.NewRequest(http.MethodGet, "https://api.example.com/", nil)
	if _, err := svc.Do(context.Background(), extv1.SecretRef{Ref: "x"}, r); extv1.CodeOf(err) != extv1.CodeIncompatible {
		t.Fatalf("unmarked 2xx must not be taken as upstream: %v", err)
	}
}
