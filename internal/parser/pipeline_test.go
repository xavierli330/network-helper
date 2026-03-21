package parser

import (
	"path/filepath"
	"testing"

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
	ribs, _ := db.GetRIBEntries("core-sw01", snapID)
	if len(ribs) != 2 {
		t.Errorf("rib entries: %d", len(ribs))
	}
	neighbors, _ := db.GetNeighbors("core-sw01", snapID)
	if len(neighbors) != 1 {
		t.Errorf("neighbors: %d", len(neighbors))
	}
}
