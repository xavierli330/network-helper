package plan

import (
	"strings"
	"testing"
)

func testUpgradeTopo() DeviceTopology {
	return DeviceTopology{
		DeviceID: "rtr-01", Hostname: "RTR-01", Vendor: "huawei",
		Protocols: []string{"bgp"},
		LocalAS:   100,
		PeerGroups: []PeerGroup{
			{
				Name: "PEERS", Type: "external", Role: RoleDownlink,
				Peers: []BGPPeerDetail{{PeerIP: "10.0.0.1", RemoteAS: 200}},
			},
		},
	}
}

func TestGenerateUpgradePlan_PhaseCount(t *testing.T) {
	topo := testUpgradeTopo()
	params := UpgradeParams{TargetVersion: "V200R021", FirmwareFile: "firmware.cc"}
	p := GenerateUpgradePlan(topo, params)

	if len(p.Phases) != 8 {
		t.Fatalf("expected 8 phases, got %d", len(p.Phases))
	}
}

func TestGenerateUpgradePlan_PhaseNumbers(t *testing.T) {
	topo := testUpgradeTopo()
	params := UpgradeParams{TargetVersion: "V200R021", FirmwareFile: "firmware.cc"}
	p := GenerateUpgradePlan(topo, params)

	for i, phase := range p.Phases {
		if phase.Number != i {
			t.Errorf("phase[%d].Number = %d, want %d", i, phase.Number, i)
		}
	}
}

func TestGenerateUpgradePlan_UpgradePhase(t *testing.T) {
	topo := testUpgradeTopo()
	params := UpgradeParams{TargetVersion: "V200R021", FirmwareFile: "firmware.cc"}
	p := GenerateUpgradePlan(topo, params)

	// Phase 4 should be upgrade execution
	if p.Phases[4].Name != "升级执行" {
		t.Errorf("phase 4 name = %s, want 升级执行", p.Phases[4].Name)
	}

	// Phase 4 description should mention the target version
	if !strings.Contains(p.Phases[4].Description, params.TargetVersion) {
		t.Errorf("phase 4 description should mention target version %s, got: %s",
			params.TargetVersion, p.Phases[4].Description)
	}

	// Phase 4 should have steps
	if len(p.Phases[4].Steps) == 0 {
		t.Errorf("phase 4 should have steps")
	}

	// Phase 4 steps should contain huawei upgrade commands
	found := false
	for _, step := range p.Phases[4].Steps {
		for _, cmd := range step.Commands {
			if strings.Contains(cmd, "startup system-software") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("phase 4 should contain 'startup system-software' for huawei")
	}
}

func TestGenerateUpgradePlan_VerifyPhase(t *testing.T) {
	topo := testUpgradeTopo()
	params := UpgradeParams{TargetVersion: "V200R021", FirmwareFile: "firmware.cc"}
	p := GenerateUpgradePlan(topo, params)

	// Phase 5 should be upgrade verification
	if p.Phases[5].Name != "升级验证" {
		t.Errorf("phase 5 name = %s, want 升级验证", p.Phases[5].Name)
	}

	// Phase 5 steps should contain version check
	hasDisplayVersion := false
	hasVersionRef := false
	for _, step := range p.Phases[5].Steps {
		for _, cmd := range step.Commands {
			if cmd == "display version" {
				hasDisplayVersion = true
			}
			if strings.Contains(cmd, params.TargetVersion) {
				hasVersionRef = true
			}
		}
	}
	if !hasDisplayVersion {
		t.Errorf("phase 5 should contain 'display version'")
	}
	if !hasVersionRef {
		t.Errorf("phase 5 should reference target version %s", params.TargetVersion)
	}
}

func TestGenerateUpgradePlan_ContainsVersion(t *testing.T) {
	topo := testUpgradeTopo()
	params := UpgradeParams{TargetVersion: "V200R021C10SPC600", FirmwareFile: "NE40E-firmware.cc"}
	p := GenerateUpgradePlan(topo, params)

	// Phase 1 (pre-check) notes should mention target version
	phase1Notes := strings.Join(p.Phases[1].Notes, " ")
	if !strings.Contains(phase1Notes, params.TargetVersion) {
		t.Errorf("phase 1 notes should mention target version %s", params.TargetVersion)
	}

	// Phase 1 notes should mention firmware file
	if !strings.Contains(phase1Notes, params.FirmwareFile) {
		t.Errorf("phase 1 notes should mention firmware file %s", params.FirmwareFile)
	}
}

func TestGenerateUpgradePlan_RecoveryPhase(t *testing.T) {
	topo := testUpgradeTopo()
	params := UpgradeParams{TargetVersion: "V200R021", FirmwareFile: "firmware.cc"}
	p := GenerateUpgradePlan(topo, params)

	// Phase 6 should be traffic recovery
	if p.Phases[6].Name != "恢复流量" {
		t.Errorf("phase 6 name = %s, want 恢复流量", p.Phases[6].Name)
	}
}

func TestGenerateUpgradePlan_MetaFields(t *testing.T) {
	topo := testUpgradeTopo()
	params := UpgradeParams{TargetVersion: "V200R021", FirmwareFile: "firmware.cc"}
	p := GenerateUpgradePlan(topo, params)

	if p.TargetDevice != topo.DeviceID {
		t.Errorf("TargetDevice = %s, want %s", p.TargetDevice, topo.DeviceID)
	}
	if p.TargetHostname != topo.Hostname {
		t.Errorf("TargetHostname = %s, want %s", p.TargetHostname, topo.Hostname)
	}
	if p.TargetVendor != topo.Vendor {
		t.Errorf("TargetVendor = %s, want %s", p.TargetVendor, topo.Vendor)
	}
	if p.GeneratedAt.IsZero() {
		t.Errorf("GeneratedAt should not be zero")
	}
}
