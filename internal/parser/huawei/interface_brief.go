package huawei

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

func ParseInterfaceBrief(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}
		if !headerFound {
			if strings.Contains(trimmed, "PHY") && strings.Contains(trimmed, "Protocol") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}
		phyStatus := strings.ToLower(fields[1])
		if strings.HasPrefix(phyStatus, "*") {
			phyStatus = "admin-down"
		}
		result.Interfaces = append(result.Interfaces, model.Interface{
			Name:   fields[0],
			Type:   inferInterfaceType(fields[0]),
			Status: phyStatus,
		})
	}
	return result, nil
}
