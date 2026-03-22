package parser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// ---------------------------------------------------------------------------
// Brace-aware block extraction helpers
// ---------------------------------------------------------------------------

// extractJuniperBlock finds "keyword {" at the appropriate nesting level and
// returns everything between the matching braces (exclusive).  It handles
// arbitrary nesting.  keyword may be a multi-word prefix such as
// "protocols bgp" — every word must appear on the same line (in order) before
// the opening brace.
func extractJuniperBlock(config, keyword string) string {
	lines := strings.Split(config, "\n")
	startIdx := -1
	depth := 0

	kw := strings.TrimSpace(keyword)
	// Build a simple check: every word must appear in order.
	kwWords := strings.Fields(kw)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if startIdx < 0 {
			if matchKeywordLine(trimmed, kwWords) {
				startIdx = i
				// Count braces on this line; the opening { should be here.
				depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
				if depth <= 0 {
					// Degenerate: single-line block.
					return ""
				}
			}
			continue
		}
		depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
		if depth <= 0 {
			// Return content between startIdx+1 .. i-1
			return strings.Join(lines[startIdx+1:i], "\n")
		}
	}
	return ""
}

// matchKeywordLine checks that trimmed contains all kwWords in order and ends
// with "{" (possibly with trailing whitespace).
func matchKeywordLine(trimmed string, kwWords []string) bool {
	if !strings.Contains(trimmed, "{") {
		return false
	}
	pos := 0
	for _, w := range kwWords {
		idx := strings.Index(trimmed[pos:], w)
		if idx < 0 {
			return false
		}
		pos += idx + len(w)
	}
	return true
}

// extractNamedBlocks finds all occurrences of "<prefix> <name> {" inside
// blockText and returns a slice of (name, innerContent) pairs.
func extractNamedBlocks(blockText, prefix string) []struct {
	Name    string
	Content string
} {
	lines := strings.Split(blockText, "\n")
	var results []struct {
		Name    string
		Content string
	}

	re := regexp.MustCompile(`(?i)^\s*` + regexp.QuoteMeta(prefix) + `\s+(\S+)\s*\{`)

	for i := 0; i < len(lines); i++ {
		m := re.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		name := m[1]
		depth := strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
		start := i + 1
		for j := start; j < len(lines); j++ {
			depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
			if depth <= 0 {
				results = append(results, struct {
					Name    string
					Content string
				}{Name: name, Content: strings.Join(lines[start:j], "\n")})
				i = j
				break
			}
		}
	}
	return results
}

// extractBracedBlock extracts the content inside a "keyword {" block from the
// given text, where keyword appears at the start of a trimmed line.
func extractBracedBlock(text, keyword string) string {
	return extractJuniperBlock(text, keyword)
}

// ---------------------------------------------------------------------------
// Simple value extractors (for lines like "type internal;")
// ---------------------------------------------------------------------------

func juniperValue(block, key string) string {
	return juniperValueAtDepth(block, key, -1)
}

// juniperValueTopLevel extracts a value only from the top level (depth 0) of
// a block, ignoring any matches inside nested braces.
func juniperValueTopLevel(block, key string) string {
	return juniperValueAtDepth(block, key, 0)
}

