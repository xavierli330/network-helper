package plan

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// bgpIsolateStep
// ---------------------------------------------------------------------------

func TestBGPIsolateStep_H3C(t *testing.T) {
	pg := PeerGroup{
		Name: "DOWNSTREAM",
		Type: "external",
		Role: RoleDownlink,
		Peers: []BGPPeerDetail{
			{PeerIP: "10.0.0.1", RemoteAS: 65508, Description: "peer-A"},
			{PeerIP: "10.0.0.2", RemoteAS: 65508, Description: "peer-B"},
		},
	}

	dc := bgpIsolateStep("dev-h3c", 65500, pg, "h3c")

	if dc.Vendor != "h3c" {
		t.Errorf("expected vendor h3c, got %s", dc.Vendor)
	}

	// Must open with system-view and bgp block
	if dc.Commands[0] != "system-view" {
		t.Errorf("first command must be system-view, got %q", dc.Commands[0])
	}
	if dc.Commands[1] != "bgp 65500" {
		t.Errorf("second command must be 'bgp 65500', got %q", dc.Commands[1])
	}

	// Must close with quit + return
	n := len(dc.Commands)
	if dc.Commands[n-2] != "quit" {
		t.Errorf("second-to-last must be quit, got %q", dc.Commands[n-2])
	}
	if dc.Commands[n-1] != "return" {
		t.Errorf("last command must be return, got %q", dc.Commands[n-1])
	}

	// Both peers must have "peer X ignore" lines with their descriptions
	if !containsLinePrefix(dc.Commands, "peer 10.0.0.1 ignore") {
		t.Error("expected 'peer 10.0.0.1 ignore' in commands")
	}
	if !containsLinePrefix(dc.Commands, "peer 10.0.0.2 ignore") {
		t.Error("expected 'peer 10.0.0.2 ignore' in commands")
	}
	// Descriptions should appear as inline comments
	found := false
	for _, c := range dc.Commands {
		if strings.HasPrefix(c, "peer 10.0.0.1 ignore") && strings.Contains(c, "# peer-A") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '# peer-A' comment on peer 10.0.0.1 line, commands: %v", dc.Commands)
	}

	// Purpose should mention peer count and single AS
	if !strings.Contains(dc.Purpose, "DOWNSTREAM") {
		t.Errorf("purpose must contain group name, got: %s", dc.Purpose)
	}
	if !strings.Contains(dc.Purpose, "2 peers") {
		t.Errorf("purpose must contain '2 peers', got: %s", dc.Purpose)
	}
	if !strings.Contains(dc.Purpose, "65508") {
		t.Errorf("purpose must mention AS 65508, got: %s", dc.Purpose)
	}
}

func TestBGPIsolateStep_Huawei(t *testing.T) {
	pg := PeerGroup{
		Name: "UPSTREAM",
		Type: "external",
		Role: RoleUplink,
		Peers: []BGPPeerDetail{
			{PeerIP: "192.168.1.1", RemoteAS: 100, Description: "transit-provider"},
		},
	}

	dc := bgpIsolateStep("dev-vrp", 100, pg, "huawei")

	if dc.Vendor != "huawei" {
		t.Errorf("expected vendor huawei, got %s", dc.Vendor)
	}
	if dc.Commands[1] != "bgp 100" {
		t.Errorf("expected 'bgp 100', got %q", dc.Commands[1])
	}
	if !containsLinePrefix(dc.Commands, "peer 192.168.1.1 ignore") {
		t.Error("expected 'peer 192.168.1.1 ignore' in commands")
	}
	if !strings.Contains(dc.Purpose, "BGP 隔离") {
		t.Errorf("purpose must contain 'BGP 隔离', got: %s", dc.Purpose)
	}
	if !strings.Contains(dc.Purpose, "1 peers") {
		t.Errorf("purpose must contain '1 peers', got: %s", dc.Purpose)
	}
}

// ---------------------------------------------------------------------------
// bgpCheckpoint
// ---------------------------------------------------------------------------

