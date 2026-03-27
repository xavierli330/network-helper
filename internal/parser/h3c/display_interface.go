// internal/parser/h3c/display_interface.go
package h3c

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseInterface dispatches to the appropriate parser based on output format:
//   - "display ip interface [brief]" → IP-focused table (Physical/Protocol/IP columns)
//   - "display interface [name] [brief]" → link-state table (Link/Speed columns)
//   - "display interface <name>" (verbose) → per-interface block format
func ParseInterface(raw string) (model.ParseResult, error) {
	for _, line := range strings.SplitN(raw, "\n", 30) {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "(") {
			continue
		}
		// "display ip interface brief" variants — header contains IP column keyword
		if (strings.Contains(t, "IP Address") || strings.Contains(t, "Primary IP")) &&
			(strings.Contains(t, "Physical") || strings.Contains(t, "Protocol") || strings.Contains(t, "Link")) {
			return parseIPInterfaceBrief(raw)
		}
		// "display interface brief" — header contains Speed column
		if strings.Contains(t, "Link") && strings.Contains(t, "Speed") {
			return parseInterfaceBrief(raw)
		}
		// Non-empty non-marker line that didn't match any header → keep scanning
		// (could be a preamble line like "Brief information on interfaces in route mode:")
		// But stop after an interface-name-looking line to avoid verbose false-positives.
		lower := strings.ToLower(t)
		isIfaceLine := false
		for _, prefix := range []string{"gigabitethernet", "ten-gigabitethernet", "fortygige", "fge", "xge",
			"hundredgige", "loopback", "vlanif", "bridge-aggregation", "bagg", "tunnel", "null"} {
			if strings.HasPrefix(lower, prefix) {
				isIfaceLine = true
				break
			}
		}
		if isIfaceLine {
			break
		}
	}
	return parseInterfaceVerbose(raw)
}

// parseIPInterfaceBrief parses both variants of the IP interface brief table:
//
// Variant 1 (newer H3C):
//
//	Interface                     Physical Protocol IP Address      Description
//	FGE2/0/1                      up       up       10.48.58.244    SH-YQ-...
//
// Variant 2 (older H3C):
//
//	Interface            Link Protocol Primary IP      Description
//	GE1/0/1              UP   UP       10.0.0.1
func parseIPInterfaceBrief(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	headerFound := false
	// ipCol is the 0-based field index of the IP address column (3 in both variants).
	const ipCol = 3

	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimRight(line, "\r \t")
		if t == "" || strings.HasPrefix(strings.TrimSpace(t), "*") || strings.HasPrefix(strings.TrimSpace(t), "(") {
			continue
		}
		if !headerFound {
			ts := strings.TrimSpace(t)
			if (strings.Contains(ts, "IP Address") || strings.Contains(ts, "Primary IP")) &&
				(strings.Contains(ts, "Physical") || strings.Contains(ts, "Protocol")) {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(t)
		if len(fields) < 3 {
			continue
		}
		name := fields[0]
		physical := strings.ToLower(fields[1])
		if strings.HasPrefix(physical, "*") {
			physical = "admin-down"
		}
		ip := ""
		if len(fields) > ipCol {
			candidate := fields[ipCol]
			if candidate != "unassigned" && candidate != "--" && strings.Contains(candidate, ".") {
				ip = candidate
			}
		}
		desc := ""
		if len(fields) > ipCol+1 {
			desc = strings.Join(fields[ipCol+1:], " ")
		}
		result.Interfaces = append(result.Interfaces, model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: physical,
			IPAddress: ip, Description: desc,
		})
	}
	return result, nil
}

// parseInterfaceBrief parses "display interface brief":
//
//	Interface            Link Speed   Duplex Type  PVID Description
//	FGE2/0/1             UP   40G(a)  Full   A     1
func parseInterfaceBrief(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	headerFound := false

	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimRight(line, "\r \t")
		if t == "" {
			continue
		}
		if !headerFound {
			if strings.Contains(t, "Interface") && strings.Contains(t, "Link") && strings.Contains(t, "Speed") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(t)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		status := strings.ToLower(fields[1])
		if status == "adm" {
			status = "admin-down"
		}
		desc := ""
		if len(fields) >= 6 {
			desc = strings.Join(fields[5:], " ")
		}
		result.Interfaces = append(result.Interfaces, model.Interface{
			Name: name, Type: inferInterfaceType(name), Status: status, Description: desc,
		})
	}
	return result, nil
}

// parseInterfaceVerbose parses verbose "display interface <name>" block output.
func parseInterfaceVerbose(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdInterface, RawText: raw}
	var current *model.Interface

	flush := func() {
		if current != nil {
			result.Interfaces = append(result.Interfaces, *current)
			current = nil
		}
	}

	ifacePrefixes := []string{
		"gigabitethernet", "ten-gigabitethernet", "fortygige", "fge", "xge",
		"hundredgige", "loopback", "vlanif", "bridge-aggregation", "bagg", "tunnel", "null",
	}

	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimRight(line, "\r \t")
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, " ") && !strings.HasPrefix(t, "\t") {
			lower := strings.ToLower(t)
			for _, prefix := range ifacePrefixes {
				if strings.HasPrefix(lower, prefix) {
					flush()
					name := strings.Fields(t)[0]
					current = &model.Interface{Name: name, Type: inferInterfaceType(name)}
					break
				}
			}
			continue
		}
		if current == nil {
			continue
		}
		trimmed := strings.TrimSpace(t)
		lc := strings.ToLower(trimmed)

		switch {
		case strings.HasPrefix(lc, "current state:"):
			state := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
			current.Status = strings.ToLower(strings.Fields(state)[0])
		case strings.HasPrefix(lc, "description:"):
			current.Description = strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1])
		case strings.HasPrefix(lc, "internet address is"):
			// "Internet Address is 10.x.x.x/30 Primary"
			for _, p := range strings.Fields(trimmed) {
				if strings.Contains(p, ".") && strings.Contains(p, "/") {
					current.IPAddress = strings.Split(p, "/")[0]
					break
				}
			}
		}
	}
	flush()
	return result, nil
}

// ParseInterfaceBrief is kept for backward compatibility.
func ParseInterfaceBrief(raw string) (model.ParseResult, error) {
	return ParseInterface(raw)
}
