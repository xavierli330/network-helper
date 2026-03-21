// internal/parser/juniper/show_ospf.go
package juniper

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseShowOSPFNeighbor(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Address") && strings.Contains(trimmed, "State") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 { continue }
		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "ospf", RemoteAddress: fields[0], LocalInterface: fields[1],
			State: strings.ToLower(fields[2]), RemoteID: fields[3],
		})
	}
	return result, nil
}
