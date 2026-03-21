// internal/graph/builder_test.go
package graph

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

func seedTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Two devices
	db.UpsertDevice(model.Device{ID: "core-01", Hostname: "Core-01", Vendor: "huawei", RouterID: "1.1.1.1", LastSeen: time.Now()})
	db.UpsertDevice(model.Device{ID: "pe-01", Hostname: "PE-01", Vendor: "cisco", RouterID: "2.2.2.2", LastSeen: time.Now()})

	// Interfaces
	db.UpsertInterface(model.Interface{ID: "core-01:GE0/0/1", DeviceID: "core-01", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.1", Mask: "30", LastUpdated: time.Now()})
	db.UpsertInterface(model.Interface{ID: "core-01:Lo0", DeviceID: "core-01", Name: "Lo0", Type: model.IfTypeLoopback, Status: "up", IPAddress: "1.1.1.1", LastUpdated: time.Now()})
	db.UpsertInterface(model.Interface{ID: "pe-01:Gi0/0", DeviceID: "pe-01", Name: "Gi0/0", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.2", Mask: "30", LastUpdated: time.Now()})

	// Snapshot + Neighbors (OSPF peer)
	snapID1, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "core-01", SourceFile: "test"})
	db.InsertNeighbors([]model.NeighborInfo{
		{DeviceID: "core-01", Protocol: "ospf", RemoteID: "2.2.2.2", LocalInterface: "GE0/0/1", State: "full", SnapshotID: snapID1},
	})

	// RIB
	db.InsertRIBEntries([]model.RIBEntry{
		{DeviceID: "core-01", Prefix: "10.0.0.0", MaskLen: 30, Protocol: "direct", NextHop: "", OutgoingInterface: "GE0/0/1", SnapshotID: snapID1},
		{DeviceID: "core-01", Prefix: "2.2.2.2", MaskLen: 32, Protocol: "ospf", NextHop: "10.0.0.2", OutgoingInterface: "GE0/0/1", SnapshotID: snapID1},
	})

	return db
}

func TestBuildFromDB(t *testing.T) {
	db := seedTestDB(t)
	g, err := BuildFromDB(db)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// Should have device nodes
	if _, ok := g.GetNode("core-01"); !ok {
		t.Error("core-01 not found")
	}
	if _, ok := g.GetNode("pe-01"); !ok {
		t.Error("pe-01 not found")
	}

	// Should have interface nodes
	if _, ok := g.GetNode("core-01:GE0/0/1"); !ok {
		t.Error("interface not found")
	}

	// Should have HAS_INTERFACE edges
	edges := g.NeighborsByType("core-01", EdgeHasInterface)
	if len(edges) < 1 {
		t.Error("expected HAS_INTERFACE edges from core-01")
	}

	// Should have PEER edges from neighbor data
	peers := g.NeighborsByType("core-01", EdgePeer)
	if len(peers) < 1 {
		t.Error("expected PEER edge from core-01")
	}

	// Should have device count >= 2
	devices := g.NodesByType(NodeTypeDevice)
	if len(devices) < 2 {
		t.Errorf("expected >=2 devices, got %d", len(devices))
	}
}

func TestBuildSubnets(t *testing.T) {
	// Create a fresh DB with two interfaces on the same /30
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "huawei", LastSeen: time.Now()})
	db.UpsertDevice(model.Device{ID: "r2", Hostname: "R2", Vendor: "cisco", LastSeen: time.Now()})
	// 10.0.0.1/30 and 10.0.0.2/30 → both in subnet 10.0.0.0/30
	db.UpsertInterface(model.Interface{ID: "r1:ge1", DeviceID: "r1", Name: "GE0/0/1", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.1", Mask: "30", LastUpdated: time.Now()})
	db.UpsertInterface(model.Interface{ID: "r2:gi0", DeviceID: "r2", Name: "Gi0/0", Type: model.IfTypePhysical, Status: "up", IPAddress: "10.0.0.2", Mask: "30", LastUpdated: time.Now()})

	g, _ := BuildFromDB(db)

	// Both interfaces should point to the same subnet node "10.0.0.0/30"
	subnets := g.NodesByType(NodeTypeSubnet)
	if len(subnets) != 1 {
		t.Fatalf("expected 1 subnet, got %d", len(subnets))
	}
	if subnets[0].ID != "10.0.0.0/30" {
		t.Errorf("expected 10.0.0.0/30, got %s", subnets[0].ID)
	}

	// Should have CONNECTS_TO edge between the two interfaces
	edges := g.EdgesBetween("r1:ge1", "r2:gi0")
	connectsFound := false
	for _, e := range edges {
		if e.Type == EdgeConnectsTo {
			connectsFound = true
		}
	}
	if !connectsFound {
		t.Error("expected CONNECTS_TO edge between interfaces on same /30 subnet")
	}
}
