package parser

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/xavierli/nethelper/internal/store"
)

// Collector captures CommandBlocks with CmdUnknown type into unknown_outputs.
// All errors are logged and swallowed — never fails the pipeline.
type Collector struct {
	db *store.DB
}

// NewCollector creates a Collector. If db is nil, Collect is a no-op.
func NewCollector(db *store.DB) *Collector {
	return &Collector{db: db}
}

// Collect records an unknown block. Safe to call from the pipeline.
// Filters out: empty output, control commands, and incomplete (abbreviated) commands.
func (c *Collector) Collect(block CommandBlock) error {
	if c.db == nil {
		return nil
	}

	// ── Filter 1: Empty / whitespace-only output ──────────────────────
	// Commands with no output are typically Tab-completion intermediates
	// (e.g. "dis link-agg<Tab>" echoes the expanded form but produces no data).
	trimmedOutput := strings.TrimSpace(block.Output)
	if trimmedOutput == "" {
		slog.Debug("collector: skipping empty output", "cmd", block.Command)
		return nil
	}

	// ── Filter 2: Control / navigation commands ──────────────────────
	// These are device shell navigation commands that never produce
	// parseable structured data.
	if IsControlCommand(block.Vendor, block.Command) {
		slog.Debug("collector: skipping control command", "cmd", block.Command)
		return nil
	}

	// ── Filter 3: Help echo / completion output ──────────────────────
	// When a user presses '?' or Tab in the CLI, the device echoes a
	// help menu listing available keywords / parameters.  These are
	// useless for rule generation.  Detect by content rather than
	// command suffix, because the command itself may not end with '?'.
	if IsHelpEcho(trimmedOutput) {
		slog.Debug("collector: skipping help echo output", "cmd", block.Command)
		return nil
	}

	// ── Filter 5: Error output ──────────────────────────────────
	// CLI error messages (% Invalid input, Error: ..., ^ pointer)
	// are not useful for rule generation.
	if IsErrorOutput(trimmedOutput) {
		slog.Debug("collector: skipping error output", "cmd", block.Command)
		return nil
	}

	norm := NormaliseCommand(block.Vendor, block.Command)

	// ── Filter 4: Separate args from pattern ─────────────────────────
	// Strip trailing instance arguments (interface names, IDs, IPs) so
	// "display link-aggregation verbose bridge-aggregation 1" and
	// "display link-aggregation verbose bridge-aggregation 2" share the
	// same command_norm = "display link-aggregation verbose bridge-aggregation {id}".
	pattern := StripArgs(norm)

	hash := hashContent(block.Output)
	entry := store.UnknownOutput{
		DeviceID:    block.Hostname,
		Vendor:      block.Vendor,
		CommandRaw:  block.Command,
		CommandNorm: pattern,
		RawOutput:   block.Output,
		ContentHash: hash,
	}
	if err := c.db.UpsertUnknownOutput(entry); err != nil {
		slog.Warn("collector: failed to upsert unknown output", "cmd", block.Command, "error", err)
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// Help-echo detection (Filter 3)
// ═══════════════════════════════════════════════════════════════════════════

// helpEchoSignatures are exact substrings that appear in CLI help/completion
// output on virtually every vendor.  Any single match → help echo.
var helpEchoSignatures = []string{
	"<cr>",                  // "press Enter" marker in all vendors
	">  Redirect it to a file",
	"|  Matching output",
	"|  Begin with",
	"|  Exclude",
	"|  Include",
	"|  Redirect to", // Juniper pipe modifiers
	"Incomplete command",    // error hint on some platforms
	// ── 新增签名 ──
	"Possible completions:",        // Juniper帮助标志行
	"|  Output modifiers",          // Cisco管道修饰符
	"% Ambiguous command",          // Cisco歧义错误
	"% Invalid input detected",     // Cisco输入错误
	"% Incomplete command",         // Cisco不完整命令
	"Error: Unrecognized command",  // 华为/H3C错误
	"Error: Wrong parameter found", // 华为参数错误
	"Unknown command",              // 通用错误
}

// helpEchoLineRe matches a typical help menu line: a keyword followed by
// two-or-more spaces and then a description.  Examples:
//
//	"  interface       Select an interface to configure"
//	"  <cr>"
//
// We require at least 3 such lines to avoid false positives on short
// tabular output that happens to have wide column gaps.
var helpEchoLineRe = regexp.MustCompile(`^\s*\S+\s{2,}\S`)

// dataIndicatorRe matches lines that contain structured network data rather
// than CLI help descriptions. These indicators disambiguate tabular command
// output (which happens to have wide column gaps) from help-echo text.
//
// Matches: IP addresses, community values (1:1), CIDR prefixes (/24),
// MAC addresses, interface identifiers with numbers, etc.
var dataIndicatorRe = regexp.MustCompile(
	`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}` + // IPv4 address
		`|` + `\d+:\d+` + // community value or time format (e.g. 1:1, 1:65100)
		`|` + `/\d{1,3}\b` + // CIDR mask (e.g. /24, /32)
		`|` + `[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}` + // MAC address (Huawei format)
		`|` + `[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}`, // MAC address (Cisco format)
)

// helpDescriptionRe matches the "description" portion of a help line — it should
// be predominantly alphabetic words (a natural-language description), NOT numeric
// data. True help lines look like: "  keyword     Some description text here"
var helpDescriptionRe = regexp.MustCompile(`^\s*[a-zA-Z<|>][\w<>-]*\s{2,}[A-Z]`)

// IsHelpEcho returns true if the output text looks like CLI help/completion
// output rather than real command output. Exported for use by log_analyzer.
func IsHelpEcho(output string) bool {
	// Fast path: check known fixed signatures first.
	for _, sig := range helpEchoSignatures {
		if strings.Contains(output, sig) {
			return true
		}
	}

	// Heuristic: if ≥80% of non-blank lines match the "keyword  description"
	// pattern and there are at least 3 such lines, it MIGHT be a help menu.
	// But first, check for data indicators — structured data with IPs,
	// community values, CIDR masks etc. is never help output.
	lines := strings.Split(output, "\n")
	total, matched, dataLines, descLines := 0, 0, 0, 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		total++
		if helpEchoLineRe.MatchString(line) {
			matched++
		}
		if dataIndicatorRe.MatchString(line) {
			dataLines++
		}
		if helpDescriptionRe.MatchString(line) {
			descLines++
		}
	}

	// If any line contains structured data indicators (IPs, communities, etc.),
	// this is command output, not a help menu.
	if dataLines > 0 {
		return false
	}

	// Require both: high percentage of "keyword  description" lines AND
	// a significant portion must look like actual help descriptions
	// (starting with an alphabetic keyword followed by a capitalized description).
	if total >= 3 && float64(matched)/float64(total) >= 0.8 {
		// Additional check: at least half the matching lines should look like
		// genuine help descriptions (keyword + English description)
		return descLines >= 2 || float64(descLines)/float64(total) >= 0.4
	}
	return false
}

// ═══════════════════════════════════════════════════════════════════════════
// Error-output detection (Filter 5)
// ═══════════════════════════════════════════════════════════════════════════

// errorPointerRe matches a line containing only whitespace and a caret (^),
// which is used by CLI error messages to indicate the error position.
var errorPointerRe = regexp.MustCompile(`^\s+\^\s*$`)

// IsErrorOutput returns true if the output looks like a CLI error message
// (e.g. "% Invalid input", "Error: ...", or an error pointer "^").
func IsErrorOutput(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "^" || errorPointerRe.MatchString(line) {
			return true
		}
		if strings.HasPrefix(trimmed, "% ") {
			return true
		}
		if strings.HasPrefix(trimmed, "Error:") {
			return true
		}
	}
	return false
}

