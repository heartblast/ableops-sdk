package extserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

func TestHostExtensionServicesCarryRequestCredential(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer host-secret" || r.Header.Get(HeaderExtensionID) != "demo" {
			t.Errorf("host auth headers")
		}
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/mcp/call" {
			if r.Header.Get(HeaderOnBehalfOf) != "alice" || r.Header.Get(HeaderRequestToken) != "lease" {
				t.Errorf("missing request credential")
			}
			_ = json.NewEncoder(w).Encode(extv1.MCPToolResult{StructuredContent: json.RawMessage(`{"ok":true}`)})
			return
		}
		if r.Header.Get(HeaderRequestToken) != "" {
			t.Errorf("background credential leaked")
		}
		switch r.URL.Path {
		case "/v1/metadata":
			_ = json.NewEncoder(w).Encode(extv1.HostMetadata{ExtensionID: "demo"})
		case "/v1/metrics":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	env := Environment{ExtensionID: "demo", HostURL: srv.URL, HostToken: "host-secret", HostAPIVersion: "v1", Capabilities: []extv1.Capability{extv1.CapMCPCall, extv1.CapMetricsWrite, extv1.CapHostMetadata}}
	host, _ := newHostContext(env, nil, 0)
	meta, err := host.RequireHostMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if value, err := meta.GetHostMetadata(context.Background()); err != nil || value.ExtensionID != "demo" {
		t.Fatalf("metadata: %+v %v", value, err)
	}
	metrics, err := host.RequireMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if err := metrics.RecordMetric(context.Background(), extv1.Metric{Name: "jobs", Kind: extv1.MetricCounter, Value: 1}); err != nil {
		t.Fatal(err)
	}
	mcp, err := host.RequireMCPCall()
	if err != nil {
		t.Fatal(err)
	}
	ctx := withRequestToken(WithIdentity(context.Background(), extv1.Identity{UserID: "alice"}), "lease")
	if _, err := mcp.CallMCPTool(ctx, extv1.MCPToolCall{ServerID: "srv", Name: "read"}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("paths: %v", paths)
	}
}
