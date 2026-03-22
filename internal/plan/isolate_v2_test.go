package plan

import (
	"strings"
	"testing"
)

// testTopology returns a DeviceTopology with:
//   - 3 PeerGroups: LA1 (downlink, 1 peer), QCDR (uplink, 1 peer), SDN (management, 1 peer)
//   - 1 LAGBundle, 1 PhysicalLink
//   - Protocols: ["bgp"]
//   - IsSPOF: true
func testTopology() DeviceTopology {
	return DeviceTopology{
		DeviceID:  "router-01",
		Hostname:  "router-01",
		Vendor:    "huawei",
		LocalAS:   65001,
		Protocols: []string{"bgp"},
		PeerGroups: []PeerGroup{
			{
				Name:    "LA1",
				Type:    "external",
				Role:    RoleDownlink,
				LocalAS: 65001,
				Peers: []BGPPeerDetail{
					{PeerIP: "10.0.1.1", RemoteAS: 65100, Description: "LA1-peer"},
				},
			},
			{
				Name:    "QCDR",
				Type:    "external",
				Role:    RoleUplink,
				LocalAS: 65001,
				Peers: []BGPPeerDetail{
					{PeerIP: "10.0.2.1", RemoteAS: 65200, Description: "QCDR-peer"},
				},
			},
			{
				Name:    "SDN",
				Type:    "internal",
				Role:    RoleManagement,
				LocalAS: 65001,
				Peers: []BGPPeerDetail{
					{PeerIP: "10.0.3.1", RemoteAS: 65001, Description: "SDN-controller"},
				},
			},
		},
		LAGs: []LAGBundle{
			{
				Name:        "Eth-Trunk1",
				IP:          "192.168.1.1",
				Mask:        "30",
				Description: "uplink-lag",
				Members:     []string{"GigabitEthernet0/0/1", "GigabitEthernet0/0/2"},
			},
		},
		PhysicalLinks: []PhysicalLink{
			{
				Interface:   "GigabitEthernet0/0/3",
				IP:          "172.16.1.1",
				Mask:        "30",
				Description: "downlink-phy",
				PeerGroup:   "LA1",
			},
		},
		IsSPOF:        true,
		ImpactDevices: []string{"downstream-01"},
	}
}

func TestGenerateIsolationPlanV2_PhaseCount(t *testing.T) {
	topo := testTopology()
	plan := GenerateIsolationPlanV2(topo)

	if got := len(plan.Phases); got != 6 {
		t.Errorf("expected 6 phases, got %d", got)
	}

	expectedNames := []string{"采集", "预检查", "协议级隔离", "接口级隔离", "变更后检查", "回退方案"}
	for i, name := range expectedNames {
		if plan.Phases[i].Name != name {
			t.Errorf("phase %d: expected name %q, got %q", i, name, plan.Phases[i].Name)
		}
	}
}

func TestGenerateIsolationPlanV2_Phase2_OrderDownlinkFirst(t *testing.T) {
	topo := testTopology()
	plan := GenerateIsolationPlanV2(topo)

	phase2 := plan.Phases[2]
	if len(phase2.Steps) == 0 {
		t.Fatal("phase 2 has no steps")
	}

	// First step should be the BGP isolate for LA1 (downlink)
	firstStep := phase2.Steps[0]
	if !strings.Contains(firstStep.Purpose, "LA1") {
		t.Errorf("expected first step in phase 2 to be LA1 (downlink), got Purpose=%q", firstStep.Purpose)
	}
}

func TestGenerateIsolationPlanV2_Phase2_ContainsPeerIPs(t *testing.T) {
	topo := testTopology()
	plan := GenerateIsolationPlanV2(topo)

	phase2 := plan.Phases[2]

	// Collect all commands from all steps in phase 2
	var allCmds []string
	for _, step := range phase2.Steps {
		allCmds = append(allCmds, step.Commands...)
	}
	joined := strings.Join(allCmds, "\n")

	// Should contain peer IPs from all 3 groups
	for _, ip := range []string{"10.0.1.1", "10.0.2.1", "10.0.3.1"} {
		if !strings.Contains(joined, ip) {
			t.Errorf("phase 2 commands missing peer IP %s", ip)
		}
	}

	// Should contain bgp <localAS> block header
	if !strings.Contains(joined, "bgp 65001") {
		t.Errorf("phase 2 commands missing 'bgp 65001' block")
	}
}

