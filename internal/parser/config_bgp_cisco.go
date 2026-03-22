package parser

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// ---------- Cisco IOS-XR BGP extraction ----------

// Cisco IOS-XR configs can be in two formats:
// 1. Indented (from "show run"): `router bgp 45090\n neighbor X\n  remote-as Y`
// 2. Flat (from config file export): `router bgp 45090\nneighbor X\nremote-as Y\n!\n!`
//    In flat format, `!` acts as a closing brace for the current nesting level.
//
// The strategy: detect flat format and normalize to indented format before parsing.

var (
	ciscoBGPBlockRe    = regexp.MustCompile(`(?m)^router bgp\s+(\d+)`)
	ciscoNeighGroupRe  = regexp.MustCompile(`(?m)^\s+neighbor-group\s+(\S+)`)
	ciscoNeighborRe    = regexp.MustCompile(`(?m)^\s+neighbor\s+(\S+)`)
	ciscoRemoteASRe    = regexp.MustCompile(`(?i)^\s*remote-as\s+(\d+)`)
	ciscoUpdateSrcRe   = regexp.MustCompile(`(?i)^\s*update-source\s+(\S+)`)
	ciscoUseGroupRe    = regexp.MustCompile(`(?i)^\s*use\s+neighbor-group\s+(\S+)`)
	ciscoDescriptionRe = regexp.MustCompile(`(?i)^\s*description\s+(.+)`)
	ciscoAFRe          = regexp.MustCompile(`(?i)^\s*address-family\s+(.+)`)
	ciscoRPInRe        = regexp.MustCompile(`(?i)^\s*route-policy\s+(\S+)\s+in`)
	ciscoRPOutRe       = regexp.MustCompile(`(?i)^\s*route-policy\s+(\S+)\s+out`)
	ciscoSoftRecfgRe   = regexp.MustCompile(`(?i)^\s*soft-reconfiguration\s+inbound`)
	ciscoVRFSubRe      = regexp.MustCompile(`(?m)^\s+vrf\s+(\S+)`)
	ciscoRDRe          = regexp.MustCompile(`(?i)^\s*rd\s+(\S+)`)
	ciscoEBGPMultiRe   = regexp.MustCompile(`(?i)^\s*ebgp-multihop\s+(\d+)`)
	ciscoBFDRe         = regexp.MustCompile(`(?i)^\s*bfd\s+minimum-interval`)
	ciscoShutdownRe    = regexp.MustCompile(`(?i)^\s*shutdown`)
	ciscoNextHopSelfRe = regexp.MustCompile(`(?i)^\s*next-hop-self`)
)

// indentLevel returns the number of leading spaces of a line.
func indentLevel(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// ciscoContextOpenerRe matches lines that open a new nesting level in IOS-XR.
var ciscoContextOpenerRe = regexp.MustCompile(`(?i)^(neighbor-group|neighbor|vrf|address-family|import\s+route-target|export\s+route-target|bmp\s+server)\s`)

// isCiscoFlat checks if the BGP block uses flat format (no indentation).
func isCiscoFlat(lines []string) bool {
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || trimmed == "!" {
			continue
		}
		if indentLevel(l) > 0 {
			return false
		}
	}
	return true
}

// normalizeCiscoFlat converts a flat IOS-XR BGP block to indented form.
// Each context opener increases the indent for subsequent lines.
// Each `!` decreases the indent (closes current block).
func normalizeCiscoFlat(flatLines []string) []string {
	depth := 1 // start at depth 1 (inside router bgp)
	var result []string
	for _, line := range flatLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == "!" {
			if depth > 1 {
				result = append(result, strings.Repeat(" ", depth)+"!")
				depth--
			} else {
				result = append(result, " !")
			}
			continue
		}
		// Add line at current depth
		result = append(result, strings.Repeat(" ", depth)+trimmed)
		// Check if this line opens a new context
		if ciscoContextOpenerRe.MatchString(trimmed) {
			depth++
		}
	}
	return result
}

