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
			hostname := trimmed[1:end]
			// Reject if it looks like a shell prompt or contains invalid chars
			if strings.ContainsAny(hostname, "@$~/ ") { return "", false }
			// Reject hostnames that contain brackets (e.g. "<[Enter]>")
			if strings.ContainsAny(hostname, "[]") { return "", false }
			// Reject very short hostnames (e.g. "<cr>" from Cisco help output)
			if len(hostname) < 3 { return "", false }
			return hostname, true
		}
	}
	// [hostname] style — Huawei config mode
	if len(trimmed) > 2 && trimmed[0] == '[' {
		end := strings.Index(trimmed, "]")
		if end > 1 {
			hostname := trimmed[1:end]
			// Reject shell prompts like [user@host ~]$
			if strings.ContainsAny(hostname, "@$~ ") { return "", false }
			return hostname, true
		}
	}
	return "", false
}

func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))

	// Normalize common abbreviations: "dis" → "display"
	if strings.HasPrefix(lower, "dis ") && !strings.HasPrefix(lower, "display ") {
		lower = "display " + lower[4:]
	}

	switch {
	case strings.HasPrefix(lower, "display ip routing-table"),
		strings.HasPrefix(lower, "display ip rout"):
		return model.CmdRIB
	case strings.HasPrefix(lower, "display fib"):
		return model.CmdFIB
	case strings.HasPrefix(lower, "display mpls lsp"),
		strings.HasPrefix(lower, "display mpls forwarding"):
		return model.CmdLFIB
	case strings.HasPrefix(lower, "display interface"),
		strings.HasPrefix(lower, "display int"):
		return model.CmdInterface
	case strings.HasPrefix(lower, "display ospf peer"),
		strings.HasPrefix(lower, "display bgp peer"),
		strings.HasPrefix(lower, "display isis peer"),
		strings.HasPrefix(lower, "display mpls ldp session"),
		strings.HasPrefix(lower, "display mpls ldp peer"),
		strings.HasPrefix(lower, "display rsvp session"),
		strings.HasPrefix(lower, "display lldp neighbor"):
		return model.CmdNeighbor
	case strings.HasPrefix(lower, "display mpls te tunnel"):
		return model.CmdTunnel
	case strings.HasPrefix(lower, "display segment-routing"),
		strings.HasPrefix(lower, "display isis segment-routing"):
		return model.CmdSRMapping
	case strings.HasPrefix(lower, "display current-configuration"),
		strings.HasPrefix(lower, "display saved-configuration"),
		strings.HasPrefix(lower, "display cur"),
		strings.HasPrefix(lower, "display sa"):
		return model.CmdConfig
	case strings.HasPrefix(lower, "display route-policy"),
		strings.HasPrefix(lower, "display route-p"):
		return model.CmdUnknown // route-policy output is not running-config
	default:
		if ct := classifyGenerated(lower); ct != model.CmdUnknown {
			return ct
		}
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
	case model.CmdConfig:
		return model.ParseResult{Type: model.CmdConfig, ConfigText: raw, RawText: raw}, nil
	default:
		return parseGenerated(cmdType, raw)
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

// SupportedCmdTypes returns all CommandType values the Huawei parser handles.
func (p *Parser) SupportedCmdTypes() []model.CommandType {
	base := []model.CommandType{
		model.CmdInterface,
		model.CmdNeighbor,
		model.CmdRIB,
		model.CmdFIB,
		model.CmdLFIB,
		model.CmdTunnel,
		model.CmdSRMapping,
		model.CmdConfig,
		// CmdConfigSet removed — not classified by Huawei parser
	}
	return append(base, generatedCmdTypes()...)
}

// FieldSchema returns field definitions for the given CommandType.
func (p *Parser) FieldSchema(cmdType model.CommandType) []model.FieldDef {
	switch cmdType {
	case model.CmdInterface:
		return []model.FieldDef{
			{Name: "name",        Type: model.FieldTypeString, Description: "接口名称", Example: "GigabitEthernet0/0/0"},
			{Name: "status",      Type: model.FieldTypeString, Description: "接口状态", Example: "up"},
			{Name: "ip_address",  Type: model.FieldTypeString, Description: "IP 地址", Example: "10.0.0.1"},
			{Name: "mask",        Type: model.FieldTypeString, Description: "子网掩码", Example: "255.255.255.0"},
			{Name: "bandwidth",   Type: model.FieldTypeString, Description: "带宽配置", Example: "1000M"},
			{Name: "description", Type: model.FieldTypeString, Description: "接口描述", Example: "to-PE1"},
		}
	case model.CmdNeighbor:
		return []model.FieldDef{
			{Name: "protocol",        Type: model.FieldTypeString, Description: "邻居协议", Example: "ospf"},
			{Name: "remote_id",       Type: model.FieldTypeString, Description: "对端 ID", Example: "10.0.0.2"},
			{Name: "remote_address",  Type: model.FieldTypeString, Description: "对端地址", Example: "10.0.0.2"},
			{Name: "local_interface", Type: model.FieldTypeString, Description: "本端接口", Example: "GE0/0/1"},
			{Name: "area_id",         Type: model.FieldTypeString, Description: "OSPF 区域", Example: "0.0.0.0"},
			{Name: "state",           Type: model.FieldTypeString, Description: "邻居状态", Example: "Full"},
			{Name: "uptime",          Type: model.FieldTypeString, Description: "建立时长", Example: "2d3h"},
		}
	case model.CmdRIB:
		return []model.FieldDef{
			{Name: "prefix",             Type: model.FieldTypeString, Description: "目的前缀", Example: "10.0.0.0"},
			{Name: "mask_len",           Type: model.FieldTypeInt,    Description: "前缀长度", Example: "24"},
			{Name: "protocol",           Type: model.FieldTypeString, Description: "路由协议", Example: "ospf"},
			{Name: "next_hop",           Type: model.FieldTypeString, Description: "下一跳", Example: "10.1.0.1"},
			{Name: "outgoing_interface", Type: model.FieldTypeString, Description: "出接口", Example: "GE0/0/1"},
			{Name: "preference",         Type: model.FieldTypeInt,    Description: "路由优先级", Example: "10"},
			{Name: "metric",             Type: model.FieldTypeInt,    Description: "路由开销", Example: "1"},
		}
	case model.CmdConfig, model.CmdConfigSet:
		return []model.FieldDef{
			{Name: "config_text", Type: model.FieldTypeString, Description: "设备配置文本", Example: "sysname Router1"},
		}
	default:
		return generatedFieldSchema(cmdType)
	}
}
