package store

import (
	"testing"
	"time"
	"github.com/xavierli/nethelper/internal/model"
)

func seedDevice(t *testing.T, db *DB) {
	t.Helper()
	db.UpsertDevice(model.Device{ID: "d1", Hostname: "D1", Vendor: "huawei", LastSeen: time.Now()})
}

func TestCreateSnapshotAndRIB(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, err := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt", Commands: `["display ip routing-table"]`})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snapID == 0 {
		t.Error("snapshot ID should be non-zero")
	}
	entries := []model.RIBEntry{
		{DeviceID: "d1", Prefix: "10.0.0.0", MaskLen: 24, Protocol: "ospf", NextHop: "10.0.1.1", SnapshotID: snapID},
		{DeviceID: "d1", Prefix: "172.16.0.0", MaskLen: 16, Protocol: "bgp", NextHop: "10.0.1.2", SnapshotID: snapID},
	}
	if err := db.InsertRIBEntries(entries); err != nil {
		t.Fatalf("insert rib: %v", err)
	}
	got, err := db.GetRIBEntries("d1", snapID)
	if err != nil {
		t.Fatalf("get rib: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
}

func TestInsertAndGetFIB(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})
	entries := []model.FIBEntry{
		{DeviceID: "d1", Prefix: "10.0.0.0", MaskLen: 24, NextHop: "10.0.1.1", LabelAction: "push", OutLabel: "16001", SnapshotID: snapID},
	}
	if err := db.InsertFIBEntries(entries); err != nil {
		t.Fatalf("insert fib: %v", err)
	}
	got, _ := db.GetFIBEntries("d1", snapID)
	if len(got) != 1 {
		t.Errorf("expected 1, got %d", len(got))
	}
}

func TestInsertAndGetLFIB(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)
	snapID, _ := db.CreateSnapshot(model.Snapshot{DeviceID: "d1", SourceFile: "/tmp/log.txt"})
	entries := []model.LFIBEntry{
		{DeviceID: "d1", InLabel: 16001, Action: "swap", OutLabel: "16002", NextHop: "10.0.1.1", Protocol: "ldp", SnapshotID: snapID},
	}
	if err := db.InsertLFIBEntries(entries); err != nil {
		t.Fatalf("insert lfib: %v", err)
	}
	got, _ := db.GetLFIBEntries("d1", snapID)
	if len(got) != 1 || got[0].InLabel != 16001 {
		t.Errorf("unexpected: %+v", got)
	}
}
