package plan

import (
	"strings"

	"github.com/xavierli/nethelper/internal/graph"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

// DiscoverLinks discovers all links from the target device using three strategies:
//  1. Description matching — if an interface description mentions a known hostname, infer a link.
//  2. Subnet matching via graph — interfaces that share a /30 (or any subnet) are connected.
//  3. Config enrichment — if the config contains routing-protocol keywords, tag every link.
func DiscoverLinks(db *store.DB, deviceID string) ([]Link, error) {
	// linkMap deduplicates by "localIfaceName:peerDeviceID"
	linkMap := make(map[string]*Link)

	// Load target device's interfaces once; used by both strategy 1 and resolveParent.
	ifaces, err := db.GetInterfaces(deviceID)
	if err != nil {
		return nil, err
	}

	// ── Strategy 1: Description matching ──────────────────────────────────────
	devices, err := db.ListDevices()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		descLower := strings.ToLower(iface.Description)
		if descLower == "" {
			continue
		}
		for _, d := range devices {
			if d.ID == deviceID || d.Hostname == "" {
				continue
			}
			if strings.Contains(descLower, strings.ToLower(d.Hostname)) {
				localIfName := resolveParent(iface, ifaces)
				key := localIfName + ":" + d.ID
				if l, ok := linkMap[key]; ok {
					addSource(l, "description")
				} else {
					linkMap[key] = &Link{
						LocalDevice:    deviceID,
						LocalInterface: localIfName,
						LocalIP:        iface.IPAddress,
						PeerDevice:     d.ID,
						Sources:        []string{"description"},
					}
				}
			}
		}
	}

	// ── Strategy 2: Subnet matching via graph ──────────────────────────────────
	g, err := graph.BuildFromDB(db)
	if err == nil {
		for _, e1 := range g.NeighborsByType(deviceID, graph.EdgeHasInterface) {
			localNode, ok := g.GetNode(e1.To)
			if !ok {
				continue
			}
			for _, e2 := range g.NeighborsByType(e1.To, graph.EdgeConnectsTo) {
				remoteNode, ok := g.GetNode(e2.To)
				if !ok {
					continue
				}
				peerDevID := remoteNode.Props["device_id"]
				if peerDevID == "" || peerDevID == deviceID {
					continue
				}

				localIfName := localNode.Props["name"]
				key := localIfName + ":" + peerDevID
				if l, ok := linkMap[key]; ok {
					addSource(l, "subnet")
					if l.PeerInterface == "" {
						l.PeerInterface = remoteNode.Props["name"]
					}
					if l.PeerIP == "" {
						l.PeerIP = remoteNode.Props["ip"]
					}
				} else {
					linkMap[key] = &Link{
						LocalDevice:    deviceID,
						LocalInterface: localIfName,
						LocalIP:        localNode.Props["ip"],
						PeerDevice:     peerDevID,
						PeerInterface:  remoteNode.Props["name"],
						PeerIP:         remoteNode.Props["ip"],
						Sources:        []string{"subnet"},
					}
				}
			}
		}
	}

	// ── Strategy 3: Config enrichment ─────────────────────────────────────────
	if len(linkMap) > 0 {
		snapshots, err := db.GetConfigSnapshots(deviceID)
		if err == nil && len(snapshots) > 0 {
			cfgLower := strings.ToLower(snapshots[0].ConfigText)
			var protocols []string
			if strings.Contains(cfgLower, "ospf") {
				protocols = append(protocols, "ospf")
			}
			if strings.Contains(cfgLower, "bgp ") {
				protocols = append(protocols, "bgp")
			}
			if strings.Contains(cfgLower, "mpls ldp") {
				protocols = append(protocols, "ldp")
			}
			for _, l := range linkMap {
				for _, p := range protocols {
					if !containsStr(l.Protocols, p) {
						l.Protocols = append(l.Protocols, p)
					}
				}
			}
		}
	}

	// Flatten map to slice.
	links := make([]Link, 0, len(linkMap))
	for _, l := range linkMap {
		links = append(links, *l)
	}
	return links, nil
}

// resolveParent returns the parent interface name when iface is a trunk/LAG
// member (ParentID != ""); otherwise returns iface.Name.
func resolveParent(iface model.Interface, all []model.Interface) string {
	if iface.ParentID != "" {
		for _, p := range all {
			if p.ID == iface.ParentID {
				return p.Name
			}
		}
	}
	return iface.Name
}

// containsStr reports whether s is already in ss.
func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// addSource appends source to l.Sources if not already present.
func addSource(l *Link, source string) {
	if !containsStr(l.Sources, source) {
		l.Sources = append(l.Sources, source)
	}
}
