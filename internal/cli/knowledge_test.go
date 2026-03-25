package cli

import (
	"testing"
)

func TestNewKnowledgeCmd(t *testing.T) {
	cmd := newKnowledgeCmd()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Use != "knowledge" {
		t.Errorf("expected use 'knowledge', got %s", cmd.Use)
	}
}

func TestNewKnowledgeSearchCmd(t *testing.T) {
	cmd := newKnowledgeSearchCmd()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Use != "search <query>" {
		t.Errorf("expected use 'search <query>', got %s", cmd.Use)
	}

	// Check flags
	flags := cmd.Flags()
	if flags.Lookup("source") == nil {
		t.Error("expected --source flag")
	}
	if flags.Lookup("all") == nil {
		t.Error("expected --all flag")
	}
	if flags.Lookup("limit") == nil {
		t.Error("expected --limit flag")
	}
}

func TestNewKnowledgeSourcesCmd(t *testing.T) {
	cmd := newKnowledgeSourcesCmd()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Use != "sources" {
		t.Errorf("expected use 'sources', got %s", cmd.Use)
	}
}