func TestGenerateIsolationPlanV2_Phase0_ProtocolSpecific(t *testing.T) {
	topo := testTopology() // protocols: ["bgp"] only
	plan := GenerateIsolationPlanV2(topo)

	phase0 := plan.Phases[0]

	var allCmds []string
	for _, step := range phase0.Steps {
		allCmds = append(allCmds, step.Commands...)
	}
	joined := strings.Join(allCmds, "\n")

	// BGP protocol → should have display bgp peer
	if !strings.Contains(joined, "display bgp peer") {
		t.Errorf("phase 0 should contain 'display bgp peer' for bgp protocol, got:\n%s", joined)
	}

	// No OSPF in protocols → should NOT have display ospf peer
	if strings.Contains(joined, "display ospf peer") {
		t.Errorf("phase 0 should NOT contain 'display ospf peer' when ospf is not in protocols, got:\n%s", joined)
	}
}

func TestGenerateIsolationPlanV2_Phase5_ReverseOrder(t *testing.T) {
	topo := testTopology()
	plan := GenerateIsolationPlanV2(topo)

	phase5 := plan.Phases[5]
	if len(phase5.Steps) == 0 {
		t.Fatal("phase 5 has no steps")
	}

	// First BGP rollback step should be SDN (management) — reverse of downlink-first order
	firstStep := phase5.Steps[0]
	if !strings.Contains(firstStep.Purpose, "SDN") {
		t.Errorf("expected first rollback step in phase 5 to be SDN (management), got Purpose=%q", firstStep.Purpose)
	}
}

func TestGenerateIsolationPlanV2_WithISIS(t *testing.T) {
	topo := DeviceTopology{
		DeviceID: "qcdr-01", Hostname: "QCDR-01", Vendor: "huawei",
		LocalAS:   45090,
		Protocols: []string{"isis", "ldp", "bgp"},
		IGPs:      []IGPInfo{{Protocol: "isis", ProcessID: "1", Interfaces: []string{"Eth-Trunk1"}}},
		HasLDP:    true, LDPInterfaces: []string{"Eth-Trunk1"},
		PeerGroups: []PeerGroup{{Name: "PEERS", Type: "external", Role: RoleDownlink,
			Peers: []BGPPeerDetail{{PeerIP: "10.0.0.1", RemoteAS: 65508}}}},
	}
	p := GenerateIsolationPlanV2(topo)

	phase2Text := ""
	for _, s := range p.Phases[2].Steps {
		phase2Text += s.Purpose + "\n"
	}
	// ISIS should come before BGP
	if !strings.Contains(phase2Text, "ISIS") {
		t.Error("missing ISIS step in phase 2")
	}
	if !strings.Contains(phase2Text, "LDP") {
		t.Error("missing LDP step in phase 2")
	}
	if !strings.Contains(phase2Text, "BGP") {
		t.Error("missing BGP step in phase 2")
	}

	isisIdx := strings.Index(phase2Text, "ISIS")
	bgpIdx := strings.Index(phase2Text, "BGP")
	if isisIdx > bgpIdx {
		t.Error("ISIS should come before BGP in phase 2")
	}

	ldpIdx := strings.Index(phase2Text, "LDP")
	if ldpIdx > bgpIdx {
		t.Error("LDP should come before BGP in phase 2")
	}
}

func TestGenerateIsolationPlanV2_WithISIS_Phase5_HasIGPRollback(t *testing.T) {
	topo := DeviceTopology{
		DeviceID: "qcdr-01", Hostname: "QCDR-01", Vendor: "huawei",
		LocalAS:   45090,
		Protocols: []string{"isis", "ldp", "bgp"},
		IGPs:      []IGPInfo{{Protocol: "isis", ProcessID: "1", Interfaces: []string{"Eth-Trunk1"}}},
		HasLDP:    true, LDPInterfaces: []string{"Eth-Trunk1"},
		PeerGroups: []PeerGroup{{Name: "PEERS", Type: "external", Role: RoleDownlink,
			Peers: []BGPPeerDetail{{PeerIP: "10.0.0.1", RemoteAS: 65508}}}},
	}
	p := GenerateIsolationPlanV2(topo)

	phase5Text := ""
	for _, s := range p.Phases[5].Steps {
		phase5Text += s.Purpose + "\n"
	}

	if !strings.Contains(phase5Text, "ISIS 回退") {
		t.Error("phase 5 should contain ISIS rollback step")
	}
	if !strings.Contains(phase5Text, "LDP 回退") {
		t.Error("phase 5 should contain LDP rollback step")
	}
}
