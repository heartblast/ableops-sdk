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
	// Description is optional display text. It never carries connection or credential data.
	Description string `json:"description,omitempty"`
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	// Annotations are the MCP tool hints the Host exposes (nil when unknown).
	// They are hints for extension policy, not a grant: the Host still authorizes every call.
	Annotations *MCPToolAnnotations `json:"annotations,omitempty"`
}

// MCPToolAnnotations mirrors the MCP tool annotations. A nil hint means unknown;
// an extension that gates tools by these hints must treat unknown as unsafe.
type MCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
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
//
// Request scope: pass the context of the Host-proxied request being served (or a
// context derived from it). Background contexts carry no principal; the process
// client rejects them with PERMISSION_DENIED before contacting the Host.
//
// Capabilities: mcp.read allows ListMCPServers and ListMCPTools only. A service
// obtained through RequireMCPRead answers CallMCPTool with CAPABILITY_UNAVAILABLE
// unless mcp.call is also granted; the Host enforces the same rule.
//
// Timeout and cancel: the context deadline bounds each call. Without a deadline
// the Host client default applies. Cancelling the context aborts the Host request,
// and the Host cancels the tool execution it started for it. Such errors satisfy
// errors.Is(err, context.Canceled) or errors.Is(err, context.DeadlineExceeded).
//
// Errors: Host refusals carry SDK error codes (PERMISSION_DENIED, NOT_FOUND,
// INVALID_ARGUMENT, RATE_LIMITED, HOST_UNAVAILABLE, TEMPORARY_FAILURE). A tool
// that ran and failed is not an error: it returns MCPToolResult.IsError = true.
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
