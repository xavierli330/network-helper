// internal/parser/cisco/show_ip_route.go
package cisco

import (
	"regexp"
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// Cisco route line patterns:
// C        10.1.1.0/24 is directly connected, GigabitEthernet0/0
// O        172.16.0.0/24 [110/2] via 10.0.0.2, 00:05:30, GigabitEthernet0/1
// S*    0.0.0.0/0 [1/0] via 10.0.0.1
var routeLineRe = regexp.MustCompile(`^\s*([A-Za-z*]+\*?)\s+(\d+\.\d+\.\d+\.\d+/\d+)`)

func ParseShowIPRoute(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdRIB, RawText: raw}
	lines := strings.Split(raw, "\n")

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		m := routeLineRe.FindStringSubmatch(trimmed)
		if m == nil { continue }

		code := strings.TrimSpace(m[1])
		prefixStr := m[2]

		parts := strings.SplitN(prefixStr, "/", 2)
		if len(parts) != 2 { continue }
		prefix := parts[0]
		maskLen, _ := strconv.Atoi(parts[1])

		proto := ciscoCodeToProtocol(code)
		var pref, metric int
		var nextHop, outIface string

		// Extract [pref/metric]
		if idx := strings.Index(trimmed, "["); idx >= 0 {
			end := strings.Index(trimmed[idx:], "]")
			if end > 0 {
				pm := trimmed[idx+1 : idx+end]
				pmParts := strings.SplitN(pm, "/", 2)
				if len(pmParts) == 2 {
					pref, _ = strconv.Atoi(pmParts[0])
					metric, _ = strconv.Atoi(pmParts[1])
				}
			}
		}

		// Extract "via X.X.X.X"
		if idx := strings.Index(strings.ToLower(trimmed), "via "); idx >= 0 {
			rest := trimmed[idx+4:]
			viaFields := strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' })
			if len(viaFields) > 0 { nextHop = viaFields[0] }
		}

		// Extract interface: last comma-separated field, or "directly connected, X"
		commaFields := strings.Split(trimmed, ",")
		last := strings.TrimSpace(commaFields[len(commaFields)-1])
		if isInterfaceName(last) { outIface = last }

		result.RIBEntries = append(result.RIBEntries, model.RIBEntry{
			Prefix: prefix, MaskLen: maskLen, Protocol: proto,
			Preference: pref, Metric: metric, NextHop: nextHop,
			OutgoingInterface: outIface, VRF: "default",
		})
	}
	return result, nil
}

func ciscoCodeToProtocol(code string) string {
	code = strings.TrimRight(code, "*")
	switch code {
	case "C", "L": return "direct"
	case "S": return "static"
	case "O", "IA": return "ospf"
	case "B": return "bgp"
	case "D", "EX": return "eigrp"
	case "R": return "rip"
	case "i", "ia", "L1", "L2": return "isis"
	default: return strings.ToLower(code)
	}
}

func isInterfaceName(s string) bool {
	prefixes := []string{"gi", "fa", "te", "hu", "et", "lo", "vl", "po", "tu", "nu", "se"}
	lower := strings.ToLower(s)
	for _, p := range prefixes { if strings.HasPrefix(lower, p) { return true } }
	return false
}
