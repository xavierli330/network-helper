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

// Anthropic Messages API request format
type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Anthropic Messages API response format
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// Separate system message from user/assistant messages
	var systemPrompt string
	var messages []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		} else {
			messages = append(messages, anthropicMessage{Role: m.Role, Content: m.Content})
		}
	}

	if len(messages) == 0 {
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
		Messages:    messages,
		System:      systemPrompt,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Stream:      false,
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

	// Concatenate all text blocks
	var text string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return ChatResponse{Content: text}, nil
}