// ═══════════════════════════════════════════════════════════════════════════
// Control-command filtering
// ═══════════════════════════════════════════════════════════════════════════

// controlCommands lists device navigation/management commands that never
// produce parseable structured data. Matched as exact prefix of the
// normalised (lowercased, verb-expanded) command.
var controlCommands = []string{
	// ── Mode navigation ──
	"quit",
	"return",
	"exit",
	"system-view",
	"sys",
	"end",
	"configure terminal",
	"conf t",
	// ── Save / undo ──
	"save",
	"commit",
	"undo",
	"rollback",
	// ── Terminal control ──
	"screen-length",
	"screen-width",
	"terminal length",
	"terminal width",
	"terminal monitor",
	"user-interface",
	"set cli screen-length",
	"set cli screen-width",
	// ── Paging / more ──
	"more",
	// ── Clock / diagnostics that produce fixed text ──
	"display clock",
	"display version",
	"display device",
	"display logbuffer",
	"display trapbuffer",
	"show clock",
	"show version",
	// NOTE: "show logging" removed — "show logging last N" is a valid diagnostic
	// command and the prefix match would incorrectly filter it.
	"show tech-support",
	"show inventory",
	"show environment",
	// ── Juniper equivalents ──
	"request",
	"restart",
	"set cli",
}