func TestBGPCheckpoint(t *testing.T) {
	pg := PeerGroup{
		Name: "PEER-GROUP-A",
		Peers: []BGPPeerDetail{
			{PeerIP: "172.16.0.1", RemoteAS: 200},
			{PeerIP: "172.16.0.2", RemoteAS: 201},
		},
	}

	dc := bgpCheckpoint("dev-check", pg, "huawei")

	// Per-peer display commands
	if !containsLine(dc.Commands, "display bgp peer 172.16.0.1") {
		t.Error("expected 'display bgp peer 172.16.0.1'")
	}
	if !containsLine(dc.Commands, "display bgp peer 172.16.0.2") {
		t.Error("expected 'display bgp peer 172.16.0.2'")
	}

	// Summary display commands must appear
	if !containsLine(dc.Commands, "display bgp peer | include Established") {
		t.Error("expected 'display bgp peer | include Established'")
	}
	if !containsLine(dc.Commands, "display bgp routing-table statistics") {
		t.Error("expected 'display bgp routing-table statistics'")
	}

	// Purpose must reference group name and Idle expectation
	if !strings.Contains(dc.Purpose, "PEER-GROUP-A") {
		t.Errorf("purpose must contain group name, got: %s", dc.Purpose)
	}
	if !strings.Contains(dc.Purpose, "Idle") {
		t.Errorf("purpose must mention 'Idle', got: %s", dc.Purpose)
	}
	// Checkpoint marker brackets
	if !strings.HasPrefix(dc.Purpose, ">>>") {
		t.Errorf("purpose must start with '>>>', got: %s", dc.Purpose)
	}
}

// ---------------------------------------------------------------------------
// bgpRollbackStep
// ---------------------------------------------------------------------------

func TestBGPRollbackStep(t *testing.T) {
	pg := PeerGroup{
		Name: "DOWNSTREAM",
		Type: "external",
		Role: RoleDownlink,
		Peers: []BGPPeerDetail{
			{PeerIP: "10.1.1.1", RemoteAS: 65509, Description: "down-peer-X"},
			{PeerIP: "10.1.1.2", RemoteAS: 65510, Description: ""},
		},
	}

	dc := bgpRollbackStep("dev-rollback", 65500, pg, "huawei")

	// Structural framing
	if dc.Commands[0] != "system-view" {
		t.Errorf("first command must be system-view, got %q", dc.Commands[0])
	}
	if dc.Commands[1] != "bgp 65500" {
		t.Errorf("second command must be 'bgp 65500', got %q", dc.Commands[1])
	}
	n := len(dc.Commands)
	if dc.Commands[n-2] != "quit" || dc.Commands[n-1] != "return" {
		t.Errorf("expected quit+return at end, got %q %q", dc.Commands[n-2], dc.Commands[n-1])
	}

	// Must use "undo peer X ignore" not "peer X ignore"
	if !containsLinePrefix(dc.Commands, "undo peer 10.1.1.1 ignore") {
		t.Error("expected 'undo peer 10.1.1.1 ignore'")
	}
	if !containsLinePrefix(dc.Commands, "undo peer 10.1.1.2 ignore") {
		t.Error("expected 'undo peer 10.1.1.2 ignore'")
	}

	// Peer with description should carry comment
	found := false
	for _, c := range dc.Commands {
		if strings.HasPrefix(c, "undo peer 10.1.1.1 ignore") && strings.Contains(c, "# down-peer-X") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected description comment on undo line, commands: %v", dc.Commands)
	}

	// Peer without description must NOT carry a comment
	for _, c := range dc.Commands {
		if strings.HasPrefix(c, "undo peer 10.1.1.2 ignore") && strings.Contains(c, "#") {
			t.Errorf("peer without description must not have # comment, got: %q", c)
		}
	}

	// Purpose must contain 回退/恢复 and peer count
	if !strings.Contains(dc.Purpose, "BGP 回退") {
		t.Errorf("purpose must contain 'BGP 回退', got: %s", dc.Purpose)
	}
	if !strings.Contains(dc.Purpose, "2 peers") {
		t.Errorf("purpose must contain '2 peers', got: %s", dc.Purpose)
	}
}

// ---------------------------------------------------------------------------
// formatASList
// ---------------------------------------------------------------------------

func TestFormatASList_Single(t *testing.T) {
	pg := PeerGroup{
		Peers: []BGPPeerDetail{
			{RemoteAS: 1001},
			{RemoteAS: 1001},
			{RemoteAS: 1001},
		},
	}
	result := formatASList(pg)
	if result != "1001" {
		t.Errorf("expected '1001', got %q", result)
	}
}

func TestFormatASList_Multiple(t *testing.T) {
	pg := PeerGroup{
		Peers: []BGPPeerDetail{
			{RemoteAS: 100},
			{RemoteAS: 200},
			{RemoteAS: 300},
			{RemoteAS: 400},
			{RemoteAS: 500},
		},
	}
	result := formatASList(pg)
	if result != "5 ASes" {
		t.Errorf("expected '5 ASes', got %q", result)
	}
}
