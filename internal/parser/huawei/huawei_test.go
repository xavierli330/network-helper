package huawei

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

func TestHuaweiVendor(t *testing.T) {
	if New().Vendor() != "huawei" {
		t.Error("expected huawei")
	}
}

func TestHuaweiDetectPrompt(t *testing.T) {
	p := New()
	tests := []struct {
		line, wantHost string
		wantOK         bool
	}{
		{"<HUAWEI-Core-01>display version", "HUAWEI-Core-01", true},
		{"[HUAWEI-Core-01]interface GE0/0/1", "HUAWEI-Core-01", true},
		{"Router-PE01#show version", "", false},
	}
	for _, tt := range tests {
		host, ok := p.DetectPrompt(tt.line)
		if ok != tt.wantOK || host != tt.wantHost {
			t.Errorf("line %q: got (%q,%v), want (%q,%v)", tt.line, host, ok, tt.wantHost, tt.wantOK)
		}
	}
}

func TestParseInterfaceBrief(t *testing.T) {
	input := "Interface                   PHY   Protocol InUti OutUti   inErr  outErr\nGE0/0/1                     up    up       0.5%  0.3%         0       0\nGE0/0/2                     down  down     0%    0%           0       0\nLoopBack0                   up    up(s)    --    --           0       0\nEth-Trunk1                  up    up       1.2%  0.8%         0       0\nVlanif100                   up    up       --    --           0       0\nNULL0                       up    up(s)    --    --           0       0"

	result, err := ParseInterfaceBrief(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.Interfaces) != 6 {
		t.Fatalf("expected 6, got %d", len(result.Interfaces))
	}

	checks := []struct {
		idx    int
		name   string
		typ    model.InterfaceType
		status string
	}{
		{0, "GE0/0/1", model.IfTypePhysical, "up"},
		{1, "GE0/0/2", model.IfTypePhysical, "down"},
		{2, "LoopBack0", model.IfTypeLoopback, "up"},
		{3, "Eth-Trunk1", model.IfTypeEthTrunk, "up"},
		{4, "Vlanif100", model.IfTypeVlanif, "up"},
		{5, "NULL0", model.IfTypeNull, "up"},
	}
	for _, c := range checks {
		iface := result.Interfaces[c.idx]
		if iface.Name != c.name {
			t.Errorf("[%d] name: got %s, want %s", c.idx, iface.Name, c.name)
		}
		if iface.Type != c.typ {
			t.Errorf("[%d] type: got %s, want %s", c.idx, iface.Type, c.typ)
		}
		if iface.Status != c.status {
			t.Errorf("[%d] status: got %s, want %s", c.idx, iface.Status, c.status)
		}
	}
}

func TestParseRoutingTable(t *testing.T) {
	input := "Route Flags: R - relay\n------------------------------------------------------------------------------\nRouting Tables: Public\n         Destinations : 3        Routes : 3\n\nDestination/Mask    Proto   Pre  Cost  Flags NextHop         Interface\n192.168.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1\n10.0.0.0/16       Static  60   0     RD    10.0.0.2        GE0/0/2\n0.0.0.0/0           Static  60   0     RD    10.0.0.1        GE0/0/1"

	result, err := ParseRoutingTable(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.RIBEntries) != 3 {
		t.Fatalf("expected 3, got %d", len(result.RIBEntries))
	}

	e := result.RIBEntries[0]
	if e.Prefix != "192.168.1.0" || e.MaskLen != 24 {
		t.Errorf("prefix: %s/%d", e.Prefix, e.MaskLen)
	}
	if e.Protocol != "ospf" {
		t.Errorf("proto: %s", e.Protocol)
	}
	if e.Preference != 10 {
		t.Errorf("pref: %d", e.Preference)
	}
	if e.VRF != "default" {
		t.Errorf("vrf: %s", e.VRF)
	}
}

