package memory

import (
	"testing"
)

func TestNewIMAKnowledgeSource(t *testing.T) {
	tests := []struct {
		name     string
		clientID string
		apiKey   string
		kbID     string
		wantNil  bool
	}{
		{
			name:     "valid credentials",
			clientID: "test_client_id",
			apiKey:   "test_api_key",
			kbID:     "test_kb_id",
			wantNil:  false,
		},
		{
			name:     "missing clientID",
			clientID: "",
			apiKey:   "test_api_key",
			kbID:     "test_kb_id",
			wantNil:  true,
		},
		{
			name:     "missing apiKey",
			clientID: "test_client_id",
			apiKey:   "",
			kbID:     "test_kb_id",
			wantNil:  true,
		},
		{
			name:     "missing kbID",
			clientID: "test_client_id",
			apiKey:   "test_api_key",
			kbID:     "",
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIMAKnowledgeSource(tt.clientID, tt.apiKey, tt.kbID, "ima")
			if (got == nil) != tt.wantNil {
				t.Errorf("NewIMAKnowledgeSource() nil = %v, wantNil %v", got == nil, tt.wantNil)
			}
		})
	}
}

func TestIMAKnowledgeSource_Name(t *testing.T) {
	src := NewIMAKnowledgeSource("id", "key", "kb", "myima")
	if src == nil {
		t.Fatal("expected non-nil source")
	}

	if got := src.Name(); got != "myima" {
		t.Errorf("Name() = %v, want %v", got, "myima")
	}

	// Test default name
	src2 := NewIMAKnowledgeSource("id", "key", "kb", "")
	if src2 == nil {
		t.Fatal("expected non-nil source")
	}
	if got := src2.Name(); got != "ima" {
		t.Errorf("Name() = %v, want %v", got, "ima")
	}
}
