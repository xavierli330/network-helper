package plan

import (
	"fmt"
	"sort"
)

// bgpIsolateStep builds a DeviceCommand that applies "peer X ignore" for every
// peer in the group, wrapping the peers inside a bgp <localAS> block.
func bgpIsolateStep(deviceID string, localAS int, pg PeerGroup, vendor string) DeviceCommand {
	lines := []string{
		"system-view",
		fmt.Sprintf("bgp %d", localAS),
	}
	for _, p := range pg.Peers {
		line := fmt.Sprintf("peer %s ignore", p.PeerIP)
		if p.Description != "" {
			line += fmt.Sprintf("  # %s", p.Description)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "quit", "return")

	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: lines,
		Purpose:  fmt.Sprintf("BGP 隔离 — %s (%d peers, AS %s)", pg.Name, len(pg.Peers), formatASList(pg)),
	}
}

// bgpCheckpoint builds a DeviceCommand with per-peer display commands followed
// by group-level summary checks.  The commands are read-only and serve as the
// operator's verification step after bgpIsolateStep has been applied.
func bgpCheckpoint(deviceID string, pg PeerGroup, vendor string) DeviceCommand {
	var lines []string
	for _, p := range pg.Peers {
		lines = append(lines, fmt.Sprintf("display bgp peer %s", p.PeerIP))
	}
	lines = append(lines,
		"display bgp peer | include Established",
		"display bgp routing-table statistics",
	)

	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: lines,
		Purpose:  fmt.Sprintf(">>> 检查点: %s 组 peers 应变为 Idle <<<", pg.Name),
	}
}

// bgpRollbackStep builds a DeviceCommand that removes "peer X ignore" for every
// peer in the group, effectively restoring BGP sessions.
func bgpRollbackStep(deviceID string, localAS int, pg PeerGroup, vendor string) DeviceCommand {
	lines := []string{
		"system-view",
		fmt.Sprintf("bgp %d", localAS),
	}
	for _, p := range pg.Peers {
		line := fmt.Sprintf("undo peer %s ignore", p.PeerIP)
		if p.Description != "" {
			line += fmt.Sprintf("  # %s", p.Description)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "quit", "return")

	return DeviceCommand{
		DeviceID: deviceID,
		Vendor:   vendor,
		Commands: lines,
		Purpose:  fmt.Sprintf("BGP 回退 — 恢复 %s (%d peers)", pg.Name, len(pg.Peers)),
	}
}

// formatASList returns a compact summary of the remote ASes in a peer group.
// A single unique AS is returned as its number string; multiple ASes are
// summarised as "<N> ASes".
func formatASList(pg PeerGroup) string {
	seen := make(map[int]struct{})
	for _, p := range pg.Peers {
		seen[p.RemoteAS] = struct{}{}
	}

	if len(seen) == 1 {
		// Extract the single AS number deterministically.
		asList := make([]int, 0, 1)
		for as := range seen {
			asList = append(asList, as)
		}
		return fmt.Sprintf("%d", asList[0])
	}

	// Sort for deterministic output (useful in tests / display).
	asList := make([]int, 0, len(seen))
	for as := range seen {
		asList = append(asList, as)
	}
	sort.Ints(asList)
	return fmt.Sprintf("%d ASes", len(asList))
}
