package parser

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// ExtractInterfacesFromConfig parses running-config text to extract interface
// definitions. Supports Huawei VRP, H3C Comware, Cisco IOS-XR, and Juniper JUNOS.
func ExtractInterfacesFromConfig(configText, vendor string) []model.Interface {
	switch vendor {
	case "huawei", "h3c":
		return extractHashDelimited(configText, vendor)
	case "cisco":
		return extractCiscoConfig(configText)
	case "juniper":
		return extractJuniperConfig(configText)
	default:
		return nil
	}
}

// ---------- Huawei / H3C (# delimited) ----------

var (
	ifaceLineRe = regexp.MustCompile(`(?i)^interface\s+(\S+)`)
	ipAddrRe    = regexp.MustCompile(`(?i)^\s*ip\s+address\s+(\d+\.\d+\.\d+\.\d+)\s+(\d+\.\d+\.\d+\.\d+)`)
	descRe      = regexp.MustCompile(`(?i)^\s*description\s+(.+)`)
)

func extractHashDelimited(configText, vendor string) []model.Interface {
	sections := strings.Split(configText, "\n#")
	var result []model.Interface

	for _, sec := range sections {
		lines := strings.Split(strings.TrimSpace(sec), "\n")
		if len(lines) == 0 {
			continue
		}
		m := ifaceLineRe.FindStringSubmatch(strings.TrimSpace(lines[0]))
		if m == nil {
			continue
		}
		name := m[1]
		iface := model.Interface{
			Name:   name,
			Type:   inferInterfaceTypeByVendor(name, vendor),
			Status: "up", // default; "shutdown" flips to down
		}
		for _, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if am := ipAddrRe.FindStringSubmatch(line); am != nil {
				iface.IPAddress = am[1]
				iface.Mask = am[2]
			} else if dm := descRe.FindStringSubmatch(line); dm != nil {
				iface.Description = strings.TrimSpace(dm[1])
			} else if strings.EqualFold(trimmed, "shutdown") {
				iface.Status = "down"
			} else if strings.EqualFold(trimmed, "undo shutdown") {
				iface.Status = "up"
			}
		}
		result = append(result, iface)
	}
	return result
}

// ---------- Cisco IOS-XR (! delimited, ipv4 address) ----------

var (
	ciscoIfaceRe = regexp.MustCompile(`(?i)^interface\s+(\S+)`)
	ciscoIPRe    = regexp.MustCompile(`(?i)^\s*ipv4\s+address\s+(\d+\.\d+\.\d+\.\d+)\s+(\d+\.\d+\.\d+\.\d+)`)
)

func extractCiscoConfig(configText string) []model.Interface {
	var result []model.Interface
	var current *model.Interface

	flushCurrent := func() {
		if current != nil {
			result = append(result, *current)
			current = nil
		}
	}

	for _, line := range strings.Split(configText, "\n") {
		trimmed := strings.TrimRight(line, "\r \t")

		if m := ciscoIfaceRe.FindStringSubmatch(trimmed); m != nil {
			flushCurrent()
			name := m[1]
			current = &model.Interface{
				Name:   name,
				Type:   inferInterfaceTypeByVendor(name, "cisco"),
				Status: "up",
			}
			continue
		}

		if trimmed == "!" {
			flushCurrent()
			continue
		}

		if current == nil {
			continue
		}

		innerTrimmed := strings.TrimSpace(trimmed)
		if am := ciscoIPRe.FindStringSubmatch(trimmed); am != nil {
			current.IPAddress = am[1]
			current.Mask = am[2]
		} else if dm := descRe.FindStringSubmatch(trimmed); dm != nil {
			current.Description = strings.TrimSpace(dm[1])
		} else if strings.EqualFold(innerTrimmed, "shutdown") {
			current.Status = "down"
		}
	}
	flushCurrent()
	return result
}

// ---------- Juniper JUNOS (hierarchical { }) ----------

var (
	juniperIfBlockRe = regexp.MustCompile(`(?i)^interfaces\s*\{`)
	juniperAddrRe    = regexp.MustCompile(`address\s+(\d+\.\d+\.\d+\.\d+)/(\d+);`)
	juniperDescRe    = regexp.MustCompile(`description\s+"([^"]+)"`)
)

func extractJuniperConfig(configText string) []model.Interface {
	var result []model.Interface

	// There may be multiple "interfaces {" blocks (e.g. in different
	// routing-engine groups).  Collect all of them.
	lines := strings.Split(configText, "\n")
	for startIdx := 0; startIdx < len(lines); startIdx++ {
		if !juniperIfBlockRe.MatchString(strings.TrimSpace(lines[startIdx])) {
			continue
		}
		// Find closing brace for this interfaces block.
		depth := 0
		endIdx := startIdx
		for i := startIdx; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
			if depth <= 0 {
				endIdx = i
				break
			}
		}

		blockContent := strings.Join(lines[startIdx+1:endIdx], "\n")
		ifBlocks := extractAllTopLevelBlocks(blockContent)
		for _, ifb := range ifBlocks {
			ifaces := parseJuniperInterfaceBlock(ifb.Name, ifb.Content)
			result = append(result, ifaces...)
		}
		startIdx = endIdx
	}

	return result
}

