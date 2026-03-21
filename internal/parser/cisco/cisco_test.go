// internal/parser/cisco/cisco_test.go
package cisco

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestCiscoVendor(t *testing.T) {
	if New().Vendor() != "cisco" { t.Error("expected cisco") }
}

func TestCiscoDetectPrompt(t *testing.T) {
	p := New()
	tests := []struct{ line, wantHost string; wantOK bool }{
		{"Router-PE01#show version", "Router-PE01", true},
		{"Router-PE01(config)#interface GE0/0", "Router-PE01", true},
		{"<HUAWEI>display version", "", false},
	}
	for _, tt := range tests {
		host, ok := p.DetectPrompt(tt.line)
		if ok != tt.wantOK || host != tt.wantHost {
			t.Errorf("line %q: got (%q,%v), want (%q,%v)", tt.line, host, ok, tt.wantHost, tt.wantOK)
		}
	}
}

func TestParseShowIPInterfaceBrief(t *testing.T) {
	input := `Interface              IP-Address      OK? Method Status                Protocol
GigabitEthernet0/0     10.0.0.1    YES manual up                    up
GigabitEthernet0/1     unassigned      YES unset  administratively down down
Loopback0              1.1.1.1    YES manual up                    up
Port-channel1          10.0.0.2  YES manual up                    up`

	result, err := ParseShowIPInterfaceBrief(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Interfaces) != 4 { t.Fatalf("expected 4, got %d", len(result.Interfaces)) }

	if result.Interfaces[0].Name != "GigabitEthernet0/0" { t.Errorf("name: %s", result.Interfaces[0].Name) }
	if result.Interfaces[0].Status != "up" { t.Errorf("status: %s", result.Interfaces[0].Status) }
	if result.Interfaces[0].IPAddress != "10.0.0.1" { t.Errorf("ip: %s", result.Interfaces[0].IPAddress) }
	if result.Interfaces[0].Type != model.IfTypePhysical { t.Errorf("type: %s", result.Interfaces[0].Type) }

	if result.Interfaces[1].Status != "admin-down" { t.Errorf("status: %s", result.Interfaces[1].Status) }
	if result.Interfaces[2].Type != model.IfTypeLoopback { t.Errorf("type: %s", result.Interfaces[2].Type) }
	if result.Interfaces[3].Type != model.IfTypeEthTrunk { t.Errorf("type: %s", result.Interfaces[3].Type) }
}

func TestParseShowIPRoute(t *testing.T) {
	input := `Codes: L - local, C - connected, S - static, R - RIP, M - mobile, B - BGP
       D - EIGRP, EX - EIGRP external, O - OSPF, IA - OSPF inter area
Gateway of last resort is 10.0.0.1 to network 0.0.0.0

      10.0.0.0/8 is variably subnetted, 4 subnets, 2 masks
C        10.1.1.0/24 is directly connected, GigabitEthernet0/0
L        10.0.0.1/32 is directly connected, GigabitEthernet0/0
O        172.16.0.0/24 [110/2] via 10.0.0.2, 00:05:30, GigabitEthernet0/1
B        192.168.1.0/16 [20/0] via 10.0.0.3, 01:23:45
S*    0.0.0.0/0 [1/0] via 10.0.0.1`

	result, err := ParseShowIPRoute(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.RIBEntries) != 5 { t.Fatalf("expected 5, got %d", len(result.RIBEntries)) }

	// Check OSPF entry
	ospf := result.RIBEntries[2]
	if ospf.Prefix != "172.16.0.0" || ospf.MaskLen != 24 { t.Errorf("prefix: %s/%d", ospf.Prefix, ospf.MaskLen) }
	if ospf.Protocol != "ospf" { t.Errorf("proto: %s", ospf.Protocol) }
	if ospf.NextHop != "10.0.0.2" { t.Errorf("nexthop: %s", ospf.NextHop) }

	// Check default route
	def := result.RIBEntries[4]
	if def.Prefix != "0.0.0.0" || def.MaskLen != 0 { t.Errorf("default: %s/%d", def.Prefix, def.MaskLen) }
}

func TestParseShowOSPFNeighbor(t *testing.T) {
	input := `Neighbor ID     Pri   State           Dead Time   Address         Interface
10.0.0.2       1   FULL/DR         00:00:32    10.0.0.1    GigabitEthernet0/0
10.0.0.3       1   FULL/BDR        00:00:35    10.0.0.2  GigabitEthernet0/1`

	result, err := ParseShowOSPFNeighbor(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Neighbors) != 2 { t.Fatalf("expected 2, got %d", len(result.Neighbors)) }

	n := result.Neighbors[0]
	if n.RemoteID != "10.0.0.2" { t.Errorf("id: %s", n.RemoteID) }
	if n.State != "full/dr" { t.Errorf("state: %s", n.State) }
	if n.LocalInterface != "GigabitEthernet0/0" { t.Errorf("iface: %s", n.LocalInterface) }
	if n.Protocol != "ospf" { t.Errorf("proto: %s", n.Protocol) }
}

func TestParseShowMplsForwarding(t *testing.T) {
	input := `Local      Outgoing   Prefix           Bytes Label   Outgoing         Next Hop
Label      Label      or Tunnel Id     Switched        interface
16         Pop Label  10.1.1.0/24      0           Gi0/0            10.0.0.1
17         18         172.16.0.0/24    12345       Gi0/1            10.0.0.2
18         No Label   192.168.1.0/16       0           Gi0/2            10.0.0.3`

	result, err := ParseShowMplsForwarding(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.LFIBEntries) != 3 { t.Fatalf("expected 3, got %d", len(result.LFIBEntries)) }

	e0 := result.LFIBEntries[0]
	if e0.InLabel != 16 { t.Errorf("in: %d", e0.InLabel) }
	if e0.Action != "pop" { t.Errorf("action: %s", e0.Action) }

	e1 := result.LFIBEntries[1]
	if e1.InLabel != 17 || e1.OutLabel != "18" { t.Errorf("labels: %d→%s", e1.InLabel, e1.OutLabel) }
	if e1.Action != "swap" { t.Errorf("action: %s", e1.Action) }
}
