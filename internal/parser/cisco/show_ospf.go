// internal/parser/cisco/show_ospf.go
package cisco

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseShowOSPFNeighbor parses "show ip ospf neighbor" output.
func ParseShowOSPFNeighbor(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Neighbor ID") && strings.Contains(trimmed, "State") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 6 { continue }

		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "ospf", RemoteID: fields[0],
			State: strings.ToLower(fields[2]),
			RemoteAddress: fields[4], LocalInterface: fields[5],
		})
	}
	return result, nil
}
