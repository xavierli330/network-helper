package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertAndGetDevice(t *testing.T) {
	db := testDB(t)
	dev := model.Device{
		ID: "core-01", Hostname: "Core-01", Vendor: "huawei",
		Model: "S12700", MgmtIP: "10.0.0.1", LastSeen: time.Now(),
	}
	if err := db.UpsertDevice(dev); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := db.GetDevice("core-01")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Hostname != "Core-01" {
		t.Errorf("expected Core-01, got %s", got.Hostname)
	}
	dev.Model = "S12708"
	if err := db.UpsertDevice(dev); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	got, _ = db.GetDevice("core-01")
	if got.Model != "S12708" {
		t.Errorf("expected S12708, got %s", got.Model)
	}
}

func TestListDevices(t *testing.T) {
	db := testDB(t)
	db.UpsertDevice(model.Device{ID: "d1", Hostname: "D1", Vendor: "huawei", LastSeen: time.Now()})
	db.UpsertDevice(model.Device{ID: "d2", Hostname: "D2", Vendor: "cisco", LastSeen: time.Now()})
	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("expected 2, got %d", len(devices))
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	db := testDB(t)
	_, err := db.GetDevice("nonexistent")
	if err == nil {
		t.Error("expected error")
	}
}
