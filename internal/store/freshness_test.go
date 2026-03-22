package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func setupDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// makeSnapshot creates a snapshot for deviceID and returns its ID.
func makeSnapshot(t *testing.T, db *DB, deviceID string) int {
	t.Helper()
	id, err := db.CreateSnapshot(model.Snapshot{
		DeviceID:    deviceID,
		SourceFile:  "test.log",
		Commands:    "[]",
		CapturedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------------
// Neighbor tests
// ---------------------------------------------------------------------------

func TestInsertNeighbors_Dedup(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "huawei", LastSeen: time.Now()})
	snapID := makeSnapshot(t, db, "r1")

	n := model.NeighborInfo{
		DeviceID:      "r1",
		Protocol:      "OSPF",
		LocalID:       "1.1.1.1",
		RemoteID:      "2.2.2.2",
		LocalInterface: "GE0/0/0",
		RemoteAddress: "10.0.0.2",
		State:         "Full",
		AreaID:        "0",
		ASNumber:      0,
		Uptime:        "1d",
		SnapshotID:    snapID,
	}

	// Insert the same neighbor twice.
	if err := db.InsertNeighbors([]model.NeighborInfo{n}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.InsertNeighbors([]model.NeighborInfo{n}); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	rows, err := db.GetNeighbors("r1", snapID)
	if err != nil {
		t.Fatalf("GetNeighbors: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 row after dedup, got %d", len(rows))
	}
}

func TestInsertNeighbors_UpdateOnConflict(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "huawei", LastSeen: time.Now()})
	snapID := makeSnapshot(t, db, "r1")

	base := model.NeighborInfo{
		DeviceID:      "r1",
		Protocol:      "OSPF",
		LocalID:       "1.1.1.1",
		RemoteID:      "2.2.2.2",
		LocalInterface: "GE0/0/0",
		RemoteAddress: "10.0.0.2",
		State:         "Init",
		AreaID:        "0",
		ASNumber:      0,
		Uptime:        "5s",
		SnapshotID:    snapID,
	}

	if err := db.InsertNeighbors([]model.NeighborInfo{base}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Same key, updated state and uptime.
	updated := base
	updated.State = "Full"
	updated.Uptime = "1d"

	if err := db.InsertNeighbors([]model.NeighborInfo{updated}); err != nil {
		t.Fatalf("second insert (upsert): %v", err)
	}

	rows, err := db.GetNeighbors("r1", snapID)
	if err != nil {
		t.Fatalf("GetNeighbors: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].State != "Full" {
		t.Errorf("expected state=Full after upsert, got %q", rows[0].State)
	}
	if rows[0].Uptime != "1d" {
		t.Errorf("expected uptime=1d after upsert, got %q", rows[0].Uptime)
	}
}

func TestGetLatestNeighbors(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "huawei", LastSeen: time.Now()})

	snap1 := makeSnapshot(t, db, "r1")
	snap2 := makeSnapshot(t, db, "r1")

	n1 := model.NeighborInfo{
		DeviceID: "r1", Protocol: "OSPF",
		LocalID: "1.1.1.1", RemoteID: "2.2.2.2",
		RemoteAddress: "10.0.0.2", State: "Full",
		SnapshotID: snap1,
	}
	n2 := model.NeighborInfo{
		DeviceID: "r1", Protocol: "OSPF",
		LocalID: "1.1.1.1", RemoteID: "3.3.3.3",
		RemoteAddress: "10.0.0.3", State: "Full",
		SnapshotID: snap2,
	}

	if err := db.InsertNeighbors([]model.NeighborInfo{n1}); err != nil {
		t.Fatalf("insert snap1: %v", err)
	}
	if err := db.InsertNeighbors([]model.NeighborInfo{n2}); err != nil {
		t.Fatalf("insert snap2: %v", err)
	}

	latest, err := db.GetLatestNeighbors("r1")
	if err != nil {
		t.Fatalf("GetLatestNeighbors: %v", err)
	}
	if len(latest) != 1 {
		t.Fatalf("expected 1 row from latest snapshot, got %d", len(latest))
	}
	if latest[0].RemoteID != "3.3.3.3" {
		t.Errorf("expected RemoteID=3.3.3.3 (snap2), got %q", latest[0].RemoteID)
	}
}

// ---------------------------------------------------------------------------
// BGP peer tests
// ---------------------------------------------------------------------------

func TestInsertBGPPeers_Dedup(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "cisco", LastSeen: time.Now()})
	snapID := makeSnapshot(t, db, "r1")

	p := model.BGPPeer{
		DeviceID:      "r1",
		VRF:           "default",
		LocalAS:       65001,
		PeerIP:        "10.0.0.1",
		RemoteAS:      65002,
		AddressFamily: "ipv4",
		Enabled:       1,
		SnapshotID:    snapID,
	}

	if err := db.InsertBGPPeers([]model.BGPPeer{p}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.InsertBGPPeers([]model.BGPPeer{p}); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	peers, err := db.GetBGPPeers("r1", snapID)
	if err != nil {
		t.Fatalf("GetBGPPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Errorf("expected 1 row after dedup, got %d", len(peers))
	}
}

func TestInsertBGPPeers_UpdateOnConflict(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "cisco", LastSeen: time.Now()})
	snapID := makeSnapshot(t, db, "r1")

	p := model.BGPPeer{
		DeviceID:      "r1",
		VRF:           "default",
		LocalAS:       65001,
		PeerIP:        "10.0.0.1",
		RemoteAS:      65002,
		AddressFamily: "ipv4",
		Description:   "original",
		Enabled:       1,
		SnapshotID:    snapID,
	}

	if err := db.InsertBGPPeers([]model.BGPPeer{p}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	updated := p
	updated.Description = "updated"

	if err := db.InsertBGPPeers([]model.BGPPeer{updated}); err != nil {
		t.Fatalf("second insert (upsert): %v", err)
	}

	peers, err := db.GetBGPPeers("r1", snapID)
	if err != nil {
		t.Fatalf("GetBGPPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 row, got %d", len(peers))
	}
	if peers[0].Description != "updated" {
		t.Errorf("expected description=updated, got %q", peers[0].Description)
	}
}

func TestGetLatestBGPPeers(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "cisco", LastSeen: time.Now()})

	snap1 := makeSnapshot(t, db, "r1")
	// snap2 intentionally has no peers — GetLatestBGPPeers should return snap1 data.
	_ = makeSnapshot(t, db, "r1")

	p := model.BGPPeer{
		DeviceID:      "r1",
		VRF:           "default",
		LocalAS:       65001,
		PeerIP:        "192.168.1.1",
		RemoteAS:      65002,
		AddressFamily: "ipv4",
		Enabled:       1,
		SnapshotID:    snap1,
	}

	if err := db.InsertBGPPeers([]model.BGPPeer{p}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	peers, err := db.GetLatestBGPPeers("r1")
	if err != nil {
		t.Fatalf("GetLatestBGPPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].PeerIP != "192.168.1.1" {
		t.Errorf("expected PeerIP=192.168.1.1, got %q", peers[0].PeerIP)
	}
}

func TestGetLatestBGPPeers_Empty(t *testing.T) {
	db := setupDB(t)
	db.UpsertDevice(model.Device{ID: "r1", Hostname: "R1", Vendor: "cisco", LastSeen: time.Now()})

	peers, err := db.GetLatestBGPPeers("r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected nil/empty slice for device with no BGP peers, got %d entries", len(peers))
	}
}