// extractBGPBlock returns the lines inside `router bgp <AS>` and the AS number.
// Handles both indented and flat IOS-XR config formats.
func extractBGPBlock(configText string) (lines []string, localAS int) {
	allLines := strings.Split(configText, "\n")
	start := -1
	for i, line := range allLines {
		trimmed := strings.TrimRight(line, "\r \t")
		if ciscoBGPBlockRe.MatchString(trimmed) {
			m := ciscoBGPBlockRe.FindStringSubmatch(trimmed)
			localAS, _ = strconv.Atoi(m[1])
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, 0
	}

	// Collect raw lines after `router bgp`. For indented configs, stop at
	// a non-indented non-! line. For flat configs, we need to track ! depth.
	var rawLines []string
	for i := start; i < len(allLines); i++ {
		trimmed := strings.TrimRight(allLines[i], "\r \t")
		rawLines = append(rawLines, allLines[i])
		_ = trimmed
	}

	if isCiscoFlat(rawLines) {
		// Flat format: find the end of the BGP block by tracking ! depth.
		// depth starts at 1 (inside router bgp); each context opener increases,
		// each ! decreases. When depth reaches 0, block is done.
		var bgpRaw []string
		depth := 1
		for _, line := range rawLines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if trimmed == "!" {
				depth--
				if depth <= 0 {
					break
				}
				bgpRaw = append(bgpRaw, line)
				continue
			}
			bgpRaw = append(bgpRaw, line)
			if ciscoContextOpenerRe.MatchString(trimmed) {
				depth++
			}
		}
		lines = normalizeCiscoFlat(bgpRaw)
	} else {
		// Indented format: stop at top-level `!`
		lines = nil
		for _, line := range rawLines {
			trimmed := strings.TrimRight(line, "\r \t")
			if trimmed == "!" && indentLevel(line) == 0 {
				break
			}
			lines = append(lines, line)
		}
	}
	return lines, localAS
}

// ciscoNeighborGroup stores parsed neighbor-group attributes.
type ciscoNeighborGroup struct {
	name         string
	remoteAS     int
	updateSource string
	ebgpMultihop int
	bfdEnabled   bool
	afPolicies   map[string]ciscoAFPolicy // key = AF name
}

type ciscoAFPolicy struct {
	importPolicy string
	exportPolicy string
	softReconfig bool
	nextHopSelf  bool
}

// collectSubBlock collects indented lines starting at lines[start] that are
// deeper than baseIndent. Returns the sub-block lines and the index after the block.
func collectSubBlock(lines []string, start, baseIndent int) ([]string, int) {
	var sub []string
	i := start
	for i < len(lines) {
		trimmed := strings.TrimRight(lines[i], "\r \t")
		if trimmed == "" {
			i++
			continue
		}
		ind := indentLevel(lines[i])
		// "!" at baseIndent or less ends the sub-block
		if strings.TrimSpace(trimmed) == "!" && ind <= baseIndent {
			i++
			break
		}
		if ind <= baseIndent {
			break
		}
		sub = append(sub, lines[i])
		i++
	}
	return sub, i
}

