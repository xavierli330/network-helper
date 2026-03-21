// internal/graph/builder.go
package graph

import (
	"fmt"
	"strings"

	"github.com/xavierli/nethelper/internal/store"
)

// BuildFromDB loads all data from the store and constructs the in-memory graph.
func BuildFromDB(db *store.DB) (*Graph, error) {
	g := New()

	// 1. Load devices as nodes
	devices, err := db.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	for _, d := range devices {
		g.AddNode(d.ID, NodeTypeDevice, map[string]string{
			"hostname":  d.Hostname,
			"vendor":    d.Vendor,
			"router_id": d.RouterID,
			"mgmt_ip":   d.MgmtIP,
		})
	}

	// 2. Load interfaces and create HAS_INTERFACE + MEMBER_OF edges
	for _, d := range devices {
		ifaces, err := db.GetInterfaces(d.ID)
		if err != nil {
			continue
		}

		for _, iface := range ifaces {
			nodeID := iface.ID
			if nodeID == "" {
				nodeID = d.ID + ":" + iface.Name
			}
			g.AddNode(nodeID, NodeTypeInterface, map[string]string{
				"name":      iface.Name,
				"device_id": d.ID,
				"type":      string(iface.Type),
				"status":    iface.Status,
				"ip":        iface.IPAddress,
				"mask":      iface.Mask,
			})
			g.AddEdge(d.ID, nodeID, EdgeHasInterface, nil)

			// MEMBER_OF for trunk members
			if iface.ParentID != "" {
				g.AddEdge(nodeID, iface.ParentID, EdgeMemberOf, nil)
			}

			// Create subnet nodes from IP/mask
			if iface.IPAddress != "" && iface.Mask != "" {
				subnetID := computeSubnetID(iface.IPAddress, iface.Mask)
				if subnetID != "" {
					if _, ok := g.GetNode(subnetID); !ok {
						g.AddNode(subnetID, NodeTypeSubnet, map[string]string{"prefix": subnetID})
					}
					g.AddEdge(nodeID, subnetID, EdgeInSubnet, nil)
				}
			}
		}
	}

	// 3. Build CONNECTS_TO edges — interfaces in the same subnet are connected
	subnets := g.NodesByType(NodeTypeSubnet)
	for _, subnet := range subnets {
		// Find all interfaces pointing to this subnet
		var ifaceIDs []string
		for _, d := range devices {
			for _, e := range g.NeighborsByType(d.ID, EdgeHasInterface) {
				for _, se := range g.NeighborsByType(e.To, EdgeInSubnet) {
					if se.To == subnet.ID {
						ifaceIDs = append(ifaceIDs, e.To)
					}
				}
			}
		}
		// Connect each pair bidirectionally
		for i := 0; i < len(ifaceIDs); i++ {
			for j := i + 1; j < len(ifaceIDs); j++ {
				g.AddBidirectionalEdge(ifaceIDs[i], ifaceIDs[j], EdgeConnectsTo, nil)
			}
		}
	}

	// 4. Build PEER edges from protocol neighbors (latest snapshot per device)
	for _, d := range devices {
		snapID, err := db.LatestSnapshotID(d.ID)
		if err != nil {
			continue
		}

		neighbors, err := db.GetNeighbors(d.ID, snapID)
		if err != nil {
			continue
		}

		for _, n := range neighbors {
			// Try to find the remote device by router-ID or remote-ID
			remoteDevID := findDeviceByRouterID(g, n.RemoteID)
			if remoteDevID == "" {
				remoteDevID = findDeviceByRouterID(g, n.RemoteAddress)
			}
			if remoteDevID == "" {
				continue
			}

			g.AddEdge(d.ID, remoteDevID, EdgePeer, map[string]string{
				"protocol":  n.Protocol,
				"state":     n.State,
				"local_if":  n.LocalInterface,
				"remote_id": n.RemoteID,
			})
		}
	}

	return g, nil
}

// findDeviceByRouterID searches for a device node whose router_id matches.
func findDeviceByRouterID(g *Graph, routerID string) string {
	if routerID == "" {
		return ""
	}
	for _, n := range g.NodesByType(NodeTypeDevice) {
		if n.Props["router_id"] == routerID {
			return n.ID
		}
	}
	// Also try matching by device ID (lowercase)
	if _, ok := g.GetNode(strings.ToLower(routerID)); ok {
		return strings.ToLower(routerID)
	}
	return ""
}

// computeSubnetID computes the network address string from an IP and mask length.
// E.g., "10.0.0.1" + "30" → "10.0.0.0/30"
func computeSubnetID(ip, mask string) string {
	if ip == "" || mask == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ""
	}

	var ipBytes [4]byte
	for i, p := range parts {
		n := 0
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return ""
			}
			n = n*10 + int(ch-'0')
		}
		ipBytes[i] = byte(n)
	}

	maskLen := 0
	for _, ch := range mask {
		if ch < '0' || ch > '9' {
			return ""
		}
		maskLen = maskLen*10 + int(ch-'0')
	}
	if maskLen < 0 || maskLen > 32 {
		return ""
	}

	// Compute the subnet mask bytes
	remaining := maskLen
	var maskBytes [4]byte
	for i := 0; i < 4; i++ {
		if remaining >= 8 {
			maskBytes[i] = 0xFF
			remaining -= 8
		} else if remaining > 0 {
			maskBytes[i] = byte(0xFF << (8 - remaining))
			remaining = 0
		} else {
			maskBytes[i] = 0x00
		}
	}

	// Apply mask to get network address
	netIP := fmt.Sprintf("%d.%d.%d.%d",
		ipBytes[0]&maskBytes[0],
		ipBytes[1]&maskBytes[1],
		ipBytes[2]&maskBytes[2],
		ipBytes[3]&maskBytes[3],
	)
	return netIP + "/" + mask
}
