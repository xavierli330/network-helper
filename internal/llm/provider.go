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

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatRequest is the input to a chat completion.
type ChatRequest struct {
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// ChatResponse is the output from a chat completion.
type ChatResponse struct {
	Content string `json:"content"`
}

// Provider is the interface for LLM backends.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Supports(cap Capability) bool
}
