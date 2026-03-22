package parser

import (
	"encoding/json"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// ---------- BGP Peers ----------

var (
	bgpHeaderRe     = regexp.MustCompile(`(?m)^bgp\s+(\d+)`)
	afHeaderRe      = regexp.MustCompile(`(?i)^(ipv[46]-family)\s+(.+)`)
	peerASRe        = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+as-number\s+(\d+)`)
	peerGroupDefRe  = regexp.MustCompile(`(?i)^group\s+(\S+)\s+(internal|external)`)
	peerGroupRefRe  = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+group\s+(\S+)`)
	peerDescRe      = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+description\s+(.+)`)
	peerConnIfRe    = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+connect-interface\s+(\S+)`)
	peerBFDRe       = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+bfd\s+enable`)
	peerEnableRe    = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+enable`)
	peerUndoEnRe    = regexp.MustCompile(`(?i)^undo\s+peer\s+(\S+)\s+enable`)
	peerRoutPolRe   = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+route-policy\s+(\S+)\s+(import|export)`)
	peerAdvCommRe   = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+advertise-community`)
	peerNextHopRe   = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+next-hop-local`)
	peerSoftRecfgRe = regexp.MustCompile(`(?i)^peer\s+(\S+)\s+soft-reconfiguration`)
	routerIDRe      = regexp.MustCompile(`(?i)^router-id\s+(\S+)`)
)

// isIPAddress returns true if s looks like an IPv4 or IPv6 address (not a group name).
func isIPAddress(s string) bool {
	return net.ParseIP(s) != nil
}

// bgpGlobalInfo holds info extracted from the BGP global section.
type bgpGlobalInfo struct {
	as          int
	peerAS      map[string]int    // ip -> remote AS
	peerGroup   map[string]string // ip -> group name
	groupType   map[string]string // group name -> "internal"/"external"
	peerDesc    map[string]string // ip -> description
	peerConnIf  map[string]string // ip-or-group -> connect-interface
	peerBFD     map[string]bool   // ip-or-group -> bfd enabled
	groupAS     map[string]int    // group name -> AS (from peer <group> as-number)
	groupConnIf map[string]string // group name -> connect-interface
	groupBFD    map[string]bool   // group name -> bfd enabled
}

// bgpAFInfo holds info for one address-family section.
type bgpAFInfo struct {
	af          string // e.g. "ipv4-unicast", "vpnv4", "vpnv6"
	vrf         string
	enabled     map[string]bool   // ip-or-group -> true
	disabled    map[string]bool   // ip-or-group -> true (undo peer X enable)
	importPol   map[string]string // ip-or-group -> policy name
	exportPol   map[string]string // ip-or-group -> policy name
	advComm     map[string]bool   // ip-or-group -> true
	nextHop     map[string]bool
	softReconfig map[string]bool
	peerAS      map[string]int    // ip -> AS (vpn-instance sections can define peers)
	peerGroup   map[string]string // ip -> group name
	peerDesc    map[string]string
	peerBFD     map[string]bool
	groupType   map[string]string // group -> internal/external
}

func newBGPAFInfo(af, vrf string) *bgpAFInfo {
	return &bgpAFInfo{
		af:           af,
		vrf:          vrf,
		enabled:      make(map[string]bool),
		disabled:     make(map[string]bool),
		importPol:    make(map[string]string),
		exportPol:    make(map[string]string),
		advComm:      make(map[string]bool),
		nextHop:      make(map[string]bool),
		softReconfig: make(map[string]bool),
		peerAS:       make(map[string]int),
		peerGroup:    make(map[string]string),
		peerDesc:     make(map[string]string),
		peerBFD:      make(map[string]bool),
		groupType:    make(map[string]string),
	}
}

// ExtractBGPPeersHuawei extracts BGP peers from Huawei/H3C running-config.
// Works for both vendors since they share the same BGP config syntax.
func ExtractBGPPeersHuawei(configText string) []model.BGPPeer {
	// Find the BGP block: starts at "bgp <AS>" line.
	lines := strings.Split(configText, "\n")
	bgpStart := -1
	localAS := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := bgpHeaderRe.FindStringSubmatch(trimmed); m != nil {
			localAS, _ = strconv.Atoi(m[1])
			bgpStart = i
			break
		}
	}
	if bgpStart < 0 {
		return nil
	}

	// Collect BGP block lines: from bgpStart+1 until a line that is NOT indented
	// and NOT starting with ipv4-family/ipv6-family and NOT '#' (# separates AF sections within BGP).
	// We also need to stop at the next top-level section (non-blank, non-#, non-indented, not an AF header).
	// Strategy: split by "#" delimiter within the BGP block.
	var bgpLines []string
	for i := bgpStart + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// A top-level config section starts at column 0 with a non-empty, non-# line
		// that is not an AF header and not a BGP sub-command (indented).
		if trimmed == "#" {
			bgpLines = append(bgpLines, line)
			continue
		}
		if trimmed == "" {
			bgpLines = append(bgpLines, line)
			continue
		}

		// If line starts at column 0 (no leading space) and is not an AF header
		// and is not a known BGP sub-command, it's a new top-level section.
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			// Check if it's part of BGP (peer, group, router-id, ipv4-family, ipv6-family, etc.)
			if !isBGPSubLine(trimmed) {
				break
			}
		}
		bgpLines = append(bgpLines, line)
	}

	// Split BGP block into sections by '#'
	sections := splitByHash(bgpLines)

	// First section is global
	global := bgpGlobalInfo{
		as:          localAS,
		peerAS:      make(map[string]int),
		peerGroup:   make(map[string]string),
		groupType:   make(map[string]string),
		peerDesc:    make(map[string]string),
		peerConnIf:  make(map[string]string),
		peerBFD:     make(map[string]bool),
		groupAS:     make(map[string]int),
		groupConnIf: make(map[string]string),
		groupBFD:    make(map[string]bool),
	}

	var afSections []*bgpAFInfo

	for idx, sec := range sections {
		if idx == 0 {
			// Global section (before any AF header)
			// But check if first line is an AF header
			if len(sec) > 0 {
				trimFirst := strings.TrimSpace(sec[0])
				if afHeaderRe.MatchString(trimFirst) {
					// No global section; this is an AF section
					afSections = append(afSections, parseAFSection(sec))
					continue
				}
			}
			parseGlobalSection(sec, &global)
		} else {
			// Check if this section starts with an AF header
			if len(sec) > 0 {
				trimFirst := strings.TrimSpace(sec[0])
				if afHeaderRe.MatchString(trimFirst) {
					afSections = append(afSections, parseAFSection(sec))
				} else {
					// Could be continuation of global (unusual) — parse as global
					parseGlobalSection(sec, &global)
				}
			}
		}
	}

	// Resolve group->IP mappings for connect-interface and BFD from global
	for ip, grp := range global.peerGroup {
		if _, ok := global.peerConnIf[ip]; !ok {
			if v, ok := global.peerConnIf[grp]; ok {
				global.peerConnIf[ip] = v
			}
		}
		if !global.peerBFD[ip] {
			if global.peerBFD[grp] {
				global.peerBFD[ip] = true
			}
		}
	}
	// Also resolve group AS: if a group has an AS, peers in that group inherit it
	for ip, grp := range global.peerGroup {
		if _, ok := global.peerAS[ip]; !ok {
			if as, ok := global.groupAS[grp]; ok {
				global.peerAS[ip] = as
			}
		}
	}

	// Build peers from AF sections
	type peerKey struct {
		ip  string
		af  string
		vrf string
	}
	peers := make(map[peerKey]*model.BGPPeer)

	for _, afi := range afSections {
		// Merge AF-level peer/group definitions with global
		// In vpn-instance sections, peers and groups can be defined locally
		allPeerAS := mergeMaps(global.peerAS, afi.peerAS)
		allPeerGroup := mergeMaps(global.peerGroup, afi.peerGroup)
		allPeerDesc := mergeMaps(global.peerDesc, afi.peerDesc)
		allGroupType := mergeMaps(global.groupType, afi.groupType)

		// Collect all IPs that are enabled (or have group enabled) in this AF
		enabledIPs := make(map[string]bool)
		disabledIPs := make(map[string]bool)

		// Direct enables/disables
		for name, v := range afi.enabled {
			if v && isIPAddress(name) {
				enabledIPs[name] = true
			}
		}
		for name, v := range afi.disabled {
			if v && isIPAddress(name) {
				disabledIPs[name] = true
			}
		}

		// Group enables: if "peer <group> enable", all IPs in that group are enabled
		for name, v := range afi.enabled {
			if v && !isIPAddress(name) {
				// It's a group name — enable all IPs in this group
				for ip, grp := range allPeerGroup {
					if grp == name && isIPAddress(ip) {
						if !disabledIPs[ip] {
							enabledIPs[ip] = true
						}
					}
				}
			}
		}

		// For vpn-instance AF sections, also include peers defined locally that have AS
		for ip := range afi.peerAS {
			if isIPAddress(ip) {
				if !disabledIPs[ip] {
					enabledIPs[ip] = true
				}
			}
		}

		// Now resolve group policies to individual IPs
		resolveGroupPol := func(m map[string]string) map[string]string {
			resolved := make(map[string]string)
			for name, pol := range m {
				if isIPAddress(name) {
					resolved[name] = pol
				} else {
					// Group: apply to all IPs in that group
					for ip, grp := range allPeerGroup {
						if grp == name && isIPAddress(ip) {
							if _, exists := resolved[ip]; !exists {
								resolved[ip] = pol
							}
						}
					}
				}
			}
			return resolved
		}

		resolveGroupBool := func(m map[string]bool) map[string]bool {
			resolved := make(map[string]bool)
			for name, val := range m {
				if isIPAddress(name) {
					resolved[name] = val
				} else {
					for ip, grp := range allPeerGroup {
						if grp == name && isIPAddress(ip) {
							if !resolved[ip] {
								resolved[ip] = val
							}
						}
					}
				}
			}
			return resolved
		}

		importPols := resolveGroupPol(afi.importPol)
		exportPols := resolveGroupPol(afi.exportPol)
		advComms := resolveGroupBool(afi.advComm)
		nextHops := resolveGroupBool(afi.nextHop)
		softRecfgs := resolveGroupBool(afi.softReconfig)

		for ip := range enabledIPs {
			key := peerKey{ip: ip, af: afi.af, vrf: afi.vrf}
			p := &model.BGPPeer{
				LocalAS:       localAS,
				PeerIP:        ip,
				AddressFamily: afi.af,
				VRF:           afi.vrf,
				Enabled:       1,
			}
			if as, ok := allPeerAS[ip]; ok {
				p.RemoteAS = as
			}
			if grp, ok := allPeerGroup[ip]; ok {
				p.PeerGroup = grp
				// Determine if eBGP multihop based on group type
				if allGroupType[grp] == "external" || (p.RemoteAS != 0 && p.RemoteAS != localAS) {
					// external
				}
			}
			if desc, ok := allPeerDesc[ip]; ok {
				p.Description = desc
			}
			if connIf, ok := global.peerConnIf[ip]; ok {
				p.UpdateSource = connIf
			}
			if global.peerBFD[ip] || afi.peerBFD[ip] {
				p.BFDEnabled = 1
			}
			if pol, ok := importPols[ip]; ok {
				p.ImportPolicy = pol
			}
			if pol, ok := exportPols[ip]; ok {
				p.ExportPolicy = pol
			}
			if advComms[ip] {
				p.AdvertiseCommunity = 1
			}
			if nextHops[ip] {
				p.NextHopSelf = 1
			}
			if softRecfgs[ip] {
				p.SoftReconfig = 1
			}

			peers[key] = p
		}
	}

	result := make([]model.BGPPeer, 0, len(peers))
	for _, p := range peers {
		result = append(result, *p)
	}
	return result
}

func isBGPSubLine(trimmed string) bool {
	lower := strings.ToLower(trimmed)
	prefixes := []string{
		"peer ", "group ", "router-id ", "timer ",
		"ipv4-family", "ipv6-family", "l2vpn-family",
		"undo ", "graceful-restart", "default ",
		"import-route", "network ", "aggregate ",
		"preference ", "nexthop ", "bestroute ",
		"reflect ", "confederation",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func splitByHash(lines []string) [][]string {
	var sections [][]string
	var current []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "#" {
			if len(current) > 0 {
				sections = append(sections, current)
			}
			current = nil
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		sections = append(sections, current)
	}
	return sections
}

func parseGlobalSection(lines []string, g *bgpGlobalInfo) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if m := peerASRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			as, _ := strconv.Atoi(m[2])
			if isIPAddress(name) {
				g.peerAS[name] = as
			} else {
				g.groupAS[name] = as
			}
		} else if m := peerGroupDefRe.FindStringSubmatch(trimmed); m != nil {
			g.groupType[m[1]] = m[2]
		} else if m := peerGroupRefRe.FindStringSubmatch(trimmed); m != nil {
			g.peerGroup[m[1]] = m[2]
		} else if m := peerDescRe.FindStringSubmatch(trimmed); m != nil {
			g.peerDesc[m[1]] = strings.TrimSpace(m[2])
		} else if m := peerConnIfRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if isIPAddress(name) {
				g.peerConnIf[name] = m[2]
			} else {
				g.groupConnIf[name] = m[2]
				g.peerConnIf[name] = m[2]
			}
		} else if m := peerBFDRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			if isIPAddress(name) {
				g.peerBFD[name] = true
			} else {
				g.groupBFD[name] = true
				g.peerBFD[name] = true
			}
		}
	}
}

func parseAFSection(lines []string) *bgpAFInfo {
	af := "ipv4-unicast"
	vrf := ""

	if len(lines) > 0 {
		trimFirst := strings.TrimSpace(lines[0])
		if m := afHeaderRe.FindStringSubmatch(trimFirst); m != nil {
			family := strings.ToLower(m[1])
			rest := strings.TrimSpace(m[2])

			switch {
			case strings.EqualFold(rest, "unicast"):
				if family == "ipv4-family" {
					af = "ipv4-unicast"
				} else {
					af = "ipv6-unicast"
				}
			case strings.EqualFold(rest, "vpnv4"):
				af = "vpnv4"
			case strings.EqualFold(rest, "vpnv6"):
				af = "vpnv6"
			case strings.HasPrefix(strings.ToLower(rest), "vpn-instance "):
				parts := strings.Fields(rest)
				if len(parts) >= 2 {
					vrf = parts[1]
				}
				if family == "ipv4-family" {
					af = "ipv4-unicast"
				} else {
					af = "ipv6-unicast"
				}
			default:
				af = strings.ToLower(strings.ReplaceAll(rest, " ", "-"))
			}
		}
	}

	afi := newBGPAFInfo(af, vrf)

	startIdx := 1 // skip header line
	for _, line := range lines[startIdx:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if m := peerUndoEnRe.FindStringSubmatch(trimmed); m != nil {
			afi.disabled[m[1]] = true
		} else if m := peerEnableRe.FindStringSubmatch(trimmed); m != nil {
			afi.enabled[m[1]] = true
		} else if m := peerRoutPolRe.FindStringSubmatch(trimmed); m != nil {
			dir := strings.ToLower(m[3])
			if dir == "import" {
				afi.importPol[m[1]] = m[2]
			} else {
				afi.exportPol[m[1]] = m[2]
			}
		} else if m := peerAdvCommRe.FindStringSubmatch(trimmed); m != nil {
			afi.advComm[m[1]] = true
		} else if m := peerNextHopRe.FindStringSubmatch(trimmed); m != nil {
			afi.nextHop[m[1]] = true
		} else if m := peerSoftRecfgRe.FindStringSubmatch(trimmed); m != nil {
			afi.softReconfig[m[1]] = true
		} else if m := peerASRe.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			as, _ := strconv.Atoi(m[2])
			afi.peerAS[name] = as
		} else if m := peerGroupRefRe.FindStringSubmatch(trimmed); m != nil {
			afi.peerGroup[m[1]] = m[2]
		} else if m := peerGroupDefRe.FindStringSubmatch(trimmed); m != nil {
			afi.groupType[m[1]] = m[2]
		} else if m := peerDescRe.FindStringSubmatch(trimmed); m != nil {
			afi.peerDesc[m[1]] = strings.TrimSpace(m[2])
		} else if m := peerBFDRe.FindStringSubmatch(trimmed); m != nil {
			afi.peerBFD[m[1]] = true
		}
	}

	return afi
}

// mergeMaps returns a new map with entries from base overridden by overlay.
func mergeMaps[V any](base, overlay map[string]V) map[string]V {
	result := make(map[string]V, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// ---------- VRF Instances ----------

var (
	vrfHeaderRe = regexp.MustCompile(`(?i)^ip\s+vpn-instance\s+(\S+)`)
	rdRe        = regexp.MustCompile(`(?i)^\s*route-distinguisher\s+(\S+)`)
	vpnTargetRe = regexp.MustCompile(`(?i)^\s*vpn-target\s+(.+)`)
	tnlPolicyRe = regexp.MustCompile(`(?i)^\s*tnl-policy\s+(\S+)`)
	labelModeRe = regexp.MustCompile(`(?i)^\s*label-mode\s+(\S+)`)
	vrfAFRe     = regexp.MustCompile(`(?i)^\s*(ipv4-family|ipv6-family)\s*$`)
)

// ExtractVRFInstancesHuawei extracts VPN instances from Huawei/H3C config.
func ExtractVRFInstancesHuawei(configText string) []model.VRFInstance {
	sections := strings.Split(configText, "\n#")
	var result []model.VRFInstance

	for _, sec := range sections {
		lines := strings.Split(strings.TrimSpace(sec), "\n")
		if len(lines) == 0 {
			continue
		}
		m := vrfHeaderRe.FindStringSubmatch(strings.TrimSpace(lines[0]))
		if m == nil {
			continue
		}
		vrfName := m[1]

		// Parse the VRF section — may contain ipv4-family/ipv6-family sub-blocks
		vrf := model.VRFInstance{
			VRFName: vrfName,
		}

		var importRTs, exportRTs []string

		for _, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if rdm := rdRe.FindStringSubmatch(line); rdm != nil {
				vrf.RD = rdm[1]
			} else if vtm := vpnTargetRe.FindStringSubmatch(line); vtm != nil {
				rest := strings.TrimSpace(vtm[1])
				imp, exp := parseVPNTarget(rest)
				importRTs = append(importRTs, imp...)
				exportRTs = append(exportRTs, exp...)
			} else if tm := tnlPolicyRe.FindStringSubmatch(line); tm != nil {
				vrf.TunnelPolicy = tm[1]
			} else if lm := labelModeRe.FindStringSubmatch(line); lm != nil {
				vrf.LabelMode = lm[1]
			} else if afm := vrfAFRe.FindStringSubmatch(trimmed); afm != nil {
				if vrf.AddressFamily == "" {
					vrf.AddressFamily = strings.ToLower(afm[1])
				}
			}
		}

		importJSON, _ := json.Marshal(dedupStrings(importRTs))
		exportJSON, _ := json.Marshal(dedupStrings(exportRTs))
		vrf.ImportRT = string(importJSON)
		vrf.ExportRT = string(exportJSON)

		result = append(result, vrf)
	}
	return result
}

// parseVPNTarget parses a vpn-target line's value portion.
// Format: "45090:1003 45090:1008 import-extcommunity" or "45090:1003 export-extcommunity"
func parseVPNTarget(s string) (importRTs, exportRTs []string) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return
	}

	// Last field is the direction
	dir := strings.ToLower(parts[len(parts)-1])
	rts := parts[:len(parts)-1]

	switch dir {
	case "import-extcommunity":
		importRTs = rts
	case "export-extcommunity":
		exportRTs = rts
	default:
		// If no direction keyword, treat all as both (shouldn't normally happen)
	}
	return
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var result []string
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

// ---------- Route Policies ----------

var (
	rpHeaderRe = regexp.MustCompile(`(?i)^route-policy\s+(\S+)\s+(permit|deny)\s+node\s+(\d+)`)
	ifMatchRe  = regexp.MustCompile(`(?i)^\s*if-match\s+(\S+)\s+(.+)`)
	applyRe    = regexp.MustCompile(`(?i)^\s*apply\s+(\S+)\s+(.+)`)
)

// ExtractRoutePoliciesHuawei extracts route-policies from Huawei/H3C config.
func ExtractRoutePoliciesHuawei(configText string) []model.RoutePolicy {
	sections := strings.Split(configText, "\n#")

	// Collect nodes grouped by policy name
	type nodeInfo struct {
		seq    int
		action string
		match  []map[string]string
		apply  []map[string]string
		raw    string
	}
	policyNodes := make(map[string][]nodeInfo)
	policyOrder := make([]string, 0) // preserve order

	for _, sec := range sections {
		lines := strings.Split(strings.TrimSpace(sec), "\n")
		if len(lines) == 0 {
			continue
		}
		m := rpHeaderRe.FindStringSubmatch(strings.TrimSpace(lines[0]))
		if m == nil {
			continue
		}
		name := m[1]
		action := strings.ToLower(m[2])
		seq, _ := strconv.Atoi(m[3])

		var matchClauses []map[string]string
		var applyClauses []map[string]string

		for _, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if im := ifMatchRe.FindStringSubmatch(line); im != nil {
				matchClauses = append(matchClauses, map[string]string{
					"type":  strings.TrimSpace(im[1]),
					"value": strings.TrimSpace(im[2]),
				})
			} else if am := applyRe.FindStringSubmatch(line); am != nil {
				applyClauses = append(applyClauses, map[string]string{
					"type":  strings.TrimSpace(am[1]),
					"value": strings.TrimSpace(am[2]),
				})
			}
		}

		// Build raw text for this node
		rawLines := []string{strings.TrimSpace(lines[0])}
		for _, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				rawLines = append(rawLines, " "+trimmed)
			}
		}

		if _, seen := policyNodes[name]; !seen {
			policyOrder = append(policyOrder, name)
		}

		policyNodes[name] = append(policyNodes[name], nodeInfo{
			seq:    seq,
			action: action,
			match:  matchClauses,
			apply:  applyClauses,
			raw:    strings.Join(rawLines, "\n"),
		})
	}

	var result []model.RoutePolicy
	for _, name := range policyOrder {
		nodes := policyNodes[name]
		var rpNodes []model.RoutePolicyNode
		var rawParts []string

		for _, n := range nodes {
			matchJSON, _ := json.Marshal(n.match)
			if n.match == nil {
				matchJSON = []byte("[]")
			}
			applyJSON, _ := json.Marshal(n.apply)
			if n.apply == nil {
				applyJSON = []byte("[]")
			}

			rpNodes = append(rpNodes, model.RoutePolicyNode{
				Sequence:     n.seq,
				Action:       n.action,
				MatchClauses: string(matchJSON),
				ApplyClauses: string(applyJSON),
			})
			rawParts = append(rawParts, n.raw)
		}

		result = append(result, model.RoutePolicy{
			PolicyName: name,
			VendorType: "route-policy",
			RawText:    strings.Join(rawParts, "\n#\n"),
			Nodes:      rpNodes,
		})
	}
	return result
}
