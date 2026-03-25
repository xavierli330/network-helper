// internal/parser/collector_test.go
package parser_test

import (
	"testing"

	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/store"
)

func TestCollectorNormalisesCommand(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	c := parser.NewCollector(db)
	block := parser.CommandBlock{
		Hostname: "R1", Vendor: "huawei",
		Command: "dis int brief", Output: "PHY Protocol",
	}
	if err := c.Collect(block); err != nil {
		t.Fatal(err)
	}

	rows, err := db.ListUnknownOutputs("huawei", "new", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	// Verb expanded; interior abbrev "int" stays as-is (simplified normalisation)
	if rows[0].CommandNorm != "display int brief" {
		t.Errorf("want 'display int brief', got %q", rows[0].CommandNorm)
	}
}

func TestCollectorDeduplicates(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	block := parser.CommandBlock{
		Vendor: "huawei", Command: "display traffic-policy", Output: "identical output",
	}
	c.Collect(block)
	c.Collect(block) // same content

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 1 {
		t.Errorf("want 1 deduplicated row, got %d", len(rows))
	}
	if rows[0].OccurrenceCount != 2 {
		t.Errorf("want occurrence_count=2, got %d", rows[0].OccurrenceCount)
	}
}

func TestCollectorSilentOnNilDB(t *testing.T) {
	c := parser.NewCollector(nil)
	block := parser.CommandBlock{Vendor: "huawei", Command: "display foo", Output: "x"}
	// Must not panic; error is silently swallowed
	c.Collect(block)
}

func TestPipelineCollectsUnknownCommands(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	reg := parser.NewRegistry()
	reg.Register(huawei.New())
	pipe := parser.NewPipelineWithCollector(db, reg, parser.NewCollector(db))

	// Hostname must be ≥3 chars — huawei.DetectPrompt rejects shorter hostnames
	// "display traffic-policy" is not in huawei.ClassifyCommand → CmdUnknown
	log := "<RTR>display traffic-policy\nPolicy: p1\n"
	_, err := pipe.Ingest("test.log", log)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 1 {
		t.Errorf("want 1 unknown output collected, got %d", len(rows))
	}
}
