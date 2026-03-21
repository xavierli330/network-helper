// internal/parser/juniper/show_interfaces.go
package juniper

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

func ParseShowInterfacesTerse(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Interface") && strings.Contains(trimmed, "Admin") && strings.Contains(trimmed, "Link") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 { continue }

		name := fields[0]
		admin := strings.ToLower(fields[1])
		link := strings.ToLower(fields[2])

		status := link
		if admin == "down" { status = "admin-down" }

		var ip string
		// If inet proto present, IP is after it: "inet 10.0.0.1/24"
		for i, f := range fields {
			if f == "inet" && i+1 < len(fields) {
				ipMask := fields[i+1]
				ip = strings.SplitN(ipMask, "/", 2)[0]
				break
			}
		}

		result.Interfaces = append(result.Interfaces, model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: status, IPAddress: ip,
		})
	}
	return result, nil
}
