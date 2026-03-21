package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

var _ interface{ Vendor() string } = (*Parser)(nil) // compile-time check

type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Vendor() string { return "huawei" }

func (p *Parser) DetectPrompt(line string) (string, bool) {
	trimmed := strings.TrimRight(line, "\r \t")
	// <hostname> style
	if len(trimmed) > 2 && trimmed[0] == '<' {
		end := strings.Index(trimmed, ">")
		if end > 1 {
			return trimmed[1:end], true
		}
	}
	// [hostname] style
	if len(trimmed) > 2 && trimmed[0] == '[' {
		end := strings.Index(trimmed, "]")
		if end > 1 {
			return trimmed[1:end], true
		}
	}
	return "", false
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.HasPrefix(lower, "display ip routing-table"):
		return model.CmdRIB
	case strings.HasPrefix(lower, "display fib"):
		return model.CmdFIB
	case strings.HasPrefix(lower, "display mpls lsp"), strings.HasPrefix(lower, "display mpls forwarding"):
		return model.CmdLFIB
	case strings.HasPrefix(lower, "display interface"):
		return model.CmdInterface
	case strings.HasPrefix(lower, "display ospf peer"), strings.HasPrefix(lower, "display bgp peer"),
		strings.HasPrefix(lower, "display isis peer"), strings.HasPrefix(lower, "display mpls ldp session"),
		strings.HasPrefix(lower, "display mpls ldp peer"), strings.HasPrefix(lower, "display rsvp session"),
		strings.HasPrefix(lower, "display lldp neighbor"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "display mpls te tunnel"):
		return model.CmdTunnel
	case strings.HasPrefix(lower, "display segment-routing"), strings.HasPrefix(lower, "display isis segment-routing"):
		return model.CmdSRMapping
	case strings.HasPrefix(lower, "display current-configuration"), strings.HasPrefix(lower, "display saved-configuration"):
		return model.CmdConfig
	default:
		return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface:
		return ParseInterfaceBrief(raw)
	case model.CmdRIB:
		return ParseRoutingTable(raw)
	case model.CmdNeighbor:
		return ParseNeighbor(raw)
	case model.CmdLFIB:
		return ParseMplsLsp(raw)
	default:
		return model.ParseResult{Type: cmdType, RawText: raw}, nil
	}
}

func inferInterfaceType(name string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "loopback"), strings.HasPrefix(lower, "lo"):
		return model.IfTypeLoopback
	case strings.HasPrefix(lower, "vlanif"):
		return model.IfTypeVlanif
	case strings.HasPrefix(lower, "eth-trunk"):
		return model.IfTypeEthTrunk
	case strings.HasPrefix(lower, "tunnel") && strings.Contains(lower, "te"):
		return model.IfTypeTunnelTE
	case strings.HasPrefix(lower, "tunnel"):
		return model.IfTypeTunnelGRE
	case strings.HasPrefix(lower, "nve"):
		return model.IfTypeNVE
	case strings.HasPrefix(lower, "null"):
		return model.IfTypeNull
	case strings.Contains(lower, "."):
		return model.IfTypeSubInterface
	default:
		return model.IfTypePhysical
	}
}
