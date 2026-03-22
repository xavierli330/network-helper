package plan

import (
	"strings"
	"testing"
)

// allCommandText joins all commands across all DeviceCommands into a single string for easy assertion.
func allCommandText(cmds []DeviceCommand) string {
	var parts []string
	for _, dc := range cmds {
		parts = append(parts, dc.Commands...)
	}
	return strings.Join(parts, "\n")
}

// --- Test fixtures ---

var testLinksOSPFBGPLDP = []Link{
	{
		LocalDevice:    "router-a",
		LocalInterface: "GigabitEthernet0/0/0",
		LocalIP:        "10.0.0.1",
		PeerDevice:     "router-b",
		PeerInterface:  "GigabitEthernet0/0/1",
		PeerIP:         "10.0.0.2",
		Protocols:      []string{"ospf", "bgp", "ldp"},
	},
}

var testLinksOSPFOnly = []Link{
	{
		LocalDevice:    "router-a",
		LocalInterface: "GigabitEthernet0/0/0",
		LocalIP:        "10.0.0.1",
		PeerDevice:     "router-b",
		PeerIP:         "10.0.0.2",
		Protocols:      []string{"ospf"},
	},
}

var testLinksTwoPeers = []Link{
	{
		LocalDevice:    "router-a",
		LocalInterface: "GigabitEthernet0/0/0",
		LocalIP:        "10.0.0.1",
		PeerDevice:     "router-b",
		PeerIP:         "10.0.0.2",
		Protocols:      []string{"ospf", "bgp"},
	},
	{
		LocalDevice:    "router-a",
		LocalInterface: "GigabitEthernet0/0/1",
		LocalIP:        "10.1.0.1",
		PeerDevice:     "router-c",
		PeerIP:         "10.1.0.2",
		Protocols:      []string{"ospf"},
	},
}

var testLinksDupInterface = []Link{
	{
		LocalDevice:    "router-a",
		LocalInterface: "GigabitEthernet0/0/0",
		LocalIP:        "10.0.0.1",
		PeerDevice:     "router-b",
		PeerIP:         "10.0.0.2",
		Protocols:      []string{"ospf"},
	},
	{
		LocalDevice:    "router-a",
		LocalInterface: "GigabitEthernet0/0/0", // same interface, different peer
		LocalIP:        "10.0.0.1",
		PeerDevice:     "router-c",
		PeerIP:         "10.0.0.3",
		Protocols:      []string{"bgp"},
	},
}

var testLinksNoPeerIP = []Link{
	{
		LocalDevice:    "router-a",
		LocalInterface: "GigabitEthernet0/0/0",
		LocalIP:        "10.0.0.1",
		PeerDevice:     "router-b",
		PeerIP:         "", // empty — BGP should be skipped
		Protocols:      []string{"bgp"},
	},
}

// --- Huawei tests ---

func TestHuaweiGenerator_CollectionCommands(t *testing.T) {
	g := &HuaweiGenerator{}
	cmds := g.CollectionCommands("router-a", testLinksOSPFBGPLDP)

	if len(cmds) == 0 {
		t.Fatal("expected at least one DeviceCommand")
	}

	text := allCommandText(cmds)

	// Target device commands must include all protocol displays and new mandatory commands.
	for _, want := range []string{
		"display interface brief",
		"display ip routing-table statistics",
		"display ospf peer brief",
		"display bgp peer",
		"display mpls ldp session",
		"display current-configuration",
		"display lldp neighbor brief",
		"display version",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing command %q in collection commands", want)
		}
	}

	// Must also include a DeviceCommand for the peer device.
	hasPeer := false
	for _, dc := range cmds {
		if dc.DeviceID == "router-b" {
			hasPeer = true
			break
		}
	}
	if !hasPeer {
		t.Error("expected a DeviceCommand for peer device router-b")
	}
}

