package plan

import (
	"strings"
	"testing"
)

// baseCutoverTopo returns a minimal DeviceTopology suitable for cutover plan tests.
func baseCutoverTopo() DeviceTopology {
	return DeviceTopology{
		DeviceID: "qcdr-01",
		Hostname: "QCDR-01",
		Vendor:   "huawei",
		Protocols: []string{"isis", "ldp"},
		IGPs: []IGPInfo{
			{
				Protocol:   "isis",
				ProcessID:  "1",
				Interfaces: []string{"Eth-Trunk1"},
			},
		},
		HasLDP:        true,
		LDPInterfaces: []string{"Eth-Trunk1"},
		PhysicalLinks: []PhysicalLink{},
		LAGs: []LAGBundle{
			{
				Name:        "Eth-Trunk1",
				IP:          "10.1.2.1",
				Mask:        "30",
				Description: "to-peer-router",
				Members:     []string{"GE0/0/0", "GE0/0/1"},
			},
		},
	}
}

func makeCutoverPlan(topo DeviceTopology) Plan {
	return GenerateCutoverPlan(topo, CutoverParams{
		OldInterface: "Eth-Trunk1",
		NewInterface: "Eth-Trunk200",
	})
}

// TestGenerateCutoverPlan_PhaseCount verifies exactly 7 phases are produced.
func TestGenerateCutoverPlan_PhaseCount(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())
	if len(p.Phases) != 7 {
		t.Errorf("expected 7 phases, got %d", len(p.Phases))
	}
}

// TestGenerateCutoverPlan_PhaseNumbers verifies phases are numbered 0–6 in order.
func TestGenerateCutoverPlan_PhaseNumbers(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())
	for i, ph := range p.Phases {
		if ph.Number != i {
			t.Errorf("phase index %d has Number=%d, want %d", i, ph.Number, i)
		}
	}
}

// TestGenerateCutoverPlan_ContainsInterfaces checks that both the old and new
// interface names appear somewhere in the generated plan commands.
func TestGenerateCutoverPlan_ContainsInterfaces(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())

	foundOld := false
	foundNew := false
	for _, ph := range p.Phases {
		for _, step := range ph.Steps {
			for _, cmd := range step.Commands {
				if strings.Contains(cmd, "Eth-Trunk1") {
					foundOld = true
				}
				if strings.Contains(cmd, "Eth-Trunk200") {
					foundNew = true
				}
			}
		}
	}

	if !foundOld {
		t.Error("old interface Eth-Trunk1 not found in any command")
	}
	if !foundNew {
		t.Error("new interface Eth-Trunk200 not found in any command")
	}
}

// TestGenerateCutoverPlan_ConfigNewPhase ensures Phase 2 (index 2) contains
// the new interface declaration and an ip address command.
func TestGenerateCutoverPlan_ConfigNewPhase(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())

	phase2 := p.Phases[2]
	if phase2.Number != 2 {
		t.Fatalf("expected phase 2, got phase %d", phase2.Number)
	}

	allCmds := collectCommands(phase2)

	if !containsSubstring(allCmds, "interface Eth-Trunk200") {
		t.Error("Phase 2 does not contain 'interface Eth-Trunk200'")
	}
	if !containsSubstring(allCmds, "ip address") {
		t.Error("Phase 2 does not contain 'ip address'")
	}
}

// TestGenerateCutoverPlan_ConfigNewPhase_ISISAndLDP checks Phase 2 includes
// isis enable and mpls ldp commands when the old interface carried both.
func TestGenerateCutoverPlan_ConfigNewPhase_ISISAndLDP(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())
	phase2 := p.Phases[2]
	allCmds := collectCommands(phase2)

	if !containsSubstring(allCmds, "isis enable") {
		t.Error("Phase 2 missing 'isis enable' even though old interface ran ISIS")
	}
	if !containsSubstring(allCmds, "mpls ldp") {
		t.Error("Phase 2 missing 'mpls ldp' even though old interface had LDP")
	}
}

// TestGenerateCutoverPlan_ShutOldPhase ensures Phase 5 (index 5) issues a
// shutdown command for the old interface.
func TestGenerateCutoverPlan_ShutOldPhase(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())

	phase5 := p.Phases[5]
	if phase5.Number != 5 {
		t.Fatalf("expected phase 5, got phase %d", phase5.Number)
	}

	allCmds := collectCommands(phase5)

	if !containsSubstring(allCmds, "shutdown") {
		t.Error("Phase 5 does not contain 'shutdown'")
	}
	if !containsSubstring(allCmds, "Eth-Trunk1") {
		t.Error("Phase 5 does not reference old interface Eth-Trunk1")
	}
}

