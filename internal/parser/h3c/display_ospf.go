// internal/parser/h3c/display_ospf.go
package h3c

import (
	"strings"
	"github.com/xavierli/nethelper/internal/model"
)

// ParseNeighbor auto-detects BGP vs OSPF/ISIS output and delegates accordingly.
func ParseNeighbor(raw string) (model.ParseResult, error) {
	for _, line := range strings.SplitN(raw, "\n", 30) {
		t := strings.TrimSpace(line)
		if strings.Contains(t, "BGP local router ID") || strings.Contains(t, "MsgRcvd") {
			return parseBGPPeer(raw)
		}
		if strings.Contains(t, "Area Id") && strings.Contains(t, "Neighbor") {
			return parseOSPFPeer(raw)
		}
	}
	r, _ := parseBGPPeer(raw)
	if len(r.Neighbors) > 0 {
		return r, nil
	}
	return parseOSPFPeer(raw)
}

// parseBGPPeer parses "display bgp peer ipv4" summary table:
//
//	  Peer                    AS  MsgRcvd  MsgSent OutQ PrefRcv Up/Down  State
//	  10.48.58.245          1001  4769972  ...                           Established
func parseBGPPeer(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	headerFound := false

	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimRight(line, "\r \t")
		if t == "" {
			continue
		}
		if !headerFound {
			if strings.Contains(t, "Peer") && strings.Contains(t, "AS") && strings.Contains(t, "State") {
				headerFound = true
			}
			continue
		}
		trimmed := strings.TrimSpace(t)
		if trimmed == "" || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "-") {
			continue
		}
		fields := strings.Fields(trimmed)
		// Peer AS MsgRcvd MsgSent OutQ PrefRcv Up/Down State
		if len(fields) < 8 {
			continue
		}
		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "bgp",
			RemoteID: fields[0],
			AreaID:   fields[1], // reuse AreaID for AS number
			State:    strings.ToLower(fields[7]),
		})
	}
	return result, nil
}

// parseOSPFPeer parses "display ospf peer" summary table:
//
//	 Area Id          Interface            Neighbor id      State
//	 0.0.0.0          GE0/0                10.0.0.2         Full
func parseOSPFPeer(raw string) (model.ParseResult, error) {
	result := model.ParseResult{Type: model.CmdNeighbor, RawText: raw}
	headerFound := false

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}
		if !headerFound {
			if strings.Contains(trimmed, "Area Id") && strings.Contains(trimmed, "Neighbor") {
				headerFound = true
			}
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 {
			continue
		}
		result.Neighbors = append(result.Neighbors, model.NeighborInfo{
			Protocol: "ospf", AreaID: fields[0], LocalInterface: fields[1],
			RemoteID: fields[2], State: strings.ToLower(fields[3]),
		})
	}
	return result, nil
}

// ParseOspfPeer is kept for backward compatibility.
func ParseOspfPeer(raw string) (model.ParseResult, error) {
	return parseOSPFPeer(raw)
}
