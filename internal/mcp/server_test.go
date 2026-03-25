package mcp

import (
	"testing"

	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

func TestNewServer(t *testing.T) {
	t.Run("create server with nil dependencies", func(t *testing.T) {
		server := NewServer(nil, nil, nil)
		if server == nil {
			t.Fatal("expected non-nil server")
		}
	})

	t.Run("create server with mock db", func(t *testing.T) {
		db := &store.DB{}
		pipeline := &parser.Pipeline{}
		router := llm.NewRouter()

		server := NewServer(db, pipeline, router)
		if server == nil {
			t.Fatal("expected non-nil server")
		}
	})
}
