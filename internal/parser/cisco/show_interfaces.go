// internal/parser/cisco/show_interfaces.go
package cisco

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseShowIPInterfaceBrief parses "show ip interface brief" output.
func ParseShowIPInterfaceBrief(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Interface") && strings.Contains(trimmed, "IP-Address") { headerFound = true }
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 6 { continue }

		name := fields[0]
		ip := fields[1]
		if ip == "unassigned" { ip = "" }
		status := strings.ToLower(fields[4])
		proto := strings.ToLower(fields[5])
		_ = proto

		// "administratively down" → "admin-down"
		if status == "administratively" && len(fields) > 5 {
			status = "admin-down"
			// Re-parse: "administratively down down" shifts fields
			if len(fields) >= 7 { proto = strings.ToLower(fields[6]) }
		}

		iface := model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: status, IPAddress: ip,
		}
		result.Interfaces = append(result.Interfaces, iface)
	}
	return result, nil
}
