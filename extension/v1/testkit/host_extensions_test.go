package testkit

import (
	"context"
	"errors"
	"testing"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

func TestHostExtensionFakesRequireExplicitGrantAndPrincipal(t *testing.T) {
	mcp := &FakeMCP{Servers: []extv1.MCPServer{{ID: "one"}}, Authorize: func(ctx context.Context) bool { return ctx.Value(principalKey{}) == "alice" }}
	metrics := &FakeMetrics{}
	host := NewHost("demo", WithMCPRead(mcp), WithMetrics(metrics), WithHostMetadata(FakeHostMetadata{Value: extv1.HostMetadata{ExtensionID: "demo"}})).Context()
	if _, err := host.RequireMCPCall(); !errors.Is(err, extv1.ErrCapabilityUnavailable) {
		t.Fatalf("ungranted call: %v", err)
	}
	read, err := host.RequireMCPRead()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read.ListMCPServers(context.Background()); err == nil {
		t.Fatal("background request was authorized")
	}
	if servers, err := read.ListMCPServers(context.WithValue(context.Background(), principalKey{}, "alice")); err != nil || len(servers) != 1 {
		t.Fatalf("authorized list: %v %v", servers, err)
	}
	metricSvc, err := host.RequireMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if err := metricSvc.RecordMetric(context.Background(), extv1.Metric{Name: "jobs", Kind: extv1.MetricCounter, Value: 1}); err != nil || len(metrics.Records) != 1 {
		t.Fatalf("metric: %v %v", metrics.Records, err)
	}
}

type principalKey struct{}