func TestHuaweiGenerator_ProtocolIsolateCommands(t *testing.T) {
	g := &HuaweiGenerator{}
	cmds := g.ProtocolIsolateCommands("router-a", testLinksOSPFBGPLDP)

	text := allCommandText(cmds)

	for _, want := range []string{
		"system-view",
		"interface GigabitEthernet0/0/0",
		"ospf cost 65535",
		"peer 10.0.0.2 ignore",
		"return",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing command %q in protocol isolate commands", want)
		}
	}
}

func TestHuaweiGenerator_InterfaceIsolateCommands(t *testing.T) {
	g := &HuaweiGenerator{}
	cmds := g.InterfaceIsolateCommands("router-a", testLinksTwoPeers)

	text := allCommandText(cmds)

	for _, want := range []string{
		"system-view",
		"interface GigabitEthernet0/0/0",
		"shutdown",
		"interface GigabitEthernet0/0/1",
		"return",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing command %q in interface isolate commands", want)
		}
	}
}

func TestHuaweiGenerator_RollbackCommands(t *testing.T) {
	g := &HuaweiGenerator{}
	cmds := g.RollbackCommands("router-a", testLinksOSPFBGPLDP)

	text := allCommandText(cmds)

	for _, want := range []string{
		"undo shutdown",
		"undo ospf cost",
		"undo peer 10.0.0.2 ignore",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing command %q in rollback commands", want)
		}
	}

	// Verify correct rollback order: protocols must come before interface undo shutdown.
	protoIdx := strings.Index(text, "undo ospf cost")
	ifaceIdx := strings.Index(text, "undo shutdown")
	if protoIdx == -1 || ifaceIdx == -1 {
		t.Fatal("could not find expected rollback commands for ordering check")
	}
	if protoIdx >= ifaceIdx {
		t.Errorf("rollback order wrong: 'undo ospf cost' (proto) should appear before 'undo shutdown' (iface); got proto@%d iface@%d", protoIdx, ifaceIdx)
	}

	// Verify purpose ordering: first DeviceCommand should be protocol restore.
	if len(cmds) < 2 {
		t.Fatalf("expected 2 DeviceCommands in rollback, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0].Purpose, "protocol") {
		t.Errorf("first rollback DeviceCommand should be protocol restore, got purpose: %q", cmds[0].Purpose)
	}
	if !strings.Contains(cmds[1].Purpose, "interface") {
		t.Errorf("second rollback DeviceCommand should be interface re-enable, got purpose: %q", cmds[1].Purpose)
	}
}

func TestHuaweiGenerator_DedupInterfaces(t *testing.T) {
	g := &HuaweiGenerator{}
	cmds := g.InterfaceIsolateCommands("router-a", testLinksDupInterface)

	text := allCommandText(cmds)

	// Count occurrences of "shutdown" — should be exactly 1.
	count := strings.Count(text, "\nshutdown")
	// Also account for "shutdown" being the first element (no leading newline).
	if strings.HasPrefix(text, "shutdown") {
		count++
	}
	// Simpler: count all occurrences of the literal line.
	allLines := strings.Split(text, "\n")
	shutdowns := 0
	for _, line := range allLines {
		if strings.TrimSpace(line) == "shutdown" {
			shutdowns++
		}
	}
	if shutdowns != 1 {
		t.Errorf("expected exactly 1 shutdown command for duplicate interfaces, got %d\nfull output:\n%s", shutdowns, text)
	}
}

func TestHuaweiGenerator_BGPSkippedWhenNoPeerIP(t *testing.T) {
	g := &HuaweiGenerator{}
	cmds := g.ProtocolIsolateCommands("router-a", testLinksNoPeerIP)

	text := allCommandText(cmds)

	// "peer  ignore" (with double space) must NOT appear — indicates empty PeerIP was used.
	if strings.Contains(text, "peer  ignore") {
		t.Errorf("BGP peer ignore command should be skipped when PeerIP is empty, but found it: %q", text)
	}
}

// --- H3C tests ---

