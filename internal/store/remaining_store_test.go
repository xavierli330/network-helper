package store

import (
	"testing"
	"time"
	"github.com/xavierli/nethelper/internal/model"
)

func TestInsertAndGetNeighbors(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})
	neighbors := []model.NeighborInfo{
		{DeviceID: "d1", Protocol: "ospf", RemoteID: "2.2.2.2", State: "full", AreaID: "0.0.0.0", SnapshotID: snapID},
	}
	if err := db.InsertNeighbors(neighbors); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, _ := db.GetNeighbors("d1", snapID)
	if len(got) != 1 || got[0].Protocol != "ospf" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestInsertAndGetTunnels(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})
	tunnels := []model.TunnelInfo{
		{DeviceID: "d1", TunnelName: "Tunnel1", Type: "rsvp-te", State: "up", Destination: "3.3.3.3", SnapshotID: snapID},
	}
	if err := db.InsertTunnels(tunnels); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, _ := db.GetTunnels("d1", snapID)
	if len(got) != 1 || got[0].TunnelName != "Tunnel1" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestInsertAndGetSRMappings(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})
	mappings := []model.SRMapping{
		{DeviceID: "d1", Prefix: "1.1.1.1", SIDIndex: 100, SIDLabel: 16100, Algorithm: 0, Source: "isis", SnapshotID: snapID},
	}
	if err := db.InsertSRMappings(mappings); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, _ := db.GetSRMappings("d1", snapID)
	if len(got) != 1 || got[0].SIDLabel != 16100 {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestUpsertIngestion(t *testing.T) {
	db := testDB(t)
	ing := model.LogIngestion{FilePath: "/tmp/log.txt", FileHash: "abc123", LastOffset: 1024, ProcessedAt: time.Now()}
	if err := db.UpsertIngestion(ing); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := db.GetIngestion("/tmp/log.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastOffset != 1024 {
		t.Errorf("expected 1024, got %d", got.LastOffset)
	}
	ing.LastOffset = 2048
	db.UpsertIngestion(ing)
	got, _ = db.GetIngestion("/tmp/log.txt")
	if got.LastOffset != 2048 {
		t.Errorf("expected 2048, got %d", got.LastOffset)
	}
}
