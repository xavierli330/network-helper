//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"
)

func TestIngestHuawei(t *testing.T) {
	pipeline, db := setupPipeline(t)
	content := readTestdata(t, "huawei/teg_20260321162156.log")

	result, err := pipeline.Ingest("huawei_test.log", content)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if result.DevicesFound != 2 {
		t.Fatalf("expected 2 devices, got %d", result.DevicesFound)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices in DB, got %d", len(devices))
	}

	expectedDevices := map[string]bool{
		"gz-hxy-g160304-b02-hw12816-cuf-13": false,
		"cd-gx-0402-j20-ne40e-br-01":        false,
	}
	for _, d := range devices {
		if _, ok := expectedDevices[d.ID]; !ok {
			t.Errorf("unexpected device ID: %s", d.ID)
			continue
		}
		expectedDevices[d.ID] = true
		if d.Vendor != "huawei" {
			t.Errorf("device %s: expected vendor=huawei, got %s", d.ID, d.Vendor)
		}
	}
	for id, found := range expectedDevices {
		if !found {
			t.Errorf("expected device %s not found", id)
		}
	}

	// Check snapshots exist for both devices
	for id := range expectedDevices {
		snapID, err := db.LatestSnapshotID(id)
		if err != nil {
			t.Errorf("no snapshot for device %s: %v", id, err)
			continue
		}
		snap, err := db.GetSnapshot(snapID)
		if err != nil {
			t.Errorf("GetSnapshot(%d): %v", snapID, err)
			continue
		}
		if snap.CapturedAt.IsZero() {
			t.Errorf("device %s: CapturedAt is zero", id)
		}
	}

	// Check config snapshots exist with expected prefix
	for id := range expectedDevices {
		configs, err := db.GetConfigSnapshots(id)
		if err != nil {
			t.Errorf("GetConfigSnapshots(%s): %v", id, err)
			continue
		}
		if len(configs) == 0 {
			t.Errorf("device %s: no config snapshots", id)
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(configs[0].ConfigText), "!Software Version") {
			t.Errorf("device %s: config does not start with '!Software Version', got: %.60s",
				id, configs[0].ConfigText)
		}
	}
}

func TestIngestH3C(t *testing.T) {
	pipeline, db := setupPipeline(t)
	content := readTestdata(t, "h3c/teg_20260321163710.log")

	_, err := pipeline.Ingest("h3c_test.log", content)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	d := devices[0]
	expectedID := "gz-hxy-0203-c05-h12516xaf-qcdr-01"
	if d.ID != expectedID {
		t.Errorf("expected device ID %s, got %s", expectedID, d.ID)
	}
	if d.Vendor != "h3c" {
		t.Errorf("expected vendor=h3c, got %s (H3C disambiguation failed)", d.Vendor)
	}

	configs, err := db.GetConfigSnapshots(d.ID)
	if err != nil {
		t.Fatalf("GetConfigSnapshots: %v", err)
	}
	if len(configs) == 0 {
		t.Fatal("no config snapshots found")
	}
	if !strings.Contains(configs[0].ConfigText, "version 7.1.070") {
		t.Error("config does not contain 'version 7.1.070'")
	}
}

func TestIngestCisco(t *testing.T) {
	pipeline, db := setupPipeline(t)
	content := readTestdata(t, "cisco/teg_20260321162808.log")

	_, err := pipeline.Ingest("cisco_test.log", content)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	d := devices[0]
	expectedID := "gz-ys-0101-g05-asr9912-qcstix-01"
	if d.ID != expectedID {
		t.Errorf("expected device ID %s, got %s", expectedID, d.ID)
	}
	if d.Vendor != "cisco" {
		t.Errorf("expected vendor=cisco, got %s", d.Vendor)
	}

	configs, err := db.GetConfigSnapshots(d.ID)
	if err != nil {
		t.Fatalf("GetConfigSnapshots: %v", err)
	}
	// Help queries (show running-config ?) should NOT create config snapshots.
	// Only the actual "show running-config" should produce exactly 1 config.
	if len(configs) != 1 {
		t.Errorf("expected exactly 1 config snapshot (help queries excluded), got %d", len(configs))
	}
}

func TestIngestJuniper(t *testing.T) {
	pipeline, db := setupPipeline(t)
	content := readTestdata(t, "juniper/teg (1)_20260321162932.log")

	_, err := pipeline.Ingest("juniper_test.log", content)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}

	// Should have exactly 1 device — the one we successfully connected to.
	// CQ-TH-M3103-V06-MX960-BR-601a (failed SSH) should NOT appear.
	if len(devices) != 1 {
		var ids []string
		for _, d := range devices {
			ids = append(ids, d.ID)
		}
		t.Fatalf("expected 1 device, got %d: %v", len(devices), ids)
	}

	d := devices[0]
	if !strings.Contains(d.ID, "sz-bh-0701-j04-mx960-qctix-02") {
		t.Errorf("expected device ID containing 'sz-bh-0701-j04-mx960-qctix-02', got %s", d.ID)
	}
	if d.Vendor != "juniper" {
		t.Errorf("expected vendor=juniper, got %s", d.Vendor)
	}

	configs, err := db.GetConfigSnapshots(d.ID)
	if err != nil {
		t.Fatalf("GetConfigSnapshots: %v", err)
	}
	// Juniper logs contain both "show configuration" (hierarchical) and
	// "show configuration | display set" (set format) → 2 config snapshots.
	if len(configs) != 2 {
		t.Errorf("expected 2 config snapshots (hierarchical + set), got %d", len(configs))
	}

	// Verify no device for the failed SSH target
	_, err = db.GetDevice("cq-th-m3103-v06-mx960-br-601a")
	if err == nil {
		t.Error("device cq-th-m3103-v06-mx960-br-601a should NOT exist (failed SSH)")
	}
}

func TestIngestTimestampExtraction(t *testing.T) {
	pipeline, db := setupPipeline(t)
	content := readTestdata(t, "huawei/teg_20260321162156.log")

	_, err := pipeline.Ingest("timestamp_test.log", content)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) == 0 {
		t.Fatal("no devices found")
	}

	snapID, err := db.LatestSnapshotID(devices[0].ID)
	if err != nil {
		t.Fatalf("LatestSnapshotID: %v", err)
	}
	snap, err := db.GetSnapshot(snapID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}

	if snap.CapturedAt.IsZero() {
		t.Fatal("CapturedAt is zero — timestamp extraction failed")
	}

	// The log timestamps are from 2026-03-21.
	expectedDate := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	if snap.CapturedAt.Year() != expectedDate.Year() ||
		snap.CapturedAt.Month() != expectedDate.Month() ||
		snap.CapturedAt.Day() != expectedDate.Day() {
		t.Errorf("CapturedAt should be on 2026-03-21, got %s", snap.CapturedAt.Format(time.RFC3339))
	}

	// Ensure it's NOT close to time.Now() (should be the log time, not ingest time).
	if time.Since(snap.CapturedAt) < 1*time.Minute {
		t.Errorf("CapturedAt %s is too close to now — should be the log timestamp, not ingest time",
			snap.CapturedAt.Format(time.RFC3339))
	}
}
