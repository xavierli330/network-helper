package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func TestInsertAndGetCoverageCheck(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Seed a device
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "huawei"})

	cc := CoverageCheck{
		DeviceID:     "r1",
		Vendor:       "huawei",
		TotalCount:   10,
		CoveredCount: 7,
		CoveragePct:  70.0,
		ItemsJSON:    `[{"command":"display bgp peer","status":"covered"}]`,
		CheckedAt:    time.Now(),
	}
	id, err := db.InsertCoverageCheck(cc)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}

	got, err := db.GetCoverageCheck("r1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.TotalCount != 10 {
		t.Errorf("expected total_count=10, got %d", got.TotalCount)
	}
	if got.CoveredCount != 7 {
		t.Errorf("expected covered_count=7, got %d", got.CoveredCount)
	}
}

func TestCoverageCheckReplaceOnConflict(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cc1 := CoverageCheck{
		DeviceID: "r1", Vendor: "huawei",
		TotalCount: 10, CoveredCount: 5, CoveragePct: 50.0,
		ItemsJSON: "[]", CheckedAt: time.Now(),
	}
	db.InsertCoverageCheck(cc1)

	cc2 := CoverageCheck{
		DeviceID: "r1", Vendor: "huawei",
		TotalCount: 12, CoveredCount: 9, CoveragePct: 75.0,
		ItemsJSON: "[]", CheckedAt: time.Now(),
	}
	db.InsertCoverageCheck(cc2)

	// Should only have latest
	got, _ := db.GetCoverageCheck("r1")
	if got.TotalCount != 12 {
		t.Errorf("expected replaced total_count=12, got %d", got.TotalCount)
	}
}

func TestListCoverageChecks(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.InsertCoverageCheck(CoverageCheck{
		DeviceID: "r1", Vendor: "huawei",
		TotalCount: 10, CoveredCount: 7, CoveragePct: 70.0,
		ItemsJSON: "[]", CheckedAt: time.Now(),
	})
	db.InsertCoverageCheck(CoverageCheck{
		DeviceID: "r2", Vendor: "cisco",
		TotalCount: 8, CoveredCount: 8, CoveragePct: 100.0,
		ItemsJSON: "[]", CheckedAt: time.Now(),
	})

	checks, err := db.ListCoverageChecks()
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(checks))
	}
}

func TestGetCoverageCheck_NotFound(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	got, err := db.GetCoverageCheck("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for non-existent device")
	}
}

func TestDeleteCoverageCheck(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.InsertCoverageCheck(CoverageCheck{
		DeviceID: "r1", Vendor: "huawei",
		TotalCount: 5, CoveredCount: 5, CoveragePct: 100.0,
		ItemsJSON: "[]", CheckedAt: time.Now(),
	})

	err = db.DeleteCoverageCheck("r1")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetCoverageCheck("r1")
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestParseCoverageItems(t *testing.T) {
	json := `[{"command":"display bgp peer","category":"bgp","reason":"has BGP","priority":"high","status":"covered","cmd_type":"neighbor"}]`
	items, err := ParseCoverageItems(json)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Command != "display bgp peer" {
		t.Errorf("unexpected command: %s", items[0].Command)
	}
}
