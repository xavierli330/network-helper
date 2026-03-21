package llm

import (
	"context"
	"testing"
)

type mockProvider struct {
	name     string
	response string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{Content: m.response}, nil
}
func (m *mockProvider) Supports(cap Capability) bool { return true }

func TestCapabilityString(t *testing.T) {
	tests := []struct {
		cap  Capability
		want string
	}{
		{CapExtract, "extract"},
		{CapAnalyze, "analyze"},
		{CapExplain, "explain"},
		{CapParse, "parse"},
	}
	for _, tt := range tests {
		if string(tt.cap) != tt.want {
			t.Errorf("got %q, want %q", tt.cap, tt.want)
		}
	}
}

func TestChatRequestMessages(t *testing.T) {
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a network engineer."},
			{Role: "user", Content: "What is OSPF?"},
		},
	}
	if len(req.Messages) != 2 {
		t.Errorf("expected 2, got %d", len(req.Messages))
	}
}

func TestMockProviderChat(t *testing.T) {
	p := &mockProvider{name: "mock", response: "test response"}
	resp, err := p.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "test response" {
		t.Errorf("got: %s", resp.Content)
	}
}
