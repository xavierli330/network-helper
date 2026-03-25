package memory

import (
	"context"
	"fmt"
	"time"

	ima "github.com/xavierli330/ima-sdk-go"
)

// IMAKnowledgeSource searches IMA (Tencent Intelligent Medical Assistant) knowledge bases.
type IMAKnowledgeSource struct {
	client *ima.Client
	kbID   string
	name   string
}

// NewIMAKnowledgeSource creates a new IMA-backed knowledge source.
// clientID and apiKey are IMA OpenAPI credentials.
// kbID is the knowledge base ID to search within.
// name is the display name used in SearchResult.Source (e.g., "ima").
func NewIMAKnowledgeSource(clientID, apiKey, kbID, name string) *IMAKnowledgeSource {
	if clientID == "" || apiKey == "" || kbID == "" {
		return nil
	}
	client := ima.NewClient(clientID, apiKey)
	client.WithTimeout(10 * time.Second)
	return &IMAKnowledgeSource{
		client: client,
		kbID:   kbID,
		name:   name,
	}
}

// Name returns the source name.
func (s *IMAKnowledgeSource) Name() string {
	if s.name != "" {
		return s.name
	}
	return "ima"
}

// Search queries the IMA knowledge base and returns matching results.
func (s *IMAKnowledgeSource) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("ima client not initialized")
	}

	// Use IMA KB Search API
	result, err := s.client.KB.Search(query, s.kbID, "")
	if err != nil {
		return nil, fmt.Errorf("ima search failed: %w", err)
	}

	// Convert IMA results to SearchResult format
	results := make([]SearchResult, 0, len(result.Items))
	for i, item := range result.Items {
		if i >= topK {
			break
		}
		results = append(results, SearchResult{
			Source:  fmt.Sprintf("%s:%s", s.Name(), item.MediaID),
			Title:   item.Title,
			Content: item.HighlightContent,
			Score:   0.8, // IMA doesn't return score, use fixed high score
		})
	}

	return results, nil
}
