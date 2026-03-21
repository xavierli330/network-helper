package parser

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestDetectVendor(t *testing.T) {
	tests := []struct{ line, wantVendor, wantHost string }{
		{"<HUAWEI-Core-01>display version", "huawei", "HUAWEI-Core-01"},
		{"[HUAWEI-Core-01]display version", "huawei", "HUAWEI-Core-01"},
		{"Router-PE01#show version", "cisco", "Router-PE01"},
		{"Router-PE01(config)#interface GE0/0", "cisco", "Router-PE01"},
		{"admin@MX204-01> show version", "juniper", "MX204-01"},
		{"admin@MX204-01# set interfaces", "juniper", "MX204-01"},
		{"just some random text", "", ""},
	}
	for _, tt := range tests {
		vendor, hostname := DetectVendor(tt.line)
		if vendor != tt.wantVendor { t.Errorf("line %q: vendor=%q, want %q", tt.line, vendor, tt.wantVendor) }
		if hostname != tt.wantHost { t.Errorf("line %q: host=%q, want %q", tt.line, hostname, tt.wantHost) }
	}
}

func TestClassifyHuaweiCommand(t *testing.T) {
	tests := []struct{ cmd string; want model.CommandType }{
		{"display ip routing-table", model.CmdRIB},
		{"display ip routing-table vpn-instance VPN1", model.CmdRIB},
		{"display interface brief", model.CmdInterface},
		{"display ospf peer", model.CmdNeighbor},
		{"display bgp peer", model.CmdNeighbor},
		{"display mpls ldp session", model.CmdNeighbor},
		{"display mpls lsp", model.CmdLFIB},
		{"display current-configuration", model.CmdConfig},
		{"display version", model.CmdUnknown},
	}
	for _, tt := range tests {
		got := ClassifyHuaweiCommand(tt.cmd)
		if got != tt.want { t.Errorf("cmd %q: got %q, want %q", tt.cmd, got, tt.want) }
	}
}
