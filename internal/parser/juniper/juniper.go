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
	case strings.Contains(lower, "| display set"): return model.CmdConfigSet
	case strings.HasPrefix(lower, "show configuration"): return model.CmdConfig
	default:
		if ct := classifyGenerated(lower); ct != model.CmdUnknown {
			return ct
		}
		return model.CmdUnknown
	}
}

func (p *Parser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	case model.CmdInterface: return ParseShowInterfacesTerse(raw)
	case model.CmdRIB: return ParseShowRoute(raw)
	case model.CmdNeighbor: return ParseShowOSPFNeighbor(raw)
	case model.CmdLFIB: return ParseShowMplsRoute(raw)
	case model.CmdConfig: return model.ParseResult{Type: model.CmdConfig, ConfigText: raw, RawText: raw}, nil
	case model.CmdConfigSet: return model.ParseResult{Type: model.CmdConfigSet, ConfigText: raw, RawText: raw}, nil
	default: return parseGenerated(cmdType, raw)
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

// SupportedCmdTypes returns all CommandType values handled by the Juniper parser.
func (p *Parser) SupportedCmdTypes() []model.CommandType {
	base := []model.CommandType{
		model.CmdInterface,
		model.CmdNeighbor,
		model.CmdRIB,
		model.CmdLFIB,
		model.CmdTunnel,
		model.CmdConfig,
		model.CmdConfigSet,
		// CmdFIB: not classified by Juniper parser
		// CmdSRMapping: not classified by Juniper parser
	}
	return append(base, generatedCmdTypes()...)
}

// FieldSchema returns field definitions for the given CommandType.
func (p *Parser) FieldSchema(cmdType model.CommandType) []model.FieldDef {
	switch cmdType {
	case model.CmdInterface:
		return []model.FieldDef{
			{Name: "name",       Type: model.FieldTypeString, Description: "Interface name", Example: "ge-0/0/0.0"},
			{Name: "status",     Type: model.FieldTypeString, Description: "Link status (up/down/admin-down)", Example: "up"},
			{Name: "ip_address", Type: model.FieldTypeString, Description: "IPv4 address (inet)", Example: "10.0.0.1"},
		}
	case model.CmdNeighbor:
		return []model.FieldDef{
			{Name: "protocol",        Type: model.FieldTypeString, Description: "Routing protocol", Example: "ospf"},
			{Name: "remote_address",  Type: model.FieldTypeString, Description: "Neighbor IP address", Example: "10.0.0.2"},
			{Name: "local_interface", Type: model.FieldTypeString, Description: "Local interface toward neighbor", Example: "ge-0/0/0.0"},
			{Name: "state",           Type: model.FieldTypeString, Description: "Neighbor state", Example: "full"},
			{Name: "remote_id",       Type: model.FieldTypeString, Description: "Neighbor router-ID", Example: "10.0.0.2"},
		}
	case model.CmdRIB:
		return []model.FieldDef{
			{Name: "prefix",             Type: model.FieldTypeString, Description: "Route prefix", Example: "10.0.0.0"},
			{Name: "mask_len",           Type: model.FieldTypeInt,    Description: "Prefix length", Example: "24"},
			{Name: "protocol",           Type: model.FieldTypeString, Description: "Routing protocol", Example: "ospf"},
			{Name: "next_hop",           Type: model.FieldTypeString, Description: "Next-hop address", Example: "10.1.0.1"},
			{Name: "outgoing_interface", Type: model.FieldTypeString, Description: "Outgoing interface", Example: "ge-0/0/0.0"},
			{Name: "preference",         Type: model.FieldTypeInt,    Description: "Administrative distance", Example: "10"},
			{Name: "metric",             Type: model.FieldTypeInt,    Description: "Route metric", Example: "2"},
		}
	case model.CmdConfig, model.CmdConfigSet:
		return []model.FieldDef{
			{Name: "config_text", Type: model.FieldTypeString, Description: "Device configuration text", Example: "set interfaces ge-0/0/0 unit 0"},
		}
	default:
		return generatedFieldSchema(cmdType)
	}
}
