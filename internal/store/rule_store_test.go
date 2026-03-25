// internal/store/rule_store_test.go
package store_test

import (
	"testing"
	"time"
	"github.com/xavierli/nethelper/internal/store"
)

func TestUpsertUnknownOutput(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	entry := store.UnknownOutput{
		DeviceID: "dev1", Vendor: "huawei",
		CommandRaw: "dis int brief", CommandNorm: "display interface brief",
		RawOutput: "PHY...", ContentHash: "hash1",
	}
	if err := db.UpsertUnknownOutput(entry); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertUnknownOutput(entry); err != nil { // duplicate
		t.Fatal(err)
	}

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].OccurrenceCount != 2 {
		t.Fatalf("want occurrence_count=2, got %d", rows[0].OccurrenceCount)
	}
}

func TestCreateAndGetPendingRule(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	id, err := db.CreatePendingRule(store.PendingRule{
		Vendor: "huawei", CommandPattern: "display traffic-policy",
		OutputType: "table", SampleInputs: `["sample1"]`, Status: "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := db.GetPendingRule(id)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Vendor != "huawei" {
		t.Errorf("want vendor=huawei, got %s", rule.Vendor)
	}
}

func TestApprovePendingRule(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	id, _ := db.CreatePendingRule(store.PendingRule{
		Vendor: "huawei", CommandPattern: "display qos",
		OutputType: "table", SampleInputs: "[]", Status: "draft",
	})
	now := time.Now()
	if err := db.ApprovePendingRule(id, "testuser", now); err != nil {
		t.Fatal(err)
	}
	rule, _ := db.GetPendingRule(id)
	if rule.Status != "approved" {
		t.Errorf("want status=approved, got %s", rule.Status)
	}
	if rule.ApprovedBy != "testuser" {
		t.Errorf("want approved_by=testuser, got %s", rule.ApprovedBy)
	}
	if rule.ApprovedAt == nil {
		t.Error("want approved_at set")
	}
}

func TestRuleStoreTables(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO unknown_outputs
		(device_id, vendor, command_raw, command_norm, raw_output, content_hash)
		VALUES ('d1','huawei','dis int','display interface','output','abc123')`)
	if err != nil {
		t.Errorf("unknown_outputs insert: %v", err)
	}

	_, err = db.Exec(`INSERT INTO pending_rules
		(vendor, command_pattern, output_type, sample_inputs)
		VALUES ('huawei','display traffic-policy','table','[]')`)
	if err != nil {
		t.Errorf("pending_rules insert: %v", err)
	}

	var ruleID int
	db.QueryRow(`SELECT id FROM pending_rules LIMIT 1`).Scan(&ruleID)
	_, err = db.Exec(`INSERT INTO rule_test_cases (rule_id, input, expected) VALUES (?, 'raw', '{}')`, ruleID)
	if err != nil {
		t.Errorf("rule_test_cases insert: %v", err)
	}
}
