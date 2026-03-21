// internal/parser/h3c/h3c_test.go
package h3c

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestH3CVendor(t *testing.T) {
	if New().Vendor() != "h3c" { t.Error("expected h3c") }
}

func TestH3CDetectPrompt(t *testing.T) {
	p := New()
	// H3C uses same <hostname> as Huawei
	host, ok := p.DetectPrompt("<H3C-SW01>display version")
	if !ok || host != "H3C-SW01" { t.Errorf("got (%q, %v)", host, ok) }

	host, ok = p.DetectPrompt("[H3C-SW01]interface GE1/0/1")
	if !ok || host != "H3C-SW01" { t.Errorf("got (%q, %v)", host, ok) }

	_, ok = p.DetectPrompt("Router#show version")
	if ok { t.Error("should not match Cisco") }
}

func TestH3CParseInterfaceBrief(t *testing.T) {
	input := `Brief information on interfaces in route mode:
Link: ADM - administratively down; Stby - standby
Protocol: (s) - spoofing
Interface            Link Protocol Primary IP      Description
GE1/0/1              UP   UP       10.0.0.1
GE1/0/2              DOWN DOWN     --
Loop0                UP   UP(s)    1.1.1.1`

	result, err := ParseInterfaceBrief(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Interfaces) != 3 { t.Fatalf("expected 3, got %d", len(result.Interfaces)) }
	if result.Interfaces[0].Name != "GE1/0/1" { t.Errorf("name: %s", result.Interfaces[0].Name) }
	if result.Interfaces[0].Status != "up" { t.Errorf("status: %s", result.Interfaces[0].Status) }
	if result.Interfaces[2].Type != model.IfTypeLoopback { t.Errorf("type: %s", result.Interfaces[2].Type) }
}

func TestH3CParseRoutingTable(t *testing.T) {
	input := `Routing Tables: Public
	Destinations : 3        Routes : 3

Destination/Mask    Proto  Pre  Cost         NextHop         Interface
10.1.1.0/24        O_INTRA 10   2            10.0.0.1        GE1/0/1
172.16.0.0/16      S      60   0            10.0.0.2        GE1/0/2
0.0.0.0/0           S      60   0            10.0.0.1        GE1/0/1`

	result, err := ParseRoutingTable(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.RIBEntries) != 3 { t.Fatalf("expected 3, got %d", len(result.RIBEntries)) }
	if result.RIBEntries[0].Protocol != "ospf" { t.Errorf("proto: %s", result.RIBEntries[0].Protocol) }
}
