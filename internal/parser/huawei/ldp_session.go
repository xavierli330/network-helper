package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

func parseLdpSession(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}
		if !headerFound {
			if strings.Contains(trimmed, "Peer LDP ID") && strings.Contains(trimmed, "State") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		remoteID := strings.Split(fields[0], ":")[0]
		var uptime string
		if len(fields) >= 5 {
			uptime = fields[4]
		}
		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "ldp",
			RemoteID: remoteID,
			State:    strings.ToLower(fields[1]),
			Uptime:   uptime,
		})
	}
	return result, nil
}

// ParseNeighbor routes to the appropriate neighbor parser based on content.
func ParseNeighbor(raw string) (model.ParseResult, error) {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "ospf") && strings.Contains(lower, "area id"):
		return parseOspfPeer(raw)
	case strings.Contains(lower, "peer ldp id"):
		return parseLdpSession(raw)
	default:
		return model.ParseResult{Type: model.CmdNeighbor, RawText: raw}, nil
	}
}