// IsControlCommand returns true if cmd is a device navigation / management
// command that should not be collected as an unknown output.
func IsControlCommand(vendor, cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	// Expand verb abbreviation first, same as normaliseCommand
	switch vendor {
	case "huawei", "h3c":
		if strings.HasPrefix(lower, "dis ") && !strings.HasPrefix(lower, "display ") {
			lower = "display " + lower[4:]
		}
	case "cisco":
		if strings.HasPrefix(lower, "sh ") && !strings.HasPrefix(lower, "show ") {
			lower = "show " + lower[3:]
		}
	}
	lower = strings.Join(strings.Fields(lower), " ")

	for _, ctrl := range controlCommands {
		if lower == ctrl || strings.HasPrefix(lower, ctrl+" ") {
			return true
		}
	}
	return false
}

// ═══════════════════════════════════════════════════════════════════════════
// Normalisation — verb + interior abbreviation expansion
// ═══════════════════════════════════════════════════════════════════════════

// interiorAbbrevs maps common CLI abbreviations to their full keyword.
// Applied after leading-verb expansion, per-word, only when the abbreviation
// is an unambiguous prefix of exactly one expansion.
//
// Network engineer note: these are abbreviations that are universally used
// across Huawei/H3C/Cisco. We deliberately keep this list conservative —
// better to miss an obscure abbreviation than to incorrectly expand one.
var interiorAbbrevs = map[string]string{
	// Interface types
	"int":      "interface",
	"inter":    "interface",
	"lo":       "loopback",
	"loop":     "loopback",
	"vlan":     "vlanif", // Huawei/H3C: "display int Vlanif100" but "display int vlan brief" is ambiguous — keep as-is
	"gi":       "gigabitethernet",
	"ge":       "gigabitethernet",
	// NOTE: "te" removed — ambiguous between "ten-gigabitethernet" (interface)
	// and "te" (Traffic Engineering in segment-routing/MPLS context).
	"xe":       "ten-gigabitethernet",
	"eth-trunk": "eth-trunk", // already full form, no-op
	// Aggregation
	"agg":         "aggregation",
	"link-agg":    "link-aggregation",
	// Protocol prefixes
	"bgp":  "bgp",  // no-op, already full
	"ospf": "ospf", // no-op
	"isis": "isis", // no-op
	"ldp":  "ldp",  // no-op
	"rsvp": "rsvp", // no-op
	"mpls": "mpls", // no-op
	// Modifiers
	"ver":  "verbose",
	"bri":  "brief",
	"det":  "detail",
	"sum":  "summary",
	"summ": "summary",
	// Routing
	"ro":    "routing-table",
	"rout":  "routing-table",
	"route": "route",
	// Config
	"cur":  "current-configuration",
	"curr": "current-configuration",
	"sa":   "saved-configuration",
	"run":  "running-config",
	// ── 新增缩写 ──
	"nei":   "neighbor",          // show bgp nei (Cisco常用缩写)
	"neigh": "neighbor",          // show bgp neigh
	"for":   "forwarding-table",  // display mpls for
	"forw":  "forwarding-table",  // display mpls forw
	"tun":   "tunnel",            // display mpls te tun
	"stat":  "statistics",        // display interface stat
	"desc":  "descriptions",      // show interfaces desc
}

