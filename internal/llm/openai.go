package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// OpenAIProvider implements Provider for any OpenAI-compatible API.
// Works with: OpenAI, Ollama (/v1 endpoint), DeepSeek, Groq, Together, etc.
type OpenAIProvider struct {
	name    string
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAIProvider creates a provider for the official OpenAI API.
func NewOpenAIProvider(apiKey, model, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIProvider{name: "openai", apiKey: apiKey, model: model, baseURL: baseURL, client: &http.Client{}}
}

// NewOpenAICompatProvider creates a provider for any OpenAI-compatible API.
func NewOpenAICompatProvider(name, apiKey, model, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	// Ollama uses /v1 path
	if name == "ollama" && baseURL == "http://localhost:11434" {
		baseURL = "http://localhost:11434/v1"
	}
	return &OpenAIProvider{name: name, apiKey: apiKey, model: model, baseURL: baseURL, client: &http.Client{}}
}

func (p *OpenAIProvider) Name() string { return p.name }

func (p *OpenAIProvider) Supports(cap Capability) bool {
	return true // OpenAI-compatible APIs support all capabilities
}

// openAIMessage is the wire format for a single chat message.
// Content is a pointer to allow null (required when tool_calls are present).
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`                // nullable when tool_calls are present
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`   // for assistant messages with tool calls
	ToolCallID string           `json:"tool_call_id,omitempty"` // for role="tool" result messages
	Name       string           `json:"name,omitempty"`
}

// openAIToolCall is a single tool invocation in an assistant message.
type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON-encoded string, not an object
	} `json:"function"`
}

// openAITool describes a callable tool in the request.
type openAITool struct {
	Type     string         `json:"type"` // always "function"
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content   *string          `json:"content"`
			ToolCalls []openAIToolCall  `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// toOpenAIMessages converts our unified Message slice to the OpenAI wire format,
// handling nullable content and tool call / tool result shapes.
func toOpenAIMessages(msgs []Message) []openAIMessage {
	result := make([]openAIMessage, 0, len(msgs))
	for _, m := range msgs {
		om := openAIMessage{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		switch {
		case m.Role == "tool":
			// Tool result message: content is the result text.
			om.Content = &m.Content
		case len(m.ToolCalls) > 0:
			// Assistant message requesting tool calls; content may be empty/null.
			if m.Content != "" {
				om.Content = &m.Content
			}
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				om.ToolCalls = append(om.ToolCalls, openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		default:
			om.Content = &m.Content
		}
		result = append(result, om)
	}
	return result
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	apiReq := openAIRequest{
		Model:       p.model,
		Messages:    toOpenAIMessages(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if apiReq.Temperature == 0 {
		apiReq.Temperature = 0.3
	}
	if apiReq.MaxTokens == 0 {
		apiReq.MaxTokens = 2048
	}

	// Map ToolDef → openAITool
	if len(req.Tools) > 0 {
		apiReq.Tools = make([]openAITool, len(req.Tools))
		for i, t := range req.Tools {
			apiReq.Tools[i] = openAITool{
				Type: "function",
				Function: openAIFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			}
		}
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read response: %w", err)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return ChatResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return ChatResponse{}, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("no choices in response")
	}

	choice := apiResp.Choices[0]
	chatResp := ChatResponse{}

	if choice.Message.Content != nil {
		chatResp.Content = *choice.Message.Content
	}

	if len(choice.Message.ToolCalls) > 0 {
		chatResp.StopReason = "tool_use"
		for _, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			// Arguments is a JSON string; unmarshal it into a map.
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			chatResp.ToolCalls = append(chatResp.ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
	}

	return chatResp, nil
}
