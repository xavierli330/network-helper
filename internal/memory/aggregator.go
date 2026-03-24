package memory

import (
	"context"
	"log"
	"sort"
	"sync"
)

// Aggregator fans out a query to multiple KnowledgeSource instances in parallel
// and merges results, sorted by score descending.
type Aggregator struct {
	sources []KnowledgeSource
}

// NewAggregator creates an Aggregator pre-loaded with the given sources.
func NewAggregator(sources ...KnowledgeSource) *Aggregator {
	a := &Aggregator{}
	for _, s := range sources {
		if s != nil {
			a.sources = append(a.sources, s)
		}
	}
	return a
}

// AddSource appends a new KnowledgeSource. Nil sources are silently ignored.
func (a *Aggregator) AddSource(s KnowledgeSource) {
	if s != nil {
		a.sources = append(a.sources, s)
	}
}

// Len returns the number of registered sources.
func (a *Aggregator) Len() int { return len(a.sources) }

// SearchAll queries all sources concurrently and returns up to topK results
// merged and sorted by score descending.
// Source errors are logged and skipped so a single unhealthy source does not
// block the response.
func (a *Aggregator) SearchAll(ctx context.Context, query string, topK int) []SearchResult {
	if len(a.sources) == 0 {
		return nil
	}

	var (
		mu  sync.Mutex
		all []SearchResult
		wg  sync.WaitGroup
	)

	for _, src := range a.sources {
		wg.Add(1)
		go func(s KnowledgeSource) {
			defer wg.Done()
			results, err := s.Search(ctx, query, topK)
			if err != nil {
				log.Printf("[knowledge/%s] search error: %v", s.Name(), err)
				return
			}
			if len(results) == 0 {
				return
			}
			mu.Lock()
			all = append(all, results...)
			mu.Unlock()
		}(src)
	}

	wg.Wait()

	// Sort by score descending, then trim to topK.
	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if len(all) > topK {
		all = all[:topK]
	}
	return all
}
