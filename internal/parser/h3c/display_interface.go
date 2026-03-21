// internal/parser/h3c/display_interface.go
package h3c

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
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Interface") && strings.Contains(trimmed, "Link") && strings.Contains(trimmed, "Protocol") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 { continue }

		name := fields[0]
		linkStatus := strings.ToLower(fields[1])
		if linkStatus == "adm" { linkStatus = "admin-down" }

		var ip string
		if len(fields) >= 4 && fields[3] != "--" { ip = fields[3] }

		result.Interfaces = append(result.Interfaces, model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: linkStatus, IPAddress: ip,
		})
	}
	return result, nil
}