// parseAFPolicies parses address-family sub-blocks within a neighbor/group block.
func parseAFPolicies(blockLines []string, baseIndent int) map[string]ciscoAFPolicy {
	result := make(map[string]ciscoAFPolicy)
	for i := 0; i < len(blockLines); i++ {
		line := blockLines[i]
		if m := ciscoAFRe.FindStringSubmatch(strings.TrimRight(line, "\r \t")); m != nil {
			afName := strings.TrimSpace(m[1])
			afIndent := indentLevel(line)
			pol := ciscoAFPolicy{}
			// Scan lines inside this AF
			for j := i + 1; j < len(blockLines); j++ {
				afLine := blockLines[j]
				afTrimmed := strings.TrimRight(afLine, "\r \t")
				if strings.TrimSpace(afTrimmed) == "!" {
					i = j
					break
				}
				afLineIndent := indentLevel(afLine)
				if afLineIndent <= afIndent {
					i = j - 1
					break
				}
				if rm := ciscoRPInRe.FindStringSubmatch(afTrimmed); rm != nil {
					pol.importPolicy = rm[1]
				}
				if rm := ciscoRPOutRe.FindStringSubmatch(afTrimmed); rm != nil {
					pol.exportPolicy = rm[1]
				}
				if ciscoSoftRecfgRe.MatchString(afTrimmed) {
					pol.softReconfig = true
				}
				if ciscoNextHopSelfRe.MatchString(afTrimmed) {
					pol.nextHopSelf = true
				}
				if j == len(blockLines)-1 {
					i = j
				}
			}
			result[afName] = pol
		}
	}
	return result
}

