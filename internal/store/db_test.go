package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer db.Close()

	tables := []string{
		"devices", "interfaces", "snapshots", "rib_entries",
		"fib_entries", "lfib_entries", "protocol_neighbors",
		"mpls_te_tunnels", "sr_mappings", "config_snapshots",
		"troubleshoot_logs", "command_references", "log_ingestions",
		"embedding_meta",
	}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}
}

func TestOpenCreatesDirectory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sub", "dir", "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	db.Close()
}
