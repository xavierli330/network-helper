package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

func parseOspfPeer(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}
		if !headerFound {
			if strings.Contains(trimmed, "Area Id") && strings.Contains(trimmed, "Neighbor id") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 {
			continue
		}
		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol:       "ospf",
			AreaID:         fields[0],
			LocalInterface: fields[1],
			RemoteID:       fields[2],
			State:          strings.ToLower(fields[3]),
		})
	}
	return result, nil
}