// ExtractBGPPeersCisco parses Cisco IOS-XR running-config and returns BGP peers.
func ExtractBGPPeersCisco(configText string) []model.BGPPeer {
	bgpLines, localAS := extractBGPBlock(configText)
	if bgpLines == nil {
		return nil
	}

	// Phase 1: Parse neighbor-groups
	groups := make(map[string]*ciscoNeighborGroup)
	// Phase 2: Parse global neighbors
	type rawNeighbor struct {
		ip           string
		vrf          string
		remoteAS     int
		useGroup     string
		description  string
		updateSource string
		ebgpMultihop int
		bfdEnabled   bool
		shutdown     bool
		afPolicies   map[string]ciscoAFPolicy
	}
	var neighbors []rawNeighbor

	// Phase 3: Parse VRF sub-blocks (neighbors inside VRF)
	// We'll iterate through bgpLines tracking context

	// First pass: identify top-level sections by indent
	bgpIndent := indentLevel(bgpLines[0]) // typically 1
	if len(bgpLines) == 0 {
		return nil
	}
	// Find the base indent (the indent of direct children of router bgp)
	baseIndent := 0
	for _, l := range bgpLines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && trimmed != "!" {
			baseIndent = indentLevel(l)
			break
		}
	}
	_ = bgpIndent

	i := 0
	for i < len(bgpLines) {
		line := bgpLines[i]
		trimmed := strings.TrimRight(line, "\r \t")
		trimmedClean := strings.TrimSpace(trimmed)

		// Skip empty/delimiter lines
		if trimmedClean == "" || trimmedClean == "!" {
			i++
			continue
		}

		lineIndent := indentLevel(line)

		// neighbor-group at base indent
		if m := ciscoNeighGroupRe.FindStringSubmatch(trimmed); m != nil && lineIndent == baseIndent {
			gName := m[1]
			grp := &ciscoNeighborGroup{name: gName, afPolicies: make(map[string]ciscoAFPolicy)}
			subLines, nextI := collectSubBlock(bgpLines, i+1, lineIndent)
			for _, sl := range subLines {
				st := strings.TrimRight(sl, "\r \t")
				if rm := ciscoRemoteASRe.FindStringSubmatch(st); rm != nil {
					grp.remoteAS, _ = strconv.Atoi(rm[1])
				}
				if rm := ciscoUpdateSrcRe.FindStringSubmatch(st); rm != nil {
					grp.updateSource = rm[1]
				}
				if rm := ciscoEBGPMultiRe.FindStringSubmatch(st); rm != nil {
					grp.ebgpMultihop, _ = strconv.Atoi(rm[1])
				}
				if ciscoBFDRe.MatchString(st) {
					grp.bfdEnabled = true
				}
			}
			grp.afPolicies = parseAFPolicies(subLines, lineIndent)
			groups[gName] = grp
			i = nextI
			continue
		}

		// neighbor at base indent (global scope)
		if m := ciscoNeighborRe.FindStringSubmatch(trimmed); m != nil && lineIndent == baseIndent {
			n := rawNeighbor{ip: m[1], afPolicies: make(map[string]ciscoAFPolicy)}
			subLines, nextI := collectSubBlock(bgpLines, i+1, lineIndent)
			for _, sl := range subLines {
				st := strings.TrimRight(sl, "\r \t")
				if rm := ciscoUseGroupRe.FindStringSubmatch(st); rm != nil {
					n.useGroup = rm[1]
				}
				if rm := ciscoDescriptionRe.FindStringSubmatch(st); rm != nil {
					n.description = strings.TrimSpace(rm[1])
				}
				if rm := ciscoRemoteASRe.FindStringSubmatch(st); rm != nil {
					n.remoteAS, _ = strconv.Atoi(rm[1])
				}
				if rm := ciscoUpdateSrcRe.FindStringSubmatch(st); rm != nil {
					n.updateSource = rm[1]
				}
				if rm := ciscoEBGPMultiRe.FindStringSubmatch(st); rm != nil {
					n.ebgpMultihop, _ = strconv.Atoi(rm[1])
				}
				if ciscoBFDRe.MatchString(st) {
					n.bfdEnabled = true
				}
				if ciscoShutdownRe.MatchString(st) {
					n.shutdown = true
				}
			}
			n.afPolicies = parseAFPolicies(subLines, lineIndent)
			neighbors = append(neighbors, n)
			i = nextI
			continue
		}

		// vrf sub-block at base indent
		if m := ciscoVRFSubRe.FindStringSubmatch(trimmed); m != nil && lineIndent == baseIndent {
			vrfName := m[1]
			subLines, nextI := collectSubBlock(bgpLines, i+1, lineIndent)
			// Parse neighbors within VRF sub-block
			vrfBaseIndent := 0
			for _, sl := range subLines {
				st := strings.TrimSpace(sl)
				if st != "" && st != "!" {
					vrfBaseIndent = indentLevel(sl)
					break
				}
			}
			for j := 0; j < len(subLines); j++ {
				sl := subLines[j]
				st := strings.TrimRight(sl, "\r \t")
				slIndent := indentLevel(sl)
				if nm := ciscoNeighborRe.FindStringSubmatch(st); nm != nil && slIndent == vrfBaseIndent {
					n := rawNeighbor{ip: nm[1], vrf: vrfName, afPolicies: make(map[string]ciscoAFPolicy)}
					nSub, nNextJ := collectSubBlock(subLines, j+1, slIndent)
					for _, nsl := range nSub {
						nst := strings.TrimRight(nsl, "\r \t")
						if rm := ciscoRemoteASRe.FindStringSubmatch(nst); rm != nil {
							n.remoteAS, _ = strconv.Atoi(rm[1])
						}
						if rm := ciscoDescriptionRe.FindStringSubmatch(nst); rm != nil {
							n.description = strings.TrimSpace(rm[1])
						}
						if rm := ciscoUpdateSrcRe.FindStringSubmatch(nst); rm != nil {
							n.updateSource = rm[1]
						}
						if rm := ciscoUseGroupRe.FindStringSubmatch(nst); rm != nil {
							n.useGroup = rm[1]
						}
						if rm := ciscoEBGPMultiRe.FindStringSubmatch(nst); rm != nil {
							n.ebgpMultihop, _ = strconv.Atoi(rm[1])
						}
						if ciscoBFDRe.MatchString(nst) {
							n.bfdEnabled = true
						}
						if ciscoShutdownRe.MatchString(nst) {
							n.shutdown = true
						}
					}
					n.afPolicies = parseAFPolicies(nSub, slIndent)
					neighbors = append(neighbors, n)
					j = nNextJ - 1
					continue
				}
			}
			i = nextI
			continue
		}

		i++
	}

	// Phase 4: Resolve group inheritance and emit BGPPeer structs
	var result []model.BGPPeer
	for _, n := range neighbors {
		// Merge group defaults
		grp := groups[n.useGroup]

		remoteAS := n.remoteAS
		updateSrc := n.updateSource
		ebgpMH := n.ebgpMultihop
		bfd := n.bfdEnabled
		if grp != nil {
			if remoteAS == 0 {
				remoteAS = grp.remoteAS
			}
			if updateSrc == "" {
				updateSrc = grp.updateSource
			}
			if ebgpMH == 0 {
				ebgpMH = grp.ebgpMultihop
			}
			if !bfd {
				bfd = grp.bfdEnabled
			}
		}

		// Collect all AFs: group AFs merged with neighbor overrides
		allAFs := make(map[string]ciscoAFPolicy)
		if grp != nil {
			for af, pol := range grp.afPolicies {
				allAFs[af] = pol
			}
		}
		for af, pol := range n.afPolicies {
			merged := allAFs[af]
			if pol.importPolicy != "" {
				merged.importPolicy = pol.importPolicy
			}
			if pol.exportPolicy != "" {
				merged.exportPolicy = pol.exportPolicy
			}
			if pol.softReconfig {
				merged.softReconfig = true
			}
			if pol.nextHopSelf {
				merged.nextHopSelf = true
			}
			allAFs[af] = merged
		}

		// If no AFs found, emit one peer with empty AF
		if len(allAFs) == 0 {
			allAFs[""] = ciscoAFPolicy{}
		}

		for af, pol := range allAFs {
			peer := model.BGPPeer{
				LocalAS:        localAS,
				PeerIP:         n.ip,
				RemoteAS:       remoteAS,
				PeerGroup:      n.useGroup,
				Description:    n.description,
				UpdateSource:   updateSrc,
				EBGPMultihop:   ebgpMH,
				AddressFamily:  af,
				ImportPolicy:   pol.importPolicy,
				ExportPolicy:   pol.exportPolicy,
				VRF:            n.vrf,
				Enabled:        1,
			}
			if bfd {
				peer.BFDEnabled = 1
			}
			if pol.softReconfig {
				peer.SoftReconfig = 1
			}
			if pol.nextHopSelf {
				peer.NextHopSelf = 1
			}
			if n.shutdown {
				peer.Shutdown = 1
				peer.Enabled = 0
			}
			result = append(result, peer)
		}
	}

	return result
}

