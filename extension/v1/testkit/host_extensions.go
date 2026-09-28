package testkit

import (
	"context"
	"sync"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

// FakeMCP records calls and returns a fixed inventory. It requires a request
// context marker supplied by the test so background calls fail closed.
type FakeMCP struct {
	mu        sync.Mutex
	Servers   []extv1.MCPServer
	Tools     map[string][]extv1.MCPTool
	Calls     []extv1.MCPToolCall
	Result    extv1.MCPToolResult
	Authorize func(context.Context) bool
}

func (f *FakeMCP) allowed(ctx context.Context) bool { return f.Authorize != nil && f.Authorize(ctx) }
func (f *FakeMCP) ListMCPServers(ctx context.Context) ([]extv1.MCPServer, error) {
	if !f.allowed(ctx) {
		return nil, extv1.NewError(extv1.CodePermissionDenied, "request principal required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.MCPServer(nil), f.Servers...), nil
}
func (f *FakeMCP) ListMCPTools(ctx context.Context, id string) ([]extv1.MCPTool, error) {
	if !f.allowed(ctx) {
		return nil, extv1.NewError(extv1.CodePermissionDenied, "request principal required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.MCPTool(nil), f.Tools[id]...), nil
}
func (f *FakeMCP) CallMCPTool(ctx context.Context, call extv1.MCPToolCall) (extv1.MCPToolResult, error) {
	if !f.allowed(ctx) {
		return extv1.MCPToolResult{}, extv1.NewError(extv1.CodePermissionDenied, "request principal required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, call)
	return f.Result, nil
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

func WithMCPRead(f *FakeMCP) Option { return WithService(extv1.CapMCPRead, extv1.MCPService(f)) }
func WithMCPCall(f *FakeMCP) Option { return WithService(extv1.CapMCPCall, extv1.MCPService(f)) }
func WithMetrics(f *FakeMetrics) Option {
	return WithService(extv1.CapMetricsWrite, extv1.MetricsService(f))
}
func WithHostMetadata(f FakeHostMetadata) Option {
	return WithService(extv1.CapHostMetadata, extv1.HostMetadataService(f))
}