// juniperValueAtDepth extracts the first value for key at the given brace
// depth.  If targetDepth < 0, any depth matches (same as juniperValue).
func juniperValueAtDepth(block, key string, targetDepth int) string {
	re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s+([^;{]+);`)
	depth := 0
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		opens := strings.Count(trimmed, "{")
		closes := strings.Count(trimmed, "}")

		if targetDepth < 0 || depth == targetDepth {
			if m := re.FindStringSubmatch(trimmed); m != nil {
				return strings.TrimSpace(m[1])
			}
		}
		depth += opens - closes
	}
	return ""
}

// juniperValues returns all values for repeated keys (e.g. multiple
// "interface" lines).
func juniperValues(block, key string) []string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s+([^;{]+);`)
	matches := re.FindAllStringSubmatch(block, -1)
	var out []string
	for _, m := range matches {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// ---------------------------------------------------------------------------
// BGP Peer Extraction
// ---------------------------------------------------------------------------

// juniperFamilyMap maps Juniper family block names to canonical address-family
// strings.
var juniperFamilyMap = map[string]string{
	"inet":     "ipv4-unicast",
	"inet-vpn": "vpnv4",
	"inet6-vpn": "vpnv6",
	"inet6":    "ipv6-unicast",
}

// ExtractBGPPeersJuniper parses hierarchical Juniper JUNOS config and returns
// one BGPPeer per (neighbor, address-family) combination.
func ExtractBGPPeersJuniper(configText string) []model.BGPPeer {
	var peers []model.BGPPeer

	// Determine local AS from routing-options { autonomous-system <N>; }
	// There may be multiple routing-options blocks (e.g. per routing-engine);
	// search all of them for autonomous-system.
	localAS := findJuniperLocalAS(configText)
	// Also check for local-as in the BGP block itself
	if localAS == 0 {
		bgpBlock := extractJuniperBlock(configText, "protocols")
		if bgpBlock != "" {
			bgpInner := extractJuniperBlock(bgpBlock, "bgp")
			if bgpInner != "" {
				if v := juniperValue(bgpInner, "local-as"); v != "" {
					localAS, _ = strconv.Atoi(v)
				}
			}
		}
	}

	// Collect BGP groups from the top-level protocols block.
	peers = append(peers, extractBGPGroupsFrom(configText, "", localAS)...)

	return peers
}

// findJuniperLocalAS searches through all routing-options blocks in the config
// to find the autonomous-system number.  There may be multiple routing-options
// blocks (e.g. per routing-engine group); we search all of them.
func findJuniperLocalAS(configText string) int {
	re := regexp.MustCompile(`(?m)^\s*autonomous-system\s+(\d+);`)
	// First try: search all routing-options blocks
	lines := strings.Split(configText, "\n")
	roRe := regexp.MustCompile(`(?i)^\s*routing-options\s*\{`)
	for i := 0; i < len(lines); i++ {
		if !roRe.MatchString(lines[i]) {
			continue
		}
		depth := 0
		for j := i; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
			if m := re.FindStringSubmatch(trimmed); m != nil {
				v, _ := strconv.Atoi(m[1])
				if v > 0 {
					return v
				}
			}
			if depth <= 0 && j > i {
				break
			}
		}
	}
	// Fallback: look for local-as in protocols bgp block
	bgpBlock := extractJuniperBlock(configText, "protocols")
	if bgpBlock != "" {
		bgpInner := extractJuniperBlock(bgpBlock, "bgp")
		if bgpInner != "" {
			if v := juniperValue(bgpInner, "local-as"); v != "" {
				as, _ := strconv.Atoi(v)
				return as
			}
		}
	}
	return 0
}

// extractBGPGroupsFrom finds "protocols { bgp {" inside text and extracts all
// groups/neighbors. vrfName is "" for global table. localAS is the autonomous
// system number for iBGP remote-as resolution.
func extractBGPGroupsFrom(text, vrfName string, localAS int) []model.BGPPeer {
	bgpBlock := extractJuniperBlock(text, "protocols")
	if bgpBlock == "" {
		return nil
	}
	bgpInner := extractJuniperBlock(bgpBlock, "bgp")
	if bgpInner == "" {
		// Maybe "protocols { bgp {" on same level — try direct.
		bgpInner = extractJuniperBlock(text, "protocols bgp")
		if bgpInner == "" {
			return nil
		}
	}

	var peers []model.BGPPeer
	groups := extractNamedBlocks(bgpInner, "group")
	for _, g := range groups {
		peers = append(peers, parseJuniperBGPGroup(g.Name, g.Content, vrfName, localAS)...)
	}
	return peers
}

func parseJuniperBGPGroup(groupName, groupContent, vrfName string, localAS int) []model.BGPPeer {
	groupType := juniperValue(groupContent, "type")
	localAddr := juniperValue(groupContent, "local-address")

	// Determine peer-as at group level (for external groups).
	groupPeerAS := 0
	if v := juniperValue(groupContent, "peer-as"); v != "" {
		groupPeerAS, _ = strconv.Atoi(v)
	}

	// Group-level import/export (top-level only, not from nested neighbor blocks).
	groupImport := juniperValueTopLevel(groupContent, "import")
	groupExport := juniperValueTopLevel(groupContent, "export")

	// Collect address families from group-level "family" blocks.
	families := parseJuniperFamilies(groupContent)
	if len(families) == 0 {
		families = []string{"ipv4-unicast"} // default
	}

	// Parse neighbors.
	neighbors := extractNamedBlocks(groupContent, "neighbor")
	var peers []model.BGPPeer
	for _, nb := range neighbors {
		desc := juniperValue(nb.Content, "description")
		impPolicy := juniperValueTopLevel(nb.Content, "import")
		if impPolicy == "" {
			impPolicy = groupImport
		}
		expPolicy := juniperValueTopLevel(nb.Content, "export")
		if expPolicy == "" {
			expPolicy = groupExport
		}
		peerAS := groupPeerAS
		if v := juniperValue(nb.Content, "peer-as"); v != "" {
			peerAS, _ = strconv.Atoi(v)
		}

		isIBGP := strings.EqualFold(groupType, "internal")

		// For iBGP, remote AS equals local AS when peer-as is not explicitly set.
		if isIBGP && peerAS == 0 && localAS > 0 {
			peerAS = localAS
		}

		for _, af := range families {
			p := model.BGPPeer{
				VRF:           vrfName,
				PeerIP:        nb.Name,
				PeerGroup:     groupName,
				Description:   desc,
				UpdateSource:  localAddr,
				ImportPolicy:  impPolicy,
				ExportPolicy:  expPolicy,
				RemoteAS:      peerAS,
				LocalAS:       localAS,
				AddressFamily: af,
				Enabled:       1,
			}
			if isIBGP {
				// Already set RemoteAS above
			}
			peers = append(peers, p)
		}
	}
	return peers
}

// parseJuniperFamilies scans a block for "family <name> {" entries and maps
// them to canonical address-family names.
func parseJuniperFamilies(block string) []string {
	famBlocks := extractNamedBlocks(block, "family")
	seen := map[string]bool{}
	var result []string
	for _, fb := range famBlocks {
		if af, ok := juniperFamilyMap[fb.Name]; ok && !seen[af] {
			seen[af] = true
			result = append(result, af)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// VRF Instance Extraction
// ---------------------------------------------------------------------------

// ExtractVRFInstancesJuniper parses "routing-instances { ... }" and returns
// VRFInstance records.  It also appends any BGP peers found inside VRF
// routing-instances to the returned slice (callers should use
// ExtractBGPPeersJuniper for global peers separately).
func ExtractVRFInstancesJuniper(configText string) []model.VRFInstance {
	riBlock := extractJuniperBlock(configText, "routing-instances")
	if riBlock == "" {
		return nil
	}

	instances := extractNamedBlocks(riBlock, "")
	if len(instances) == 0 {
		// Fall back: parse each top-level name block inside routing-instances.
		instances = extractAllTopLevelBlocks(riBlock)
	}

	var vrfs []model.VRFInstance
	for _, inst := range instances {
		instType := juniperValue(inst.Content, "instance-type")
		if !strings.EqualFold(instType, "vrf") {
			continue
		}

		rd := juniperValue(inst.Content, "route-distinguisher")
		impPolicy := juniperValue(inst.Content, "vrf-import")
		expPolicy := juniperValue(inst.Content, "vrf-export")
		ifaces := juniperValues(inst.Content, "interface")

		vrf := model.VRFInstance{
			VRFName:      inst.Name,
			RD:           rd,
			ImportPolicy: impPolicy,
			ExportPolicy: expPolicy,
		}
		_ = ifaces // interfaces are not stored in VRFInstance model currently

		vrfs = append(vrfs, vrf)
	}
	return vrfs
}

// ExtractVRFBGPPeersJuniper extracts BGP peers defined inside
// routing-instances (VRF context).
func ExtractVRFBGPPeersJuniper(configText string) []model.BGPPeer {
	riBlock := extractJuniperBlock(configText, "routing-instances")
	if riBlock == "" {
		return nil
	}

	// Determine local AS for iBGP resolution
	localAS := findJuniperLocalAS(configText)

	instances := extractAllTopLevelBlocks(riBlock)
	var peers []model.BGPPeer
	for _, inst := range instances {
		instType := juniperValue(inst.Content, "instance-type")
		if !strings.EqualFold(instType, "vrf") {
			continue
		}
		vrfPeers := extractBGPGroupsFrom(inst.Content, inst.Name, localAS)
		peers = append(peers, vrfPeers...)
	}
	return peers
}

// extractAllTopLevelBlocks parses a block of text and returns each top-level
// "name {" … "}" as a named block.  This works for routing-instances entries
// that don't share a common keyword prefix.
func extractAllTopLevelBlocks(text string) []struct {
	Name    string
	Content string
} {
	lines := strings.Split(text, "\n")
	re := regexp.MustCompile(`^\s*(\S+)\s*\{`)

	var results []struct {
		Name    string
		Content string
	}
	for i := 0; i < len(lines); i++ {
		m := re.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		name := m[1]
		depth := strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
		start := i + 1
		for j := start; j < len(lines); j++ {
			depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
			if depth <= 0 {
				results = append(results, struct {
					Name    string
					Content string
				}{Name: name, Content: strings.Join(lines[start:j], "\n")})
				i = j
				break
			}
		}
	}
	return results
}

// ---------------------------------------------------------------------------
// Route Policy Extraction
// ---------------------------------------------------------------------------

// ExtractRoutePoliciesJuniper parses "policy-options { ... }" and extracts
// policy-statement definitions with their terms.
func ExtractRoutePoliciesJuniper(configText string) []model.RoutePolicy {
	poBlock := extractJuniperBlock(configText, "policy-options")
	if poBlock == "" {
		return nil
	}

	statements := extractNamedBlocks(poBlock, "policy-statement")
	var policies []model.RoutePolicy
	for _, stmt := range statements {
		pol := model.RoutePolicy{
			PolicyName: stmt.Name,
			VendorType: "policy-statement",
			RawText:    "policy-statement " + stmt.Name + " {\n" + stmt.Content + "\n}",
		}

		// Parse terms.
		terms := extractNamedBlocks(stmt.Content, "term")
		seq := 10
		for _, t := range terms {
			node := model.RoutePolicyNode{
				Sequence: seq,
				TermName: t.Name,
			}
			node.MatchClauses = extractFromBlock(t.Content)
			node.ApplyClauses, node.Action = extractThenBlock(t.Content)
			pol.Nodes = append(pol.Nodes, node)
			seq += 10
		}

		// Check for a top-level "then" (default action) outside any term.
		topThenApply, topThenAction := extractTopLevelThen(stmt.Content)
		if topThenAction != "" {
			pol.Nodes = append(pol.Nodes, model.RoutePolicyNode{
				Sequence:     seq,
				TermName:     "_default",
				Action:       topThenAction,
				ApplyClauses: topThenApply,
			})
		}

		policies = append(policies, pol)
	}
	return policies
}

// extractFromBlock extracts the content of "from { ... }" in a term and
// returns it as a JSON-style string.
func extractFromBlock(termContent string) string {
	fb := extractBracedBlock(termContent, "from")
	if fb == "" {
		return ""
	}
	// Collect each non-empty trimmed line as a match clause.
	var clauses []string
	for _, line := range strings.Split(fb, "\n") {
		t := strings.TrimSpace(line)
		t = strings.TrimSuffix(t, ";")
		t = strings.TrimSpace(t)
		if t != "" && t != "}" {
			clauses = append(clauses, t)
		}
	}
	if len(clauses) == 0 {
		return ""
	}
	return "[" + joinQuoted(clauses) + "]"
}

// extractThenBlock extracts the content of "then { ... }" in a term.
// Returns (applyClauses JSON string, action string).
func extractThenBlock(termContent string) (string, string) {
	tb := extractBracedBlock(termContent, "then")
	if tb == "" {
		return "", ""
	}
	var clauses []string
	action := ""
	for _, line := range strings.Split(tb, "\n") {
		t := strings.TrimSpace(line)
		t = strings.TrimSuffix(t, ";")
		t = strings.TrimSpace(t)
		if t == "" || t == "}" {
			continue
		}
		if t == "accept" || t == "reject" {
			action = t
		} else {
			clauses = append(clauses, t)
		}
	}
	clauseStr := ""
	if len(clauses) > 0 {
		clauseStr = "[" + joinQuoted(clauses) + "]"
	}
	return clauseStr, action
}

// extractTopLevelThen finds a "then" that is NOT inside a "term" block.
func extractTopLevelThen(stmtContent string) (string, string) {
	lines := strings.Split(stmtContent, "\n")
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		opens := strings.Count(trimmed, "{")
		closes := strings.Count(trimmed, "}")

		if depth == 0 {
			// Check for "then <action>;" on a single line (no braces).
			if strings.HasPrefix(trimmed, "then ") && !strings.Contains(trimmed, "{") {
				action := strings.TrimSuffix(strings.TrimPrefix(trimmed, "then "), ";")
				action = strings.TrimSpace(action)
				if action == "accept" || action == "reject" {
					return "", action
				}
			}
			// Check for "then {" block at depth 0.
			if strings.HasPrefix(trimmed, "then") && strings.Contains(trimmed, "{") {
				// This is a top-level then block — extract it.
				thenBlock := extractJuniperBlock(strings.Join(lines[i:], "\n"), "then")
				return extractThenBlock("then {\n" + thenBlock + "\n}")
			}
		}
		depth += opens - closes
	}
	return "", ""
}

func joinQuoted(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return strings.Join(quoted, ",")
}
