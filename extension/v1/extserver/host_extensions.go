package extserver

import (
	"context"
	"net/http"
	"net/url"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

type mcpService struct{ c *hostClient }

func (s mcpService) ListMCPServers(ctx context.Context) ([]extv1.MCPServer, error) {
	var out struct {
		Servers []extv1.MCPServer `json:"servers"`
	}
	err := s.c.get(ctx, "/mcp/servers", nil, &out)
	return out.Servers, err
}

func (s mcpService) ListMCPTools(ctx context.Context, serverID string) ([]extv1.MCPTool, error) {
	var out struct {
		Tools []extv1.MCPTool `json:"tools"`
	}
	err := s.c.get(ctx, "/mcp/tools", url.Values{"serverId": {serverID}}, &out)
	return out.Tools, err
}

func (s mcpService) CallMCPTool(ctx context.Context, call extv1.MCPToolCall) (extv1.MCPToolResult, error) {
	var out extv1.MCPToolResult
	err := s.c.do(ctx, http.MethodPost, "/mcp/call", nil, call, &out)
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