// ---------- Cisco IOS-XR VRF extraction ----------

var (
	ciscoVRFTopRe      = regexp.MustCompile(`(?m)^vrf\s+(\S+)`)
	ciscoImportRTRe    = regexp.MustCompile(`(?i)^\s*import\s+route-target`)
	ciscoExportRTRe    = regexp.MustCompile(`(?i)^\s*export\s+route-target`)
	ciscoImportPolRe   = regexp.MustCompile(`(?i)^\s*import\s+route-policy\s+(\S+)`)
	ciscoExportPolRe   = regexp.MustCompile(`(?i)^\s*export\s+route-policy\s+(\S+)`)
	ciscoRTValueRe     = regexp.MustCompile(`^\s+(\d+:\d+)\s*$`)
)

// ExtractVRFInstancesCisco extracts VRF definitions from Cisco IOS-XR config.
// It handles both standalone `vrf <name>` blocks and `vrf <name>` sub-blocks
// inside `router bgp`.
func ExtractVRFInstancesCisco(configText string) []model.VRFInstance {
	var result []model.VRFInstance
	allLines := strings.Split(configText, "\n")

	// Also parse VRF sub-blocks inside router bgp for RD
	bgpVRFData := make(map[string]*vrfData) // vrf name -> data from bgp block
	bgpLines, _ := extractBGPBlock(configText)
	if bgpLines != nil {
		baseIndent := 0
		for _, l := range bgpLines {
			t := strings.TrimSpace(l)
			if t != "" && t != "!" {
				baseIndent = indentLevel(l)
				break
			}
		}
		for i := 0; i < len(bgpLines); i++ {
			line := bgpLines[i]
			trimmed := strings.TrimRight(line, "\r \t")
			if m := ciscoVRFSubRe.FindStringSubmatch(trimmed); m != nil && indentLevel(line) == baseIndent {
				vrfName := m[1]
				subLines, nextI := collectSubBlock(bgpLines, i+1, baseIndent)
				vd := parseVRFSubLines(subLines)
				vd.name = vrfName
				bgpVRFData[vrfName] = vd
				i = nextI - 1
			}
		}
	}

	// Parse standalone vrf blocks
	for i := 0; i < len(allLines); i++ {
		line := strings.TrimRight(allLines[i], "\r \t")
		trimmedClean := strings.TrimSpace(line)

		// Match top-level "vrf <name>" (indent 0)
		if m := ciscoVRFTopRe.FindStringSubmatch(trimmedClean); m != nil && indentLevel(allLines[i]) == 0 {
			// Skip if this is inside router bgp (check previous non-empty line)
			vrfName := m[1]
			subLines, nextI := collectSubBlock(allLines, i+1, 0)
			vd := parseVRFSubLines(subLines)
			vd.name = vrfName

			// Merge BGP VRF data (RD comes from router bgp block)
			if bv, ok := bgpVRFData[vrfName]; ok {
				if vd.rd == "" {
					vd.rd = bv.rd
				}
				// Merge RTs
				if len(vd.importRTs) == 0 {
					vd.importRTs = bv.importRTs
				}
				if len(vd.exportRTs) == 0 {
					vd.exportRTs = bv.exportRTs
				}
				if vd.importPolicy == "" {
					vd.importPolicy = bv.importPolicy
				}
				if vd.exportPolicy == "" {
					vd.exportPolicy = bv.exportPolicy
				}
				if vd.af == "" {
					vd.af = bv.af
				}
				delete(bgpVRFData, vrfName)
			}

			result = append(result, vd.toModel())
			i = nextI - 1
			continue
		}
	}

	// Emit any BGP-only VRFs not seen as standalone
	for _, vd := range bgpVRFData {
		result = append(result, vd.toModel())
	}

	return result
}

