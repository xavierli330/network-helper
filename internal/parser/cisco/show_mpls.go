// internal/parser/cisco/show_mpls.go
package cisco

import (
	"strconv"
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseShowMplsForwarding parses "show mpls forwarding-table" output.
func ParseShowMplsForwarding(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdLFIB, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" { continue }
		if !headerFound {
			if strings.Contains(trimmed, "Local") && strings.Contains(trimmed, "Outgoing") { headerFound = true }
			// Skip the second header line too
			continue
		}
		// Skip continuation header lines
		if strings.Contains(trimmed, "Label") && strings.Contains(trimmed, "interface") { continue }

		fields := strings.Fields(trimmed)
		if len(fields) < 5 { continue }

		inLabel, err := strconv.Atoi(fields[0])
		if err != nil { continue }

		outLabelStr := fields[1]
		var action string
		var outLabel string

		lower := strings.ToLower(outLabelStr)
		switch {
		case lower == "pop":
			action = "pop"
			outLabel = ""
			// "Pop Label" is two words
		case strings.HasPrefix(lower, "no"):
			action = "pop"
			outLabel = ""
			// "No Label" is two words
		default:
			outLabel = outLabelStr
			action = "swap"
		}

		// Handle two-word labels: "Pop Label", "No Label"
		// If outLabelStr is "Pop" or "No", the actual fields shift
		var fec, outIface, nextHop string
		if lower == "pop" || lower == "no" {
			// fields[1]="Pop" fields[2]="Label" fields[3]=prefix fields[4]=bytes fields[5]=iface fields[6]=nexthop
			if len(fields) >= 7 {
				fec = fields[3]
				outIface = fields[5]
				nextHop = fields[6]
			}
		} else {
			// fields[1]=outlabel fields[2]=prefix fields[3]=bytes fields[4]=iface fields[5]=nexthop
			if len(fields) >= 6 {
				fec = fields[2]
				outIface = fields[4]
				nextHop = fields[5]
			}
		}

		result.LFIBEntries = append(result.LFIBEntries, model.LFIBEntry{
			InLabel: inLabel, Action: action, OutLabel: outLabel,
			FEC: fec, OutgoingInterface: outIface, NextHop: nextHop,
		})
	}
	return result, nil
}
