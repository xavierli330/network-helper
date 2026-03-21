package store

import (
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func TestSearchConfig(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)

	db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID: "d1", ConfigText: "interface GE0/0/1\n ip address 10.0.0.1 24\n ospf cost 100\n#",
		SourceFile: "test.log",
	})
	db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID: "d1", ConfigText: "interface GE0/0/2\n ip address 10.0.0.2 24\n#",
		SourceFile: "test2.log",
	})

	// Sync FTS index
	db.SyncConfigFTS()

	results, err := db.SearchConfig("ospf cost")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
}

func TestSearchCommands(t *testing.T) {
	db := testDB(t)

	db.InsertCommandReference(model.CommandReference{
		Vendor: "huawei", Command: "display ip routing-table",
		Description: "Show the IP routing table with all routes and their attributes",
	})
	db.InsertCommandReference(model.CommandReference{
		Vendor: "huawei", Command: "display mpls ldp session",
		Description: "Show MPLS LDP session status and peer information",
	})

	results, err := db.SearchCommands("routing table")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Command != "display ip routing-table" {
		t.Errorf("got: %s", results[0].Command)
	}
}

// suppress unused import
var _ = time.Now
