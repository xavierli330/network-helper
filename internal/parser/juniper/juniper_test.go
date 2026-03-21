// internal/parser/juniper/juniper_test.go
package juniper

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestJuniperVendor(t *testing.T) {
	if New().Vendor() != "juniper" { t.Error("expected juniper") }
}

func TestJuniperDetectPrompt(t *testing.T) {
	p := New()
	tests := []struct{ line, wantHost string; wantOK bool }{
		{"admin@MX204-01> show version", "MX204-01", true},
		{"admin@MX204-01# set interfaces", "MX204-01", true},
		{"<HUAWEI>display version", "", false},
	}
	for _, tt := range tests {
		host, ok := p.DetectPrompt(tt.line)
		if ok != tt.wantOK || host != tt.wantHost {
			t.Errorf("line %q: got (%q,%v), want (%q,%v)", tt.line, host, ok, tt.wantHost, tt.wantOK)
		}
	}
}

func TestParseShowInterfacesTerse(t *testing.T) {
	input := `Interface               Admin Link Proto    Local                 Remote
ge-0/0/0                up    up
ge-0/0/0.0              up    up   inet     10.0.0.1/24
ge-0/0/1                up    down
lo0.0                   up    up   inet     1.1.1.1/32
ae0                     up    up`

	result, err := ParseShowInterfacesTerse(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.Interfaces) != 5 { t.Fatalf("expected 5, got %d", len(result.Interfaces)) }

	if result.Interfaces[0].Name != "ge-0/0/0" { t.Errorf("name: %s", result.Interfaces[0].Name) }
	if result.Interfaces[0].Status != "up" { t.Errorf("status: %s", result.Interfaces[0].Status) }
	if result.Interfaces[0].Type != model.IfTypePhysical { t.Errorf("type: %s", result.Interfaces[0].Type) }
	if result.Interfaces[1].Type != model.IfTypeSubInterface { t.Errorf("type: %s", result.Interfaces[1].Type) }
	if result.Interfaces[1].IPAddress != "10.0.0.1" { t.Errorf("ip: %s", result.Interfaces[1].IPAddress) }
	if result.Interfaces[3].Type != model.IfTypeLoopback { t.Errorf("type: %s", result.Interfaces[3].Type) }
	if result.Interfaces[4].Type != model.IfTypeEthTrunk { t.Errorf("type: %s", result.Interfaces[4].Type) }
}

func TestParseShowRoute(t *testing.T) {
	input := `inet.0: 5 destinations, 5 routes (5 active, 0 holddown, 0 hidden)
+ = Active Route, - = Last Active, * = Both

10.1.1.0/24      *[OSPF/10] 00:05:12, metric 2
                    >  to 10.0.0.1 via ge-0/0/0.0
172.16.0.0/16       *[Static/5] 00:10:00
                    >  to 10.0.0.2 via ge-0/0/1.0
0.0.0.0/0          *[Static/5] 01:00:00
                    >  to 10.0.0.1 via ge-0/0/0.0`

	result, err := ParseShowRoute(input)
	if err != nil { t.Fatalf("error: %v", err) }
	if len(result.RIBEntries) != 3 { t.Fatalf("expected 3, got %d", len(result.RIBEntries)) }

	e := result.RIBEntries[0]
	if e.Prefix != "10.1.1.0" || e.MaskLen != 24 { t.Errorf("prefix: %s/%d", e.Prefix, e.MaskLen) }
	if e.Protocol != "ospf" { t.Errorf("proto: %s", e.Protocol) }
	if e.Preference != 10 { t.Errorf("pref: %d", e.Preference) }
	if e.NextHop != "10.0.0.1" { t.Errorf("nexthop: %s", e.NextHop) }
	if e.OutgoingInterface != "ge-0/0/0.0" { t.Errorf("iface: %s", e.OutgoingInterface) }
}
