package memory

import (
	"testing"
)

func TestSearchResult(t *testing.T) {
	sr := SearchResult{
		Source:  "test",
		Title:   "Test Result",
		Content: "test content",
		Score:   0.95,
	}

	if sr.Source != "test" {
		t.Error("source mismatch")
	}
	if sr.Score < 0 || sr.Score > 1 {
		t.Error("score should be between 0 and 1")
	}
}

func TestNewAggregator(t *testing.T) {
	agg := NewAggregator()
	if agg == nil {
		t.Fatal("expected non-nil aggregator")
	}
	if agg.Len() != 0 {
		t.Error("new aggregator should have 0 sources")
	}
}

func TestAggregator_Len(t *testing.T) {
	agg := NewAggregator()
	if agg.Len() != 0 {
		t.Error("expected 0 sources initially")
	}
}
