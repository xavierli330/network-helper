package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/xavierli/nethelper/internal/config"
)

// Embedder generates vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// OllamaEmbedder uses Ollama's /api/embeddings endpoint.
type OllamaEmbedder struct {
	BaseURL string
	Model   string
	client  *http.Client
}

// NewOllamaEmbedder creates an OllamaEmbedder with default HTTP client.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		BaseURL: baseURL,
		Model:   model,
		client:  &http.Client{},
	}
}

// Embed calls Ollama to generate an embedding vector for text.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]string{"model": e.Model, "prompt": text}
	data, _ := json.Marshal(reqBody)

	url := e.BaseURL + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse embedding response: %w", err)
	}
	return result.Embedding, nil
}

// BuildEmbedder creates an Embedder from the embedding section of the config.
// Returns nil if embedding is not configured or the provider is unknown.
func BuildEmbedder(cfg config.EmbeddingConfig) Embedder {
	if cfg.Provider == "" {
		return nil
	}
	provCfg, ok := cfg.Providers[cfg.Provider]
	if !ok {
		return nil
	}

	switch cfg.Provider {
	case "ollama":
		baseURL := provCfg.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		return NewOllamaEmbedder(baseURL, provCfg.Model)
	default:
		return nil
	}
}