// parseJuniperInterfaceBlock parses a single interface definition block and
// returns one or more Interface entries (one per unit with an address, or one
// for the interface itself if no units have addresses).
func parseJuniperInterfaceBlock(ifName, content string) []model.Interface {
	// Check for interface-level description.
	ifDesc := ""
	if dm := juniperDescRe.FindStringSubmatch(content); dm != nil {
		ifDesc = dm[1]
	}

	// Check for interface-level disable.
	ifDisabled := false
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "disable;" {
			ifDisabled = true
			break
		}
	}

	// Parse unit sub-blocks.
	units := extractNamedBlocks(content, "unit")
	var result []model.Interface

	for _, u := range units {
		desc := ifDesc
		if dm := juniperDescRe.FindStringSubmatch(u.Content); dm != nil {
			desc = dm[1]
		}

		ip := ""
		mask := ""
		if am := juniperAddrRe.FindStringSubmatch(u.Content); am != nil {
			ip = am[1]
			mask = am[2]
		}

		unitName := u.Name
		fullName := ifName + "." + unitName

		status := "up"
		if ifDisabled {
			status = "down"
		}
		// Check unit-level disable
		for _, line := range strings.Split(u.Content, "\n") {
			if strings.TrimSpace(line) == "disable;" {
				status = "down"
				break
			}
		}

		iface := model.Interface{
			Name:        fullName,
			Type:        inferInterfaceTypeByVendor(ifName, "juniper"),
			Status:      status,
			Description: desc,
			IPAddress:   ip,
			Mask:        mask,
		}
		result = append(result, iface)
	}

	// If no units found, emit the interface itself.
	if len(units) == 0 {
		status := "up"
		if ifDisabled {
			status = "down"
		}
		ip := ""
		mask := ""
		if am := juniperAddrRe.FindStringSubmatch(content); am != nil {
			ip = am[1]
			mask = am[2]
		}
		result = append(result, model.Interface{
			Name:        ifName,
			Type:        inferInterfaceTypeByVendor(ifName, "juniper"),
			Status:      status,
			Description: ifDesc,
			IPAddress:   ip,
			Mask:        mask,
		})
	}

	return result
}

// ---------- Unified interface type inference ----------

func inferInterfaceTypeByVendor(name, vendor string) model.InterfaceType {
	lower := strings.ToLower(name)
	switch vendor {
	case "huawei":
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
	case "h3c":
		switch {
		case strings.HasPrefix(lower, "loop"):
			return model.IfTypeLoopback
		case strings.HasPrefix(lower, "vlan"):
			return model.IfTypeVlanif
		case strings.HasPrefix(lower, "bridge-aggregation"), strings.HasPrefix(lower, "bagg"):
			return model.IfTypeEthTrunk
		case strings.HasPrefix(lower, "tunnel"):
			return model.IfTypeTunnelGRE
		case strings.HasPrefix(lower, "null"):
			return model.IfTypeNull
		case strings.Contains(lower, "."):
			return model.IfTypeSubInterface
		default:
			return model.IfTypePhysical
		}
	case "cisco":
		switch {
		case strings.HasPrefix(lower, "loopback"):
			return model.IfTypeLoopback
		case strings.HasPrefix(lower, "vlan"):
			return model.IfTypeVlanif
		case strings.HasPrefix(lower, "port-channel"), strings.HasPrefix(lower, "bundle-ether"):
			return model.IfTypeEthTrunk
		case strings.HasPrefix(lower, "tunnel"):
			return model.IfTypeTunnelGRE
		case strings.HasPrefix(lower, "null"):
			return model.IfTypeNull
		case strings.Contains(lower, "."):
			return model.IfTypeSubInterface
		default:
			return model.IfTypePhysical
		}
	case "juniper":
		switch {
		case strings.HasPrefix(lower, "lo"):
			return model.IfTypeLoopback
		case strings.HasPrefix(lower, "ae"):
			return model.IfTypeEthTrunk
		case strings.HasPrefix(lower, "irb"), strings.HasPrefix(lower, "vlan"):
			return model.IfTypeVlanif
		case strings.HasPrefix(lower, "gr-"), strings.HasPrefix(lower, "ip-"):
			return model.IfTypeTunnelGRE
		case strings.Contains(lower, "."):
			return model.IfTypeSubInterface
		default:
			return model.IfTypePhysical
		}
	default:
		return model.IfTypePhysical
	}
}

// cidrToMask converts a CIDR prefix length to a dotted-decimal subnet mask.
// Exported for potential reuse. Not currently called but available if we want
// to normalise Juniper masks.
func cidrToMask(prefixLen int) string {
	if prefixLen < 0 || prefixLen > 32 {
		return ""
	}
	var mask uint32
	if prefixLen > 0 {
		mask = uint32(math.MaxUint32) << (32 - prefixLen)
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		(mask>>24)&0xff, (mask>>16)&0xff, (mask>>8)&0xff, mask&0xff)
}
