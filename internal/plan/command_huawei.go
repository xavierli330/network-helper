package plan

import "fmt"

// HuaweiGenerator generates CLI commands for Huawei VRP devices.
type HuaweiGenerator struct{}

// CollectionCommands returns display commands to collect state from target and peer devices.
func (g *HuaweiGenerator) CollectionCommands(target string, links []Link) []DeviceCommand {
	protos := collectProtocols(links)

	cmds := []string{
		"display interface brief",
		"display ip routing-table statistics",
	}
	if protos["ospf"] {
		cmds = append(cmds, "display ospf peer brief")
	}
	if protos["bgp"] {
		cmds = append(cmds, "display bgp peer")
	}
	if protos["ldp"] {
		cmds = append(cmds, "display mpls ldp session")
	}

	targetCmds := cmds
	targetCmds = append(targetCmds,
		"display current-configuration",
		"display lldp neighbor brief",
		"display version",
	)

	result := []DeviceCommand{
		{
			DeviceID: target,
			Vendor:   "huawei",
			Commands: targetCmds,
			Purpose:  "Collect baseline state from target device",
		},
	}

	for _, peer := range uniquePeers(links) {
		peerCmds := []string{
			"display interface brief",
			"display ip routing-table statistics",
		}
		// Collect per-peer protocols from links involving this peer.
		peerProtos := make(map[string]bool)
		for _, l := range links {
			if l.PeerDevice == peer {
				for _, p := range l.Protocols {
					peerProtos[p] = true
				}
			}
		}
		if peerProtos["ospf"] {
			peerCmds = append(peerCmds, "display ospf peer brief")
		}
		if peerProtos["bgp"] {
			peerCmds = append(peerCmds, "display bgp peer")
		}
		if peerProtos["ldp"] {
			peerCmds = append(peerCmds, "display mpls ldp session")
		}
		result = append(result, DeviceCommand{
			DeviceID: peer,
			Vendor:   "huawei",
			Commands: peerCmds,
			Purpose:  fmt.Sprintf("Collect baseline state from peer device %s", peer),
		})
	}

	return result
}

// PreCheckCommands returns display commands to verify state on the target device before isolation.
func (g *HuaweiGenerator) PreCheckCommands(target string, links []Link) []DeviceCommand {
	protos := collectProtocols(links)

	cmds := []string{
		"display interface brief",
		"display ip routing-table statistics",
	}
	if protos["ospf"] {
		cmds = append(cmds, "display ospf peer brief")
	}
	if protos["bgp"] {
		cmds = append(cmds, "display bgp peer")
	}
	if protos["ldp"] {
		cmds = append(cmds, "display mpls ldp session")
	}

	return []DeviceCommand{
		{
			DeviceID: target,
			Vendor:   "huawei",
			Commands: cmds,
			Purpose:  "Pre-check: verify current state on target device",
		},
	}
}

// ProtocolIsolateCommands returns configuration commands to isolate routing protocols on the target.
func (g *HuaweiGenerator) ProtocolIsolateCommands(target string, links []Link) []DeviceCommand {
	cmds := []string{"system-view"}

	for _, l := range links {
		for _, p := range l.Protocols {
			switch p {
			case "ospf":
				cmds = append(cmds,
					fmt.Sprintf("interface %s", l.LocalInterface),
					"ospf cost 65535",
					"quit",
				)
			case "bgp":
				if l.PeerIP == "" {
					continue
				}
				cmds = append(cmds,
					"bgp",
					fmt.Sprintf("peer %s ignore", l.PeerIP),
					"quit",
				)
			case "ldp":
				cmds = append(cmds,
					fmt.Sprintf("interface %s", l.LocalInterface),
					"undo mpls ldp",
					"quit",
				)
			}
		}
	}

	cmds = append(cmds, "return")

	verifyCmd := []string{
		"display ospf peer brief",
		"display bgp peer",
	}

	return []DeviceCommand{
		{
			DeviceID: target,
			Vendor:   "huawei",
			Commands: cmds,
			Purpose:  "Protocol isolation: raise OSPF cost, suppress BGP peers, disable LDP",
		},
		{
			DeviceID: target,
			Vendor:   "huawei",
			Commands: verifyCmd,
			Purpose:  "Verify protocol isolation took effect",
		},
	}
}