// TestGenerateCutoverPlan_SwitchTrafficPhase verifies Phase 4 sets IGP cost to 65535.
func TestGenerateCutoverPlan_SwitchTrafficPhase(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())

	phase4 := p.Phases[4]
	if phase4.Number != 4 {
		t.Fatalf("expected phase 4, got phase %d", phase4.Number)
	}

	allCmds := collectCommands(phase4)

	if !containsSubstring(allCmds, "65535") {
		t.Error("Phase 4 does not set cost to 65535")
	}
}

// TestGenerateCutoverPlan_PostCheckHasRollback verifies Phase 6 notes contain
// rollback instructions.
func TestGenerateCutoverPlan_PostCheckHasRollback(t *testing.T) {
	p := makeCutoverPlan(baseCutoverTopo())

	phase6 := p.Phases[6]
	rollbackFound := false
	for _, note := range phase6.Notes {
		if strings.Contains(note, "回退") || strings.Contains(note, "undo shutdown") {
			rollbackFound = true
			break
		}
	}
	if !rollbackFound {
		t.Error("Phase 6 notes do not contain rollback instructions")
	}
}

// TestGenerateCutoverPlan_NoOldInterfaceInfo tests the fallback path when the
// old interface is not present in the topology (no IP/config known).
func TestGenerateCutoverPlan_NoOldInterfaceInfo(t *testing.T) {
	topo := baseCutoverTopo()
	topo.LAGs = nil // remove the LAG so old interface isn't found
	topo.IGPs = nil
	topo.HasLDP = false

	p := GenerateCutoverPlan(topo, CutoverParams{
		OldInterface: "Eth-Trunk1",
		NewInterface: "Eth-Trunk200",
	})

	if len(p.Phases) != 7 {
		t.Errorf("expected 7 phases even with unknown old interface, got %d", len(p.Phases))
	}

	// Phase 2 should still reference the new interface
	phase2 := p.Phases[2]
	allCmds := collectCommands(phase2)
	if !containsSubstring(allCmds, "Eth-Trunk200") {
		t.Error("Phase 2 should reference new interface even without old interface config info")
	}
}

// TestGenerateCutoverPlan_Cisco verifies Cisco IOS syntax is used when vendor is "cisco".
func TestGenerateCutoverPlan_Cisco(t *testing.T) {
	topo := baseCutoverTopo()
	topo.Vendor = "cisco"
	// Cisco uses physical links style
	topo.LAGs = nil
	topo.PhysicalLinks = []PhysicalLink{
		{
			Interface:   "Eth-Trunk1",
			IP:          "10.1.2.1",
			Mask:        "255.255.255.252",
			Description: "to-peer",
		},
	}
	topo.IGPs = []IGPInfo{
		{Protocol: "isis", ProcessID: "1", Interfaces: []string{"Eth-Trunk1"}},
	}
	topo.HasLDP = false

	p := GenerateCutoverPlan(topo, CutoverParams{
		OldInterface: "Eth-Trunk1",
		NewInterface: "Eth-Trunk200",
	})

	if len(p.Phases) != 7 {
		t.Errorf("expected 7 phases, got %d", len(p.Phases))
	}

	// Phase 5 shutdown should NOT include "system-view" for Cisco
	phase5 := p.Phases[5]
	allCmds := collectCommands(phase5)
	if containsSubstring(allCmds, "system-view") {
		t.Error("Phase 5 for Cisco should not include 'system-view'")
	}
	if !containsSubstring(allCmds, "shutdown") {
		t.Error("Phase 5 for Cisco should include 'shutdown'")
	}
}

// TestGenerateCutoverPlan_PlanMetadata checks that plan metadata is populated.
func TestGenerateCutoverPlan_PlanMetadata(t *testing.T) {
	topo := baseCutoverTopo()
	p := makeCutoverPlan(topo)

	if p.TargetDevice != "qcdr-01" {
		t.Errorf("TargetDevice = %q, want %q", p.TargetDevice, "qcdr-01")
	}
	if p.TargetHostname != "QCDR-01" {
		t.Errorf("TargetHostname = %q, want %q", p.TargetHostname, "QCDR-01")
	}
	if p.TargetVendor != "huawei" {
		t.Errorf("TargetVendor = %q, want %q", p.TargetVendor, "huawei")
	}
	if p.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// collectCommands flattens all Commands slices from every Step in a Phase.
func collectCommands(ph Phase) []string {
	var all []string
	for _, step := range ph.Steps {
		all = append(all, step.Commands...)
	}
	return all
}

// containsSubstring returns true if any string in cmds contains sub.
func containsSubstring(cmds []string, sub string) bool {
	for _, c := range cmds {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}
