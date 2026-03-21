package parser

import (
	"regexp"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

var (
	huaweiAnglePrompt   = regexp.MustCompile(`^<([^>]+)>(.*)`)
	huaweiBracketPrompt = regexp.MustCompile(`^\[([^\]]+)\](.*)`)
	ciscoPrompt         = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9._-]*)(?:\([^)]*\))?#(.*)`)
	juniperPrompt       = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*@([A-Za-z][A-Za-z0-9._-]*)[>#]\s*(.*)`)
)

func DetectVendor(line string) (vendor, hostname string) {
	trimmed := strings.TrimRight(line, "\r \t")
	if m := huaweiAnglePrompt.FindStringSubmatch(trimmed); m != nil { return "huawei", m[1] }
	if m := huaweiBracketPrompt.FindStringSubmatch(trimmed); m != nil { return "huawei", m[1] }
	if m := juniperPrompt.FindStringSubmatch(trimmed); m != nil { return "juniper", m[1] }
	if m := ciscoPrompt.FindStringSubmatch(trimmed); m != nil { return "cisco", m[1] }
	return "", ""
}

func ClassifyHuaweiCommand(cmd string) model.CommandType {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.HasPrefix(lower, "display ip routing-table"): return model.CmdRIB
	case strings.HasPrefix(lower, "display fib"): return model.CmdFIB
	case strings.HasPrefix(lower, "display mpls lsp"), strings.HasPrefix(lower, "display mpls forwarding"): return model.CmdLFIB
	case strings.HasPrefix(lower, "display interface"): return model.CmdInterface
	case strings.HasPrefix(lower, "display ospf peer"), strings.HasPrefix(lower, "display bgp peer"),
		strings.HasPrefix(lower, "display isis peer"), strings.HasPrefix(lower, "display mpls ldp session"),
		strings.HasPrefix(lower, "display mpls ldp peer"), strings.HasPrefix(lower, "display rsvp session"),
		strings.HasPrefix(lower, "display lldp neighbor"): return model.CmdNeighbor
	case strings.HasPrefix(lower, "display mpls te tunnel"): return model.CmdTunnel
	case strings.HasPrefix(lower, "display segment-routing"), strings.HasPrefix(lower, "display isis segment-routing"): return model.CmdSRMapping
	case strings.HasPrefix(lower, "display current-configuration"), strings.HasPrefix(lower, "display saved-configuration"): return model.CmdConfig
	default: return model.CmdUnknown
	}
}