// InterfaceIsolateCommands shuts down each unique local interface involved in links.
func (g *HuaweiGenerator) InterfaceIsolateCommands(target string, links []Link) []DeviceCommand {
	seen := make(map[string]bool)
	cmds := []string{"system-view"}

	for _, l := range links {
		if l.LocalInterface == "" || seen[l.LocalInterface] {
			continue
		}
		seen[l.LocalInterface] = true
		cmds = append(cmds,
			fmt.Sprintf("interface %s", l.LocalInterface),
			"shutdown",
			"quit",
		)
	}

	cmds = append(cmds, "return")

	return []DeviceCommand{
		{
			DeviceID: target,
			Vendor:   "huawei",
			Commands: cmds,
			Purpose:  "Interface isolation: shut down all links to target device",
		},
	}
}

// PostCheckCommands returns display commands to verify state on each peer device after isolation.
func (g *HuaweiGenerator) PostCheckCommands(target string, links []Link) []DeviceCommand {
	var result []DeviceCommand

	for _, peer := range uniquePeers(links) {
		peerProtos := make(map[string]bool)
		for _, l := range links {
			if l.PeerDevice == peer {
				for _, p := range l.Protocols {
					peerProtos[p] = true
				}
			}
		}

		cmds := []string{
			"display interface brief",
			"display ip routing-table statistics",
		}
		if peerProtos["ospf"] {
			cmds = append(cmds, "display ospf peer brief")
		}
		if peerProtos["bgp"] {
			cmds = append(cmds, "display bgp peer")
		}
		if peerProtos["ldp"] {
			cmds = append(cmds, "display mpls ldp session")
		}

		result = append(result, DeviceCommand{
			DeviceID: peer,
			Vendor:   "huawei",
			Commands: cmds,
			Purpose:  fmt.Sprintf("Post-check: verify peer %s after isolation", peer),
		})
	}

	return result
}

// RollbackCommands reverses protocol and interface isolation on the target device.
func (g *HuaweiGenerator) RollbackCommands(target string, links []Link) []DeviceCommand {
	// Correct order: first undo protocols, then re-enable interfaces.
	protoCmds := []string{"system-view"}
	for _, l := range links {
		for _, p := range l.Protocols {
			switch p {
			case "ospf":
				protoCmds = append(protoCmds,
					fmt.Sprintf("interface %s", l.LocalInterface),
					"undo ospf cost",
					"quit",
				)
			case "bgp":
				if l.PeerIP == "" {
					continue
				}
				protoCmds = append(protoCmds,
					"bgp",
					fmt.Sprintf("undo peer %s ignore", l.PeerIP),
					"quit",
				)
			case "ldp":
				protoCmds = append(protoCmds,
					fmt.Sprintf("interface %s", l.LocalInterface),
					"mpls ldp",
					"quit",
				)
			}
		}
	}
	protoCmds = append(protoCmds, "return")

	ifaceCmds := []string{"system-view"}
	seen := make(map[string]bool)
	for _, l := range links {
		if l.LocalInterface == "" || seen[l.LocalInterface] {
			continue
		}
		seen[l.LocalInterface] = true
		ifaceCmds = append(ifaceCmds,
			fmt.Sprintf("interface %s", l.LocalInterface),
			"undo shutdown",
			"quit",
		)
	}
	ifaceCmds = append(ifaceCmds, "return")

	return []DeviceCommand{
		{
			DeviceID: target,
			Vendor:   "huawei",
			Commands: protoCmds,
			Purpose:  "Rollback: restore protocol settings",
		},
		{
			DeviceID: target,
			Vendor:   "huawei",
			Commands: ifaceCmds,
			Purpose:  "Rollback: re-enable interfaces",
		},
	}
}