func TestH3CGenerator_CollectionCommands(t *testing.T) {
	g := &H3CGenerator{}
	cmds := g.CollectionCommands("router-a", testLinksOSPFOnly)

	text := allCommandText(cmds)

	// H3C uses "display ospf peer" (without "brief").
	if !strings.Contains(text, "display ospf peer") {
		t.Errorf("expected 'display ospf peer' in H3C collection commands, got:\n%s", text)
	}
	if strings.Contains(text, "display ospf peer brief") {
		t.Errorf("H3C should NOT use 'display ospf peer brief', but found it in:\n%s", text)
	}

	// Must include the new mandatory collection commands.
	for _, want := range []string{
		"display current-configuration",
		"display lldp neighbor brief",
		"display version",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing command %q in H3C collection commands", want)
		}
	}
}

func TestH3CGenerator_ProtocolIsolateCommands(t *testing.T) {
	g := &H3CGenerator{}
	cmds := g.ProtocolIsolateCommands("router-a", testLinksOSPFBGPLDP)

	text := allCommandText(cmds)

	for _, want := range []string{
		"system-view",
		"ospf cost 65535",
		"peer 10.0.0.2 ignore",
		"return",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing command %q in H3C protocol isolate commands", want)
		}
	}
}

func TestH3CGenerator_RollbackCommands(t *testing.T) {
	g := &H3CGenerator{}
	cmds := g.RollbackCommands("router-a", testLinksOSPFBGPLDP)

	text := allCommandText(cmds)

	for _, want := range []string{
		"undo shutdown",
		"undo ospf cost",
		"undo peer 10.0.0.2 ignore",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing command %q in H3C rollback commands", want)
		}
	}

	// Verify correct rollback order: protocols must come before interface undo shutdown.
	protoIdx := strings.Index(text, "undo ospf cost")
	ifaceIdx := strings.Index(text, "undo shutdown")
	if protoIdx == -1 || ifaceIdx == -1 {
		t.Fatal("could not find expected rollback commands for ordering check")
	}
	if protoIdx >= ifaceIdx {
		t.Errorf("rollback order wrong: 'undo ospf cost' (proto) should appear before 'undo shutdown' (iface); got proto@%d iface@%d", protoIdx, ifaceIdx)
	}

	// Verify purpose ordering: first DeviceCommand should be protocol restore.
	if len(cmds) < 2 {
		t.Fatalf("expected 2 DeviceCommands in H3C rollback, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0].Purpose, "protocol") {
		t.Errorf("first H3C rollback DeviceCommand should be protocol restore, got purpose: %q", cmds[0].Purpose)
	}
	if !strings.Contains(cmds[1].Purpose, "interface") {
		t.Errorf("second H3C rollback DeviceCommand should be interface re-enable, got purpose: %q", cmds[1].Purpose)
	}
}

// --- GeneratorForVendor tests ---

func TestGeneratorForVendor(t *testing.T) {
	tests := []struct {
		vendor   string
		wantType string
	}{
		{"huawei", "*plan.HuaweiGenerator"},
		{"h3c", "*plan.H3CGenerator"},
		{"cisco", "*plan.HuaweiGenerator"},  // unknown falls back
		{"juniper", "*plan.HuaweiGenerator"}, // unknown falls back
		{"", "*plan.HuaweiGenerator"},        // empty falls back
	}

	for _, tt := range tests {
		gen := GeneratorForVendor(tt.vendor)
		if gen == nil {
			t.Errorf("GeneratorForVendor(%q) returned nil", tt.vendor)
			continue
		}
		// Use a type switch to check the concrete type.
		var gotType string
		switch gen.(type) {
		case *HuaweiGenerator:
			gotType = "*plan.HuaweiGenerator"
		case *H3CGenerator:
			gotType = "*plan.H3CGenerator"
		default:
			gotType = "unknown"
		}
		if gotType != tt.wantType {
			t.Errorf("GeneratorForVendor(%q) = %s, want %s", tt.vendor, gotType, tt.wantType)
		}
	}
}
