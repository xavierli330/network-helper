// internal/store/rule_store_test.go
package store_test

import (
	"testing"
	"github.com/xavierli/nethelper/internal/store"
)

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