// NormaliseCommand expands the leading verb abbreviation, expands common
// interior abbreviations, lowercases and collapses whitespace.
//
// Examples:
//
//	"dis link-agg ver"    → "display link-aggregation verbose"  (huawei/h3c)
//	"sh int brief"        → "show interface brief"              (cisco)
//	"display int Vlanif100" → "display interface vlanif100"
func NormaliseCommand(vendor, cmd string) string {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	// Step 1: Expand leading verb
	switch vendor {
	case "huawei", "h3c":
		if strings.HasPrefix(lower, "dis ") && !strings.HasPrefix(lower, "display ") {
			lower = "display " + lower[4:]
		}
	case "cisco":
		if strings.HasPrefix(lower, "sh ") && !strings.HasPrefix(lower, "show ") {
			lower = "show " + lower[3:]
		}
	}
	// Step 2: Expand interior abbreviations (skip the verb itself)
	words := strings.Fields(lower)
	for i := 1; i < len(words); i++ {
		if full, ok := interiorAbbrevs[words[i]]; ok {
			words[i] = full
		}
	}
	return strings.Join(words, " ")
}

// ═══════════════════════════════════════════════════════════════════════════
// Argument stripping — separate pattern from instance arguments
// ═══════════════════════════════════════════════════════════════════════════

// qualifierKeywords are CLI keywords that take an argument in the next token.
// When we see one of these, the following word is an argument (VPN name, IP, etc.)
// and should be replaced with a placeholder.
// Reference: docs/command-syntax-knowledge.md §6.2
var qualifierKeywords = map[string]bool{
	// VRF/VPN
	"vpn-instance": true, "vrf": true, "instance": true,
	// Peer/Neighbor
	"peer": true, "neighbor": true, "neighbors": true,
	// Interface/Port (when used as a qualifier, e.g. "interface GigabitEthernet0/0/1")
	"interface": true, "bridge-aggregation": true, "route-aggregation": true,
	"eth-trunk": true, "port-channel": true,
	// Area/Zone
	"area": true,
	// Routing Table
	"table": true,
	// BGP Group
	"group": true,
	// ACL/Policy
	"acl": true, "access-list": true, "route-policy": true, "route-map": true,
	// VLAN
	"vlan": true, "vlan-interface": true,
	// Tunnel
	"tunnel": true,
	// Service
	"service-instance": true,
	// ── 网络通用（全厂商） ──
	"destination": true, "source": true, "community": true,
	"prefix-list": true, "label": true, "locator": true,
	"name": true, "filter": true, "last": true,
	// ── Cisco IOS-XR 专属 ──
	"location": true, "bundle-ether": true,
	"prefix-set": true, "community-set": true, "as-path-set": true,
	"extcommunity-set": true, "prefix": true, "rd": true,
	// ── Juniper JUNOS 专属 ──
	"routing-instance": true, "logical-system": true, "policy-statement": true,
	// ── 华为/H3C 专属 ──
	"ip-prefix": true, "community-filter": true, "as-path-filter": true,
	"configuration": true, "slot": true, "tunnel-policy": true,
}

// modifierKeywords are keywords that are never arguments — they are command
// mode modifiers.  Listed here to prevent false-positive stripping.
var modifierKeywords = map[string]bool{
	"brief": true, "verbose": true, "detail": true, "detailed": true,
	"summary": true, "statistics": true, "terse": true, "extensive": true,
	"descriptions": true, "accounting": true, "status": true,
	"ingress": true, "transit": true, "egress": true,
	"level-1": true, "level-2": true, "level-1-2": true,
	"unicast": true, "multicast": true,
	"advertised-routes": true, "received-routes": true,
	"longer-prefixes": true,
	"ipv4": true, "ipv6": true, "vpnv4": true, "vpnv6": true,
	// ── 新增 modifier 关键字 ──
	"all": true,                                                    // display bgp vpnv4 all peer
	"active": true, "inactive": true,                               // show route active/inactive-path
	"exact": true,                                                  // show route exact
	"no-resolve": true,                                             // show arp no-resolve (Juniper)
	"inbound": true, "outbound": true,                              // 流量策略方向
	"router": true, "network": true, "nssa": true, "opaque": true, // LSDB类型
	"comprehensive": true,                                          // Juniper QoS
	"external": true,                                               // OSPF external routes
}

