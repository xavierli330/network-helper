package llm

import (
	"context"
	"testing"
)

func TestRouterDispatch(t *testing.T) {
	r := NewRouter()
	r.SetDefault(&mockProvider{name: "default", response: "default answer"})
	r.SetForCapability(CapAnalyze, &mockProvider{name: "analyzer", response: "analysis result"})

	// Default provider for extract
	resp, err := r.Chat(context.Background(), CapExtract, ChatRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "default answer" {
		t.Errorf("got: %s", resp.Content)
	}

	// Specific provider for analyze
	resp, err = r.Chat(context.Background(), CapAnalyze, ChatRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "analysis result" {
		t.Errorf("got: %s", resp.Content)
	}
}

func TestRouterNoProvider(t *testing.T) {
	r := NewRouter()
	_, err := r.Chat(context.Background(), CapExtract, ChatRequest{})
	if err == nil {
		t.Error("expected error with no provider")
	}
}

func TestRouterAvailable(t *testing.T) {
	r := NewRouter()
	if r.Available() {
		t.Error("should not be available with no providers")
	}

	r.SetDefault(&mockProvider{name: "test"})
	if !r.Available() {
		t.Error("should be available with default provider")
	}
}
