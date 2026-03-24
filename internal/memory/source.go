package memory

import (
	"context"

	"github.com/xavierli/nethelper/internal/llm"
)

// SearchResult is a single result from any knowledge source.
type SearchResult struct {
	Source  string  // e.g. "local:network-sop.md" or "iwiki:page-123"
	Title   string  // short title
	Content string  // relevant text
	Score   float64 // relevance score (0-1)
}

// KnowledgeSource is a pluggable knowledge provider.
type KnowledgeSource interface {
	Name() string
	Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
}

// localKnowledgeAdapter wraps the embedding-based KnowledgeBase as a KnowledgeSource.
// It embeds the query on every Search call and returns cosine-nearest chunks.
type localKnowledgeAdapter struct {
	kb       *KnowledgeBase
	embedder llm.Embedder
}

// NewLocalKnowledgeAdapter returns a KnowledgeSource backed by kb.
// Returns nil when either argument is nil so callers can skip registration.
func NewLocalKnowledgeAdapter(kb *KnowledgeBase, embedder llm.Embedder) KnowledgeSource {
	if kb == nil || embedder == nil {
		return nil
	}
	return &localKnowledgeAdapter{kb: kb, embedder: embedder}
}

func (a *localKnowledgeAdapter) Name() string { return "local" }

func (a *localKnowledgeAdapter) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	vec, err := a.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	entries := a.kb.Search(vec, topK)
	if len(entries) == 0 {
		return nil, nil
	}
	results := make([]SearchResult, 0, len(entries))
	for _, e := range entries {
		results = append(results, SearchResult{
			Source:  "local:" + e.Source,
			Title:   e.Source,
			Content: e.Content,
			Score:   0.5, // local search returns binary pass/fail at 0.3 threshold; use 0.5 as baseline
		})
	}
	return results, nil
}
