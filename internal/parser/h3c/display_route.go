// internal/parser/h3c/display_route.go
package h3c

import (
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseRoutingTable(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdRIB, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false
	vrf := "default"

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if strings.HasPrefix(strings.TrimSpace(trimmed), "Routing Tables:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				vrfName := strings.TrimSpace(parts[1])
				if vrfName != "" && !strings.EqualFold(vrfName, "Public") { vrf = vrfName }
			}
			continue
		}
		if !headerFound {
			if strings.Contains(trimmed, "Destination/Mask") && strings.Contains(trimmed, "Proto") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 6 { continue }

		parts := strings.SplitN(fields[0], "/", 2)
		if len(parts) != 2 { continue }
		prefix := parts[0]
		maskLen, err := strconv.Atoi(parts[1])
		if err != nil { continue }

		proto := h3cProtoToStandard(fields[1])
		pref, _ := strconv.Atoi(fields[2])
		cost, _ := strconv.Atoi(fields[3])

		var nextHop, outIface string
		remaining := fields[4:]
		for i, f := range remaining {
			if isIPLike(f) { nextHop = f; if i+1 < len(remaining) { outIface = remaining[i+1] }; break }
		}
		if nextHop == "" && len(remaining) >= 2 { nextHop = remaining[len(remaining)-2]; outIface = remaining[len(remaining)-1] }

		result.RIBEntries = append(result.RIBEntries, model.RIBEntry{
			Prefix: prefix, MaskLen: maskLen, Protocol: proto,
			Preference: pref, Metric: cost, NextHop: nextHop,
			OutgoingInterface: outIface, VRF: vrf,
		})
	}
	return result, nil
}

func h3cProtoToStandard(proto string) string {
	lower := strings.ToLower(proto)
	switch {
	case strings.HasPrefix(lower, "o_"): return "ospf"
	case lower == "o": return "ospf"
	case lower == "s": return "static"
	case lower == "b": return "bgp"
	case lower == "d": return "direct"
	case lower == "i": return "isis"
	default: return lower
	}
}

func isIPLike(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 { return false }
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 { return false }
	}
	return true
}
