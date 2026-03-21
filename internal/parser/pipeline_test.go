package parser

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/store"
)

func TestPipelineIngestFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(huawei.New())
	pipeline := NewPipeline(db, registry)

	content := "<Core-SW01>display interface brief\n" +
		"Interface                   PHY   Protocol InUti OutUti   inErr  outErr\n" +
		"GE0/0/1                     up    up       0.5%  0.3%         0       0\n" +
		"GE0/0/2                     down  down     0%    0%           0       0\n" +
		"<Core-SW01>display ip routing-table\n" +
		"Route Flags: R - relay\n" +
		"------------------------------------------------------------------------------\n" +
		"Routing Tables: Public\n" +
		"         Destinations : 2        Routes : 2\n\n" +
		"Destination/Mask    Proto   Pre  Cost  Flags NextHop         Interface\n" +
		"192.168.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1\n" +
		"10.0.0.0/16       Static  60   0     RD    10.0.0.2        GE0/0/2\n" +
		"<Core-SW01>display ospf peer\n" +
		"OSPF Process 1 with Router ID 10.0.0.1\n" +
		"                 Peer Statistic Information\n" +
		" ----------------------------------------------------------------------------\n" +
		" Area Id          Interface                        Neighbor id      State\n" +
		" 0.0.0.0          GE0/0/1                          10.0.0.2         Full\n"

	result, err := pipeline.Ingest("test-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}
	if result.DevicesFound != 1 {
		t.Errorf("devices: %d", result.DevicesFound)
	}
	if result.BlocksParsed != 3 {
		t.Errorf("blocks parsed: %d", result.BlocksParsed)
	}

	dev, err := db.GetDevice("core-sw01")
	if err != nil {
		t.Fatalf("device not found: %v", err)
	}
	if dev.Hostname != "Core-SW01" {
		t.Errorf("hostname: %s", dev.Hostname)
	}
	if dev.Vendor != "huawei" {
		t.Errorf("vendor: %s", dev.Vendor)
	}

	ifaces, _ := db.GetInterfaces("core-sw01")
	if len(ifaces) != 2 {
		t.Errorf("interfaces: %d", len(ifaces))
	}

	snapID, err := db.LatestSnapshotID("core-sw01")
	if err != nil {
		t.Fatalf("no snapshot: %v", err)
	}
	// RIB entries now go to scratch pad (not structured storage)
	scratches, _ := db.ListScratch("core-sw01", "route", 10)
	if len(scratches) != 1 {
		t.Errorf("scratch route entries: %d (expected 1)", len(scratches))
	}

	neighbors, _ := db.GetNeighbors("core-sw01", snapID)
	if len(neighbors) != 1 {
		t.Errorf("neighbors: %d", len(neighbors))
	}
}

func TestPipelineIngest_CapturedAtPassthrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(huawei.New())
	pipeline := NewPipeline(db, registry)

	// Log content with timestamp prefixes; the first command has the earliest timestamp.
	content := "2026-03-21-10-00-00: <SW01>display version\nHuawei VRP V200R020\n" +
		"2026-03-21-10-05-00: <SW01>display interface brief\n" +
		"Interface                   PHY   Protocol InUti OutUti   inErr  outErr\n" +
		"GE0/0/1                     up    up       0.5%  0.3%         0       0\n"

	_, err = pipeline.Ingest("ts-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}

	snapID, err := db.LatestSnapshotID("sw01")
	if err != nil {
		t.Fatalf("no snapshot: %v", err)
	}
	snap, err := db.GetSnapshot(snapID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}

	wantTime, _ := time.ParseInLocation("2006-01-02-15-04-05", "2026-03-21-10-00-00", time.Local)
	if snap.CapturedAt.IsZero() {
		t.Error("snapshot CapturedAt should not be zero")
	} else if !snap.CapturedAt.Equal(wantTime) {
		t.Errorf("snapshot CapturedAt: got %v, want %v", snap.CapturedAt, wantTime)
	}
}

func TestPipelineIngest_CapturedAtFallback(t *testing.T) {
	// Without timestamps in the log, CapturedAt should be zero (db DEFAULT takes over).
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	registry := NewRegistry()
	registry.Register(huawei.New())
	pipeline := NewPipeline(db, registry)

	before := time.Now().Truncate(time.Second)
	content := "<SW02>display version\nHuawei VRP V200R020\n"
	_, err = pipeline.Ingest("no-ts-log.txt", content)
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}

	snapID, err := db.LatestSnapshotID("sw02")
	if err != nil {
		t.Fatalf("no snapshot: %v", err)
	}
	snap, err := db.GetSnapshot(snapID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}

	// Without an explicit CapturedAt the DB DEFAULT CURRENT_TIMESTAMP takes effect.
	// Verify the timestamp is recent (within a few seconds of ingestion).
	after := time.Now().Add(time.Second)
	if snap.CapturedAt.IsZero() {
		t.Error("snapshot CapturedAt should not be zero (db default should apply)")
	} else if snap.CapturedAt.Before(before) || snap.CapturedAt.After(after) {
		t.Errorf("snapshot CapturedAt %v is not within expected range [%v, %v]", snap.CapturedAt, before, after)
	}
}
