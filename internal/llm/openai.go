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

type openAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	apiReq := openAIRequest{
		Model:       p.model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if apiReq.Temperature == 0 {
		apiReq.Temperature = 0.3
	}
	if apiReq.MaxTokens == 0 {
		apiReq.MaxTokens = 2048
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

	return ChatResponse{Content: apiResp.Choices[0].Message.Content}, nil
}
