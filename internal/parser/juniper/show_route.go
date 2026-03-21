// internal/parser/juniper/show_route.go
package juniper

import (
	"regexp"
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// Juniper route format:
// 10.1.1.0/24      *[OSPF/10] 00:05:12, metric 2
//                     >  to 10.0.0.1 via ge-0/0/0.0
var routeHeadRe = regexp.MustCompile(`^(\d+\.\d+\.\d+\.\d+/\d+)\s+\*?\[([^/]+)/(\d+)\]`)
var routeNextHopRe = regexp.MustCompile(`to\s+(\d+\.\d+\.\d+\.\d+)\s+via\s+(\S+)`)

func ParseShowRoute(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdRIB, RawText: raw}
	lines := strings.Split(raw, "\n")

	var current *model.RIBEntry

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }

		// Try route header: "10.1.1.0/24 *[OSPF/10] ..."
		if m := routeHeadRe.FindStringSubmatch(trimmed); m != nil {
			// Save previous entry
			if current != nil { result.RIBEntries = append(result.RIBEntries, *current) }

			parts := strings.SplitN(m[1], "/", 2)
			maskLen, _ := strconv.Atoi(parts[1])
			pref, _ := strconv.Atoi(m[3])
			proto := strings.ToLower(m[2])

			var metric int
			if idx := strings.Index(trimmed, "metric "); idx >= 0 {
				metricStr := strings.Fields(trimmed[idx+7:])[0]
				metric, _ = strconv.Atoi(strings.TrimRight(metricStr, ","))
			}

			current = &model.RIBEntry{
				Prefix: parts[0], MaskLen: maskLen, Protocol: proto,
				Preference: pref, Metric: metric, VRF: "default",
			}
			continue
		}

		// Try next-hop line: "> to X.X.X.X via ge-0/0/0.0"
		if current != nil {
			if m := routeNextHopRe.FindStringSubmatch(trimmed); m != nil {
				current.NextHop = m[1]
				current.OutgoingInterface = m[2]
			}
		}
	}

	// Don't forget last entry
	if current != nil { result.RIBEntries = append(result.RIBEntries, *current) }

	return result, nil
}
