package extensionv1

import (
	"context"
	"encoding/json"
)

// MCPServer and MCPTool describe the host's currently exposed MCP surface.
// Tool names are scoped to a server; callers must pass both identifiers.
type MCPServer struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type MCPToolCall struct {
	ServerID  string          `json:"serverId"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type MCPToolResult struct {
	Content           json.RawMessage `json:"content,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError"`
}

// MCPService uses the host's existing exposure, policy, execution and audit path.
// Calls require a live request principal. Extension identity alone never grants access.
type MCPService interface {
	ListMCPServers(context.Context) ([]MCPServer, error)
	ListMCPTools(context.Context, string) ([]MCPTool, error)
	CallMCPTool(context.Context, MCPToolCall) (MCPToolResult, error)
}

type MetricKind string

const (
	MetricCounter  MetricKind = "counter"
	MetricDuration MetricKind = "duration"
)

// Metric has a bounded name chosen by the extension. No arbitrary labels are accepted.
type Metric struct {
	Name  string     `json:"name"`
	Kind  MetricKind `json:"kind"`
	Value float64    `json:"value"`
}

type MetricsService interface {
	RecordMetric(context.Context, Metric) error
}

type HostMetadata struct {
	HostVersion         string       `json:"hostVersion"`
	APIVersion          string       `json:"apiVersion"`
	ProtocolVersion     int          `json:"protocolVersion"`
	ExtensionID         string       `json:"extensionId"`
	GrantedCapabilities []Capability `json:"grantedCapabilities"`
}

type HostMetadataService interface {
	GetHostMetadata(context.Context) (HostMetadata, error)
}

func (h HostContext) RequireMCPRead() (MCPService, error) {
	return RequireService[MCPService](h, CapMCPRead)
}
func (h HostContext) RequireMCPCall() (MCPService, error) {
	return RequireService[MCPService](h, CapMCPCall)
}
func (h HostContext) RequireMetrics() (MetricsService, error) {
	return RequireService[MetricsService](h, CapMetricsWrite)
}
func (h HostContext) RequireHostMetadata() (HostMetadataService, error) {
	return RequireService[HostMetadataService](h, CapHostMetadata)
}