type vrfData struct {
	name         string
	rd           string
	importRTs    []string
	exportRTs    []string
	importPolicy string
	exportPolicy string
	af           string
}

func (v *vrfData) toModel() model.VRFInstance {
	importJSON, _ := json.Marshal(v.importRTs)
	exportJSON, _ := json.Marshal(v.exportRTs)
	if len(v.importRTs) == 0 {
		importJSON = []byte("[]")
	}
	if len(v.exportRTs) == 0 {
		exportJSON = []byte("[]")
	}
	return model.VRFInstance{
		VRFName:       v.name,
		RD:            v.rd,
		ImportRT:      string(importJSON),
		ExportRT:      string(exportJSON),
		ImportPolicy:  v.importPolicy,
		ExportPolicy:  v.exportPolicy,
		AddressFamily: v.af,
	}
}

func parseVRFSubLines(lines []string) *vrfData {
	vd := &vrfData{}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r \t")
		trimmed := strings.TrimSpace(line)

		if rm := ciscoRDRe.FindStringSubmatch(trimmed); rm != nil {
			vd.rd = rm[1]
		}
		if rm := ciscoImportPolRe.FindStringSubmatch(trimmed); rm != nil {
			vd.importPolicy = rm[1]
		}
		if rm := ciscoExportPolRe.FindStringSubmatch(trimmed); rm != nil {
			vd.exportPolicy = rm[1]
		}
		if m := ciscoAFRe.FindStringSubmatch(trimmed); m != nil {
			vd.af = strings.TrimSpace(m[1])
		}

		// import route-target block
		if ciscoImportRTRe.MatchString(trimmed) {
			for j := i + 1; j < len(lines); j++ {
				rtLine := strings.TrimSpace(strings.TrimRight(lines[j], "\r \t"))
				if rtLine == "!" {
					i = j
					break
				}
				if rm := ciscoRTValueRe.FindStringSubmatch(lines[j]); rm != nil {
					vd.importRTs = append(vd.importRTs, rm[1])
				}
				if j == len(lines)-1 {
					i = j
				}
			}
		}

		// export route-target block
		if ciscoExportRTRe.MatchString(trimmed) {
			for j := i + 1; j < len(lines); j++ {
				rtLine := strings.TrimSpace(strings.TrimRight(lines[j], "\r \t"))
				if rtLine == "!" {
					i = j
					break
				}
				if rm := ciscoRTValueRe.FindStringSubmatch(lines[j]); rm != nil {
					vd.exportRTs = append(vd.exportRTs, rm[1])
				}
				if j == len(lines)-1 {
					i = j
				}
			}
		}
	}
	return vd
}

