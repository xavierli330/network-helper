package parser

import (
	"regexp"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

var (
	huaweiAnglePrompt   = regexp.MustCompile(`^<([^>]+)>(.*)`)
	huaweiBracketPrompt = regexp.MustCompile(`^\[([^\]]+)\](.*)`)
	ciscoPrompt         = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9._-]*)(?:\([^)]*\))?#(.*)`)
	juniperPrompt       = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]*@([A-Za-z][A-Za-z0-9._-]*)[>#]\s*(.*)`)
)

func DetectVendor(line string) (vendor, hostname string) {
	trimmed := strings.TrimRight(line, "\r \t")
	// NOTE: H3C uses the same <hostname> prompt as Huawei.
	// DetectVendor returns "huawei" for both. In the pipeline, when the
	// full VendorParser registry is available, H3C detection happens via
	// the H3C parser's DetectPrompt — but since both match <hostname>,
	// the registry iteration order determines which parser claims the block.
	// For explicit H3C support, users should configure device-vendor mappings
	// in config.yaml (future feature). For now, Huawei parser handles both
	// since the output formats are very similar.
	if m := huaweiAnglePrompt.FindStringSubmatch(trimmed); m != nil { return "huawei", m[1] }
	if m := huaweiBracketPrompt.FindStringSubmatch(trimmed); m != nil { return "huawei", m[1] }
	if m := juniperPrompt.FindStringSubmatch(trimmed); m != nil { return "juniper", m[1] }
	if m := ciscoPrompt.FindStringSubmatch(trimmed); m != nil { return "cisco", m[1] }
	return "", ""
}

// DetectVendorWithHints implements three-layer vendor detection:
//   1. Hostname keyword match from VendorHintCache (highest priority)
//   2. Prompt regex match via DetectVendor (fallback)
//   3. Returns empty if neither matches
//
// This allows operators to configure hostname keywords (e.g. "H9850" → h3c,
// "NE40" → huawei) so that devices with ambiguous prompts (both Huawei and
// H3C use <hostname>) are correctly identified.
func DetectVendorWithHints(line string, hints *store.VendorHintCache) (vendor, hostname string) {
	// First, detect hostname via regex (always needed for hostname extraction)
	vendor, hostname = DetectVendor(line)
	if hostname == "" {
		return "", ""
	}

	// If we have a hint cache, check hostname keywords for vendor override
	if hints != nil {
		if hintVendor, ok := hints.Lookup(hostname); ok {
			return hintVendor, hostname
		}
	}

	return vendor, hostname
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
