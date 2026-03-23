package llm

import "context"

// Capability represents what an LLM provider can do.
type Capability string

const (
	CapExtract Capability = "extract" // structured extraction from logs
	CapAnalyze Capability = "analyze" // troubleshooting analysis
	CapExplain Capability = "explain" // config/output explanation
	CapParse   Capability = "parse"   // parser fallback (L2.5)
)

// ToolDef defines a tool that the LLM can call.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema object
}

// ToolCall represents an LLM's request to invoke a tool.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`                      // "system", "user", "assistant", "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`       // for assistant messages with tool calls
	ToolCallID string     `json:"tool_call_id,omitempty"`     // for tool result messages
	Name       string     `json:"name,omitempty"`             // tool name for tool results
}

// ChatRequest is the input to a chat completion.
type ChatRequest struct {
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Tools       []ToolDef `json:"tools,omitempty"`
}

// ChatResponse is the output from a chat completion.
type ChatResponse struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // populated when StopReason == "tool_use"
	StopReason string     `json:"stop_reason,omitempty"` // "tool_use", "end_turn", or ""
}

// Provider is the interface for LLM backends.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Supports(cap Capability) bool
}
