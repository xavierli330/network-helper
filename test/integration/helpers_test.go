//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/parser/cisco"
	"github.com/xavierli/nethelper/internal/parser/h3c"
	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/parser/juniper"
	"github.com/xavierli/nethelper/internal/store"
)

// setupTestDB creates a temporary SQLite database for testing.
func setupTestDB(t *testing.T) *store.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("setupTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// setupPipeline creates a DB and parser pipeline with all 4 vendor parsers
// registered in the same order as root.go: huawei, cisco, h3c, juniper.
func setupPipeline(t *testing.T) (*parser.Pipeline, *store.DB) {
	t.Helper()
	db := setupTestDB(t)
	registry := parser.NewRegistry()
	registry.Register(huawei.New())
	registry.Register(cisco.New())
	registry.Register(h3c.New())
	registry.Register(juniper.New())
	pipeline := parser.NewPipeline(db, registry)
	return pipeline, db
}

// readTestdata reads a file relative to the testdata/ directory.
func readTestdata(t *testing.T, relPath string) string {
	t.Helper()
	path := filepath.Join("testdata", relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readTestdata(%q): %v", relPath, err)
	}
	return string(data)
}
