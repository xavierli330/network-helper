package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPKnowledgeSource searches an external HTTP API.
// The API is expected to accept a POST JSON body {"query": "...", "top_k": N}
// and return {"results": [{"title": "...", "content": "...", "score": 0.9, "url": "..."}]}.
type HTTPKnowledgeSource struct {
	sourceName string
	baseURL    string // e.g. "https://iwiki.example.com/api" (no trailing slash)
	token      string // Bearer token for auth; empty means no Authorization header
	client     *http.Client
}

// NewHTTPKnowledgeSource constructs a new HTTP-backed knowledge source.
// name is used as the display prefix in SearchResult.Source (e.g. "iwiki").
// baseURL should not include a trailing slash.
// token is optional; when non-empty it is sent as "Bearer <token>".
func NewHTTPKnowledgeSource(name, baseURL, token string) *HTTPKnowledgeSource {
	return &HTTPKnowledgeSource{
		sourceName: name,
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *HTTPKnowledgeSource) Name() string { return s.sourceName }

func (s *HTTPKnowledgeSource) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// Build request body.
	reqBody, err := json.Marshal(map[string]interface{}{
		"query": query,
		"top_k": topK,
	})
	if err != nil {
		return nil, fmt.Errorf("%s marshal request: %w", s.sourceName, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/search", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("%s build request: %w", s.sourceName, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", s.sourceName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s search HTTP %d: %s", s.sourceName, resp.StatusCode, body)
	}

	// Flexible response format.
	var apiResp struct {
		Results []struct {
			Title   string  `json:"title"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
			URL     string  `json:"url"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("%s decode response: %w", s.sourceName, err)
	}

	results := make([]SearchResult, 0, len(apiResp.Results))
	for _, r := range apiResp.Results {
		source := s.sourceName
		if r.URL != "" {
			source = s.sourceName + ":" + r.URL
		}
		results = append(results, SearchResult{
			Source:  source,
			Title:   r.Title,
			Content: r.Content,
			Score:   r.Score,
		})
	}
	return results, nil
}
