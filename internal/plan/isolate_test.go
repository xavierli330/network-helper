package plan

import (
	"strings"
	"testing"
)

func TestBuildIsolationPlan_BasicStructure(t *testing.T) {
	input := PlanInput{
		TargetDevice:   "lc-01",
		TargetHostname: "LC-01",
		TargetVendor:   "huawei",
		Links: []Link{
			{
				LocalDevice:    "lc-01",
				LocalInterface: "GigabitEthernet0/0/0",
				LocalIP:        "10.0.0.1",
				PeerDevice:     "lc-02",
				PeerInterface:  "GigabitEthernet0/0/1",
				PeerIP:         "10.0.0.2",
				Protocols:      []string{"ospf", "ldp"},
				Sources:        []string{"config"},
			},
		},
		IsSPOF:        true,
		ImpactDevices: []string{"lc-03", "lc-04"},
	}

	plan := BuildIsolationPlan(input)

	// Verify 6 phases
	if len(plan.Phases) != 6 {
		t.Fatalf("expected 6 phases, got %d", len(plan.Phases))
	}

	// Verify phase names in order
	wantNames := []string{"方案规划", "预检查", "协议级隔离", "接口级隔离", "变更后检查", "回退方案"}
	for i, want := range wantNames {
		if plan.Phases[i].Name != want {
			t.Errorf("phase[%d] name = %q, want %q", i, plan.Phases[i].Name, want)
		}
	}

	// Verify phase numbers
	for i := range plan.Phases {
		if plan.Phases[i].Number != i {
			t.Errorf("phase[%d].Number = %d, want %d", i, plan.Phases[i].Number, i)
		}
	}

	// Verify GeneratedAt is non-zero
	if plan.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}

	// Verify IsSPOF is preserved
	if !plan.IsSPOF {
		t.Error("IsSPOF should be true")
	}

	// Verify SPOF warning note appears in phase 0
	phase0 := plan.Phases[0]
	foundSPOF := false
	for _, note := range phase0.Notes {
		if strings.Contains(note, "SPOF") {
			foundSPOF = true
			break
		}
	}
	if !foundSPOF {
		t.Error("phase 0 notes should contain SPOF warning when IsSPOF=true")
	}

	// Verify plan fields
	if plan.TargetDevice != "lc-01" {
		t.Errorf("TargetDevice = %q, want %q", plan.TargetDevice, "lc-01")
	}
	if plan.TargetHostname != "LC-01" {
		t.Errorf("TargetHostname = %q, want %q", plan.TargetHostname, "LC-01")
	}
	if plan.TargetVendor != "huawei" {
		t.Errorf("TargetVendor = %q, want %q", plan.TargetVendor, "huawei")
	}
}

func TestBuildIsolationPlan_NoSPOF(t *testing.T) {
	input := PlanInput{
		TargetDevice:  "lc-05",
		TargetHostname: "LC-05",
		TargetVendor:  "huawei",
		Links:         []Link{},
		IsSPOF:        false,
		ImpactDevices: nil,
	}

	plan := BuildIsolationPlan(input)

	if plan.IsSPOF {
		t.Error("IsSPOF should be false")
	}

	// SPOF note should NOT appear in phase 0
	phase0 := plan.Phases[0]
	for _, note := range phase0.Notes {
		if strings.Contains(note, "SPOF") {
			t.Error("phase 0 notes should not contain SPOF warning when IsSPOF=false")
		}
	}
}

func TestBuildIsolationPlan_H3CVendor(t *testing.T) {
	input := PlanInput{
		TargetDevice:   "h3c-01",
		TargetHostname: "H3C-01",
		TargetVendor:   "h3c",
		Links: []Link{
			{
				LocalDevice:    "h3c-01",
				LocalInterface: "GigabitEthernet1/0/0",
				LocalIP:        "192.168.1.1",
				PeerDevice:     "h3c-02",
				PeerInterface:  "GigabitEthernet1/0/1",
				PeerIP:         "192.168.1.2",
				Protocols:      []string{"ospf"},
				Sources:        []string{"config"},
			},
		},
		IsSPOF:        false,
		ImpactDevices: nil,
	}

	plan := BuildIsolationPlan(input)

	if len(plan.Phases) != 6 {
		t.Fatalf("expected 6 phases, got %d", len(plan.Phases))
	}

	// H3C uses "display ospf peer" (without "brief") in Phase 0
	phase0 := plan.Phases[0]
	foundH3COspf := false
	foundHuaweiOspf := false

	for _, step := range phase0.Steps {
		for _, cmd := range step.Commands {
			if cmd == "display ospf peer" {
				foundH3COspf = true
			}
			if cmd == "display ospf peer brief" {
				foundHuaweiOspf = true
			}
		}
	}

	if !foundH3COspf {
		t.Error("H3C phase 0 should contain 'display ospf peer'")
	}
	if foundHuaweiOspf {
		t.Error("H3C phase 0 should NOT contain 'display ospf peer brief'")
	}
}
