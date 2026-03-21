// internal/parser/juniper/juniper.go
package juniper

import (
	"regexp"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

var promptRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*@([A-Za-z][A-Za-z0-9._-]*)[>#]`)

type Parser struct{}
func New() *Parser { return &Parser{} }
func (p *Parser) Vendor() string { return "juniper" }

func (p *Parser) DetectPrompt(line string) (string, bool) {
	m := promptRe.FindStringSubmatch(strings.TrimRight(line, "\r \t"))
	if m == nil { return "", false }
	return m[1], true
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.HasPrefix(lower, "show route"): return model.CmdRIB
	case strings.HasPrefix(lower, "show interface"): return model.CmdInterface
	case strings.HasPrefix(lower, "show ospf neighbor"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show bgp summary"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show isis adjacency"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show ldp session"), strings.HasPrefix(lower, "show ldp neighbor"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "show rsvp session"): return model.CmdTunnel
	case strings.HasPrefix(lower, "show route table mpls"): return model.CmdLFIB
	case strings.HasPrefix(lower, "show configuration"): return model.CmdConfig
	default: return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface: return ParseShowInterfacesTerse(raw)
	case model.CmdRIB: return ParseShowRoute(raw)
	case model.CmdNeighbor: return ParseShowOSPFNeighbor(raw)
	case model.CmdLFIB: return ParseShowMplsRoute(raw)
	default: return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "lo"): return model.IfTypeLoopback
	case strings.HasPrefix(lower, "ae"): return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "irb"), strings.HasPrefix(lower, "vlan"): return model.IfTypeVlanif
	case strings.HasPrefix(lower, "gr-"), strings.HasPrefix(lower, "ip-"): return model.IfTypeTunnelGRE
	case strings.Contains(lower, "."): return model.IfTypeSubInterface
	default: return model.IfTypePhysical
	}
}
