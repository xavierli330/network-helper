// internal/parser/h3c/h3c.go
package h3c

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

type Parser struct{}
func New() *Parser { return &Parser{} }
func (p *Parser) Vendor() string { return "h3c" }

func (p *Parser) DetectPrompt(line string) (string, bool) {
	trimmed := strings.TrimRight(line, "\r \t")
	if len(trimmed) > 2 && trimmed[0] == '<' {
		end := strings.Index(trimmed, ">")
		if end > 1 {
			hostname := trimmed[1:end]
			if strings.ContainsAny(hostname, "@$~/ ") { return "", false }
			if strings.ContainsAny(hostname, "[]") { return "", false }
			if len(hostname) < 3 { return "", false }
			return hostname, true
		}
	}
	if len(trimmed) > 2 && trimmed[0] == '[' {
		end := strings.Index(trimmed, "]")
		if end > 1 {
			hostname := trimmed[1:end]
			if strings.ContainsAny(hostname, "@$~ ") { return "", false }
			return hostname, true
		}
	}
	return "", false
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	// Expand the common H3C abbreviation "dis" → "display".
	if strings.HasPrefix(lower, "dis ") && !strings.HasPrefix(lower, "display ") {
		lower = "display " + lower[4:]
	}
	switch {
	case strings.HasPrefix(lower, "display ip routing-table"): return model.CmdRIB
	case strings.HasPrefix(lower, "display fib"): return model.CmdFIB
	case strings.HasPrefix(lower, "display mpls lsp"), strings.HasPrefix(lower, "display mpls forwarding"): return model.CmdLFIB
	case strings.HasPrefix(lower, "display interface"),
		strings.HasPrefix(lower, "display int"):
		return model.CmdInterface
	case strings.HasPrefix(lower, "display ospf peer"), strings.HasPrefix(lower, "display bgp peer"),
		strings.HasPrefix(lower, "display isis peer"), strings.HasPrefix(lower, "display mpls ldp session"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "display current-configuration"),
		strings.HasPrefix(lower, "display cur"):
		return model.CmdConfig
	default: return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface: return ParseInterfaceBrief(raw)
	case model.CmdRIB: return ParseRoutingTable(raw)
	case model.CmdNeighbor: return ParseOspfPeer(raw)
	case model.CmdLFIB: return ParseMplsLsp(raw)
	case model.CmdConfig: return model.ParseResult{Type: model.CmdConfig, ConfigText: raw, RawText: raw}, nil
	default: return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "loop"): return model.IfTypeLoopback
	case strings.HasPrefix(lower, "vlan"): return model.IfTypeVlanif
	case strings.HasPrefix(lower, "bridge-aggregation"), strings.HasPrefix(lower, "bagg"): return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "tunnel"): return model.IfTypeTunnelGRE
	case strings.HasPrefix(lower, "null"): return model.IfTypeNull
	case strings.Contains(lower, "."): return model.IfTypeSubInterface
	default: return model.IfTypePhysical
	}
}