// ---------- Cisco IOS-XR Route-Policy extraction ----------

var (
	ciscoRoutePolicyStartRe = regexp.MustCompile(`(?m)^route-policy\s+(\S+)`)
	ciscoEndPolicyRe        = regexp.MustCompile(`(?m)^end-policy`)
)

// ExtractRoutePoliciesCisco extracts route-policy definitions from Cisco IOS-XR config.
func ExtractRoutePoliciesCisco(configText string) []model.RoutePolicy {
	var result []model.RoutePolicy
	allLines := strings.Split(configText, "\n")

	for i := 0; i < len(allLines); i++ {
		line := strings.TrimRight(allLines[i], "\r \t")
		if m := ciscoRoutePolicyStartRe.FindStringSubmatch(line); m != nil {
			policyName := m[1]
			var bodyLines []string
			bodyLines = append(bodyLines, line)
			j := i + 1
			for ; j < len(allLines); j++ {
				bodyLine := strings.TrimRight(allLines[j], "\r \t")
				bodyLines = append(bodyLines, bodyLine)
				if ciscoEndPolicyRe.MatchString(bodyLine) {
					break
				}
			}
			rawText := strings.Join(bodyLines, "\n")

			// Create a single node capturing the policy body
			node := model.RoutePolicyNode{
				Sequence:     0,
				MatchClauses: "[]",
				ApplyClauses: "[]",
			}

			// Extract apply statements
			var applies []string
			applyRe := regexp.MustCompile(`(?i)^\s*apply\s+(\S+)`)
			for _, bl := range bodyLines {
				if am := applyRe.FindStringSubmatch(bl); am != nil {
					applies = append(applies, am[1])
				}
			}
			if len(applies) > 0 {
				applyJSON, _ := json.Marshal(applies)
				node.ApplyClauses = string(applyJSON)
			}

			// Determine overall action: pass, drop, or mixed
			bodyText := strings.Join(bodyLines[1:], "\n") // exclude header
			node.Action = inferCiscoPolicyAction(bodyText)

			rp := model.RoutePolicy{
				PolicyName: policyName,
				VendorType: "route-policy",
				RawText:    rawText,
				Nodes:      []model.RoutePolicyNode{node},
			}
			result = append(result, rp)
			i = j
		}
	}
	return result
}

// inferCiscoPolicyAction guesses the overall action from the policy body.
func inferCiscoPolicyAction(body string) string {
	hasPass := strings.Contains(body, "pass")
	hasDrop := strings.Contains(body, "drop")
	if hasPass && hasDrop {
		return "mixed"
	}
	if hasDrop {
		return "deny"
	}
	if hasPass {
		return "permit"
	}
	// If there are set statements, it's likely permit
	if strings.Contains(body, "set ") {
		return "permit"
	}
	return ""
}
