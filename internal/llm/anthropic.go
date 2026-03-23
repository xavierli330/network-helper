package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultAnthropicBaseURL = "https://api.anthropic.com"

// AnthropicProvider implements Provider for Anthropic-protocol APIs.
// Works with: Anthropic Claude, Kimi Coding (which uses Anthropic protocol).
type AnthropicProvider struct {
	name    string
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewAnthropicProvider creates a provider for the official Anthropic API.
func NewAnthropicProvider(apiKey, model, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &AnthropicProvider{name: "anthropic", apiKey: apiKey, model: model, baseURL: baseURL, client: &http.Client{}}
}

// NewAnthropicCompatProvider creates a provider for any Anthropic-compatible API (e.g., Kimi Coding).
func NewAnthropicCompatProvider(name, apiKey, model, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	return &AnthropicProvider{name: name, apiKey: apiKey, model: model, baseURL: baseURL, client: &http.Client{}}
}

func (p *AnthropicProvider) Name() string { return p.name }

func (p *AnthropicProvider) Supports(cap Capability) bool {
	return true
}

// anthropicTool describes a callable tool in the Anthropic request format.
type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// anthropicContentBlock represents a single item in an Anthropic content array.
// It covers text blocks, tool_use blocks (response), and tool_result blocks (request).
type anthropicContentBlock struct {
	Type string `json:"type"`

	// type="text"
	Text string `json:"text,omitempty"`

	// type="tool_use" (response from model)
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`

	// type="tool_result" (sent back by the caller)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

// anthropicMessage is a single message in the Anthropic Messages API.
// Content is interface{} so we can send either a plain string or a content-block array.
type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// Anthropic Messages API request format
type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
}

// Anthropic Messages API response format
type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// toAnthropicMessages converts our unified Message slice to the Anthropic wire format.
// Key differences from OpenAI:
//   - Tool result messages use role="user" with a tool_result content block.
//   - Assistant messages with tool calls carry a tool_use content block array.
func toAnthropicMessages(msgs []Message) []anthropicMessage {
	result := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		switch {
		case m.Role == "tool":
			// Tool result: Anthropic expects role="user" + tool_result content block.
			block := anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: []anthropicContentBlock{block},
			})

		case len(m.ToolCalls) > 0:
			// Assistant message requesting tool calls.
			blocks := make([]anthropicContentBlock, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			result = append(result, anthropicMessage{Role: "assistant", Content: blocks})

		default:
			result = append(result, anthropicMessage{Role: m.Role, Content: m.Content})
		}
	}
	return result
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// Separate system message from user/assistant messages
	var systemPrompt string
	var nonSystemMsgs []Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		} else {
			nonSystemMsgs = append(nonSystemMsgs, m)
		}
	}

	if len(nonSystemMsgs) == 0 {
		return ChatResponse{}, fmt.Errorf("no user messages provided")
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.3
	}

	apiReq := anthropicRequest{
		Model:       p.model,
		Messages:    toAnthropicMessages(nonSystemMsgs),
		System:      systemPrompt,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Stream:      false,
	}

	// Map ToolDef → anthropicTool
	if len(req.Tools) > 0 {
		apiReq.Tools = make([]anthropicTool, len(req.Tools))
		for i, t := range req.Tools {
			apiReq.Tools[i] = anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.Parameters,
			}
		}
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	// Anthropic uses /v1/messages endpoint
	endpoint := p.baseURL + "/v1/messages"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read response: %w", err)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return ChatResponse{}, fmt.Errorf("unmarshal response (status %d): %w\nbody: %s", resp.StatusCode, err, string(respBody))
	}

	if apiResp.Error != nil {
		return ChatResponse{}, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Content) == 0 {
		return ChatResponse{}, fmt.Errorf("no content in response")
	}

	chatResp := ChatResponse{
		StopReason: apiResp.StopReason,
	}

	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			chatResp.Content += block.Text
		case "tool_use":
			// Normalise stop_reason to our unified value.
			chatResp.StopReason = "tool_use"
			chatResp.ToolCalls = append(chatResp.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return chatResp, nil
}