func TestParseRoutingTableVPN(t *testing.T) {
	input := "Route Flags: R - relay\n------------------------------------------------------------------------------\nRouting Tables: VPN1\n         Destinations : 1        Routes : 1\n\nDestination/Mask    Proto   Pre  Cost  Flags NextHop         Interface\n172.16.0.0/24       OSPF    10   3           10.1.1.1      GE0/0/5"

	result, err := ParseRoutingTable(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.RIBEntries) != 1 {
		t.Fatalf("expected 1, got %d", len(result.RIBEntries))
	}
	if result.RIBEntries[0].VRF != "VPN1" {
		t.Errorf("expected VPN1, got %s", result.RIBEntries[0].VRF)
	}
}

func TestParseOspfPeer(t *testing.T) {
	input := "OSPF Process 1 with Router ID 1.1.1.1\n                 Peer Statistic Information\n ----------------------------------------------------------------------------\n Area Id          Interface                        Neighbor id      State\n 0.0.0.0          GE0/0/1                          2.2.2.2         Full\n 0.0.0.0          GE0/0/2                          3.3.3.3         Full"

	result, err := parseOspfPeer(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.Neighbors) != 2 {
		t.Fatalf("expected 2, got %d", len(result.Neighbors))
	}
	if result.Neighbors[0].Protocol != "ospf" {
		t.Error("expected ospf")
	}
	if result.Neighbors[0].RemoteID != "2.2.2.2" {
		t.Errorf("got %s", result.Neighbors[0].RemoteID)
	}
	if result.Neighbors[0].State != "full" {
		t.Errorf("got %s", result.Neighbors[0].State)
	}
	if result.Neighbors[0].AreaID != "0.0.0.0" {
		t.Errorf("got %s", result.Neighbors[0].AreaID)
	}
}

func TestParseLdpSession(t *testing.T) {
	input := " LDP Session(s) with peer: Total 2\n ---------------------------------------------------------------------------\n Peer LDP ID         State       GR  KA(Sent/Rcvd) Up Time\n 2.2.2.2:0          Operational N   30/30          5d:12h:30m\n 3.3.3.3:0          Operational N   30/30          3d:06h:15m"

	result, err := parseLdpSession(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.Neighbors) != 2 {
		t.Fatalf("expected 2, got %d", len(result.Neighbors))
	}
	if result.Neighbors[0].RemoteID != "2.2.2.2" {
		t.Errorf("got %s", result.Neighbors[0].RemoteID)
	}
	if result.Neighbors[0].State != "operational" {
		t.Errorf("got %s", result.Neighbors[0].State)
	}
}

func TestParseMplsLsp(t *testing.T) {
	input := " -------------------------------------------------------------------------------\n                 LSP Information: LDP LSP\n -------------------------------------------------------------------------------\n FEC                In/Out Label  In/Out IF                      Vrf Name\n 2.2.2.2/32        3/1024        -/GE0/0/1\n 3.3.3.3/32        1025/1026     GE0/0/2/GE0/0/1\n 4.4.4.4/32        1027/3        GE0/0/3/-                      "

	result, err := ParseMplsLsp(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.LFIBEntries) != 3 {
		t.Fatalf("expected 3, got %d", len(result.LFIBEntries))
	}

	e0 := result.LFIBEntries[0]
	if e0.FEC != "2.2.2.2/32" {
		t.Errorf("fec: %s", e0.FEC)
	}
	if e0.InLabel != 3 {
		t.Errorf("in_label: %d", e0.InLabel)
	}
	if e0.Protocol != "ldp" {
		t.Errorf("protocol: %s", e0.Protocol)
	}

	e1 := result.LFIBEntries[1]
	if e1.Action != "swap" {
		t.Errorf("action: %s", e1.Action)
	}
	if e1.InLabel != 1025 || e1.OutLabel != "1026" {
		t.Errorf("labels: %d→%s", e1.InLabel, e1.OutLabel)
	}

	e2 := result.LFIBEntries[2]
	if e2.Action != "pop" {
		t.Errorf("action: %s", e2.Action)
	}
}