// Interface name patterns that represent instance arguments rather than
// command keywords. Matches: GigabitEthernet0/0/1, Bridge-Aggregation1,
// Vlanif100, Eth-Trunk1, Loopback0, ge-0/0/0, xe-0/0/1, ae0, etc.
var interfaceNameRe = regexp.MustCompile(`(?i)^(gigabitethernet|ten-gigabitethernet|` +
	`twentyfivegige|fortygige|hundredgige|` +
	`bridge-aggregation|bagg|eth-trunk|port-channel|` +
	`vlanif|vlan-interface|loopback|null|tunnel|nve|` +
	`mgmteth|management|serial|pos|` +
	`ge-|xe-|et-|ae|irb|em|fxp)\d`)

// ipv4Re matches IPv4 addresses with optional CIDR mask.
var ipv4Re = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(/\d{1,2})?$`)

// pureNumberRe matches a standalone number (VLAN ID, interface index, AS number, etc.)
var pureNumberRe = regexp.MustCompile(`^\d+$`)

// stripArgs replaces instance arguments at any position in a normalised command
// with placeholders so that different instance queries share the same command_norm.
//
// The algorithm scans left-to-right (skipping the verb):
//  1. If word is a qualifier keyword → next word becomes an argument placeholder.
//  2. If word matches an IP address → replace with {ip}.
//  3. If word matches an interface name → replace with {interface}.
//  4. Standalone trailing numbers → replace with {id}.
//  5. Modifier keywords and unknown keywords → kept as-is.
//
// Examples:
//
//	"display bgp vpnv4 vpn-instance VPN_A peer 10.0.0.1 verbose"
//	  → "display bgp vpnv4 vpn-instance {name} peer {ip} verbose"
//	"display interface gigabitethernet0/0/1 brief"
//	  → "display interface {interface} brief"
//	"display link-aggregation verbose bridge-aggregation 1"
//	  → "display link-aggregation verbose bridge-aggregation {id}"
//	"display vlan 100"
//	  → "display vlan {id}"
//	"display ospf 100 peer brief"
//	  → "display ospf {id} peer brief"
func StripArgs(norm string) string {
	words := strings.Fields(norm)
	if len(words) <= 1 {
		return norm
	}

	result := make([]string, len(words))
	copy(result, words)

	for i := 1; i < len(result); i++ {
		w := result[i]

		// Rule 1: qualifier keyword → next word is an argument
		if qualifierKeywords[w] && i+1 < len(result) {
			next := result[i+1]
			// Don't replace the next word if it's also a keyword
			if !modifierKeywords[next] && !qualifierKeywords[next] {
				result[i+1] = classifyArgPlaceholder(next)
				i++ // skip the argument we just replaced
			}
			continue
		}

		// Rule 2: modifier / known keyword → never an argument
		if modifierKeywords[w] || qualifierKeywords[w] {
			continue
		}

		// Rule 3: IP address at any position
		if ipv4Re.MatchString(w) {
			result[i] = "{ip}"
			continue
		}

		// Rule 4: interface name at any position
		if interfaceNameRe.MatchString(w) {
			result[i] = "{interface}"
			continue
		}

		// Rule 5: standalone number — only strip if not immediately after the verb
		// (verbs like "display" at position 0; word at position 1 could be the object)
		if pureNumberRe.MatchString(w) {
			result[i] = "{id}"
			continue
		}
	}

	return strings.Join(result, " ")
}

// classifyArgPlaceholder picks the right placeholder for an argument value.
func classifyArgPlaceholder(arg string) string {
	if ipv4Re.MatchString(arg) {
		return "{ip}"
	}
	if interfaceNameRe.MatchString(arg) {
		return "{interface}"
	}
	if pureNumberRe.MatchString(arg) {
		return "{id}"
	}
	// Free-text argument (VPN name, policy name, etc.)
	return "{name}"
}

// ═══════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════

// hashContent returns the first 16 hex chars of SHA256(s).
func hashContent(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:8])
}
