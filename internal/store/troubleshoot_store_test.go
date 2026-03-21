package store

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

func TestInsertAndListTroubleshootLogs(t *testing.T) {
	db := testDB(t)

	log1 := model.TroubleshootLog{
		Symptom: "OSPF neighbor flapping", Findings: "MTU mismatch on GE0/0/1",
		Resolution: "Set MTU to 1500 on both ends", Tags: "ospf,mtu,flap",
	}
	id, err := db.InsertTroubleshootLog(log1)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}

	log2 := model.TroubleshootLog{
		Symptom: "BGP session down", Findings: "ACL blocking TCP 179",
		Resolution: "Updated ACL to permit BGP", Tags: "bgp,acl",
	}
	db.InsertTroubleshootLog(log2)

	logs, err := db.ListTroubleshootLogs(10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2, got %d", len(logs))
	}
	// Most recent first
	if logs[0].Symptom != "BGP session down" {
		t.Errorf("order: %s", logs[0].Symptom)
	}
}

func TestSearchTroubleshootLogs(t *testing.T) {
	db := testDB(t)

	db.InsertTroubleshootLog(model.TroubleshootLog{
		Symptom: "OSPF neighbor flapping", Findings: "MTU mismatch",
		Resolution: "Set MTU to 1500", Tags: "ospf,mtu",
	})
	db.InsertTroubleshootLog(model.TroubleshootLog{
		Symptom: "BGP session down", Findings: "ACL blocking TCP 179",
		Resolution: "Updated ACL", Tags: "bgp,acl",
	})

	// Search for "OSPF"
	results, err := db.SearchTroubleshootLogs("OSPF")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Symptom != "OSPF neighbor flapping" {
		t.Errorf("got: %s", results[0].Symptom)
	}

	// Search for "MTU" (in findings)
	results, _ = db.SearchTroubleshootLogs("MTU")
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}

	// Search for something not present
	results, _ = db.SearchTroubleshootLogs("ISIS")
	if len(results) != 0 {
		t.Errorf("expected 0, got %d", len(results))
	}
}
