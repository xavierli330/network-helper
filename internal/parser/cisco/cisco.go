// internal/parser/cisco/cisco.go
package cisco

import (
	"regexp"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

var promptRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9._-]*)(?:\([^)]*\))?#`)

type Parser struct{}
func New() *Parser { return &Parser{} }
func (p *Parser) Vendor() string { return "cisco" }

func (p *Parser) DetectPrompt(line string) (string, bool) {
	m := promptRe.FindStringSubmatch(strings.TrimRight(line, "\r \t"))
	if m == nil { return "", false }
	return m[1], true
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.HasPrefix(lower, "show ip route"), strings.HasPrefix(lower, "show route"):
		return model.CmdRIB
	case strings.HasPrefix(lower, "show ip cef"):
		return model.CmdFIB
	case strings.HasPrefix(lower, "show mpls forwarding"):
		return model.CmdLFIB
	case strings.HasPrefix(lower, "show interface"), strings.HasPrefix(lower, "show ip interface"):
		return model.CmdInterface
	case strings.HasPrefix(lower, "show ip ospf neighbor"),
		strings.HasPrefix(lower, "show ip bgp summary"),
		strings.HasPrefix(lower, "show bgp summary"),
		strings.HasPrefix(lower, "show isis neighbor"),
		strings.HasPrefix(lower, "show mpls ldp neighbor"),
		strings.HasPrefix(lower, "show lldp neighbor"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "show mpls traffic-eng tunnel"):
		return model.CmdTunnel
	case strings.HasPrefix(lower, "show running-config"), strings.HasPrefix(lower, "show startup-config"):
		return model.CmdConfig
	default:
		return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface: return ParseShowIPInterfaceBrief(raw)
	case model.CmdRIB: return ParseShowIPRoute(raw)
	case model.CmdNeighbor: return ParseShowOSPFNeighbor(raw)
	case model.CmdLFIB: return ParseShowMplsForwarding(raw)
	default: return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "loopback"): return model.IfTypeLoopback
	case strings.HasPrefix(lower, "vlan"): return model.IfTypeVlanif
	case strings.HasPrefix(lower, "port-channel"): return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "tunnel"): return model.IfTypeTunnelGRE
	case strings.HasPrefix(lower, "null"): return model.IfTypeNull
	case strings.Contains(lower, "."): return model.IfTypeSubInterface
	default: return model.IfTypePhysical
	}
}
