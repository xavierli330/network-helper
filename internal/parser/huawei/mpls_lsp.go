package huawei

import (
	"strconv"
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

func ParseMplsLsp(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdLFIB, RawText: raw}
	lines := strings.Split(raw, "\n")
	headerFound := false
	protocol := "ldp"

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "ldp lsp") {
			protocol = "ldp"
			continue
		}
		if strings.Contains(lower, "rsvp lsp") || strings.Contains(lower, "te lsp") {
			protocol = "rsvp"
			continue
		}
		if strings.Contains(lower, "sr lsp") || strings.Contains(lower, "segment-routing") {
			protocol = "sr"
			continue
		}

		if !headerFound {
			if strings.Contains(trimmed, "FEC") && strings.Contains(trimmed, "In/Out Label") {
				headerFound = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, " ---") || strings.HasPrefix(trimmed, "---") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		fec := fields[0]
		labels := strings.SplitN(fields[1], "/", 2)
		if len(labels) != 2 {
			continue
		}
		inLabel, err := strconv.Atoi(labels[0])
		if err != nil {
			continue
		}
		outLabelStr := labels[1]

		var outInterface string
		if len(fields) >= 3 {
			ifPair := fields[2]
			if strings.HasPrefix(ifPair, "-/") {
				outInterface = ifPair[2:]
			} else if strings.HasSuffix(ifPair, "/-") {
				outInterface = ""
			}
		}

		outLabelNum, outErr := strconv.Atoi(outLabelStr)
		action := determineLabelAction(inLabel, outLabelStr, outLabelNum, outErr)

		result.LFIBEntries = append(result.LFIBEntries, model.LFIBEntry{
			InLabel:           inLabel,
			Action:            action,
			OutLabel:          outLabelStr,
			OutgoingInterface: outInterface,
			FEC:               fec,
			Protocol:          protocol,
		})
	}
	return result, nil
}

func determineLabelAction(inLabel int, outLabelStr string, outLabelNum int, outErr error) string {
	switch {
	case outLabelStr == "-" || outLabelStr == "":
		return "pop"
	case outErr == nil && outLabelNum == 3:
		return "pop"
	case outErr == nil && outLabelNum == 0:
		return "pop"
	case inLabel == 3 && outErr == nil && outLabelNum > 3:
		return "push"
	case inLabel == 3:
		return "pop"
	default:
		return "swap"
	}
}
