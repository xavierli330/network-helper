package plan

import (
	"strings"

	"github.com/xavierli/nethelper/internal/graph"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

type PeerGroupRole string

const (
	RoleDownlink   PeerGroupRole = "downlink"
	RoleUplink     PeerGroupRole = "uplink"
	RoleManagement PeerGroupRole = "management"
)

type BGPPeerDetail struct {
	PeerIP      string
	RemoteAS    int
	Description string
	Interface   string // matched local interface (empty if no match)
}

type PeerGroup struct {
	Name    string
	Type    string        // "external" / "internal"
	Role    PeerGroupRole
	LocalAS int
	Peers   []BGPPeerDetail
}

type LAGBundle struct {
	Name        string
	IP          string
	Mask        string
	Description string
	Members     []string
}

type PhysicalLink struct {
	Interface   string
	IP          string
	Mask        string
	Description string
	PeerGroup   string // associated BGP peer group
}

type StaticRouteEntry struct {
	Prefix    string
	NextHop   string
	Interface string
	VRF       string
}

type VRFSummary struct {
	Name string
	RD   string
}

type DeviceTopology struct {
	DeviceID      string
	Hostname      string
	Vendor        string
	LocalAS       int
	Protocols     []string
	PeerGroups    []PeerGroup
	LAGs          []LAGBundle
	PhysicalLinks []PhysicalLink
	StaticRoutes  []StaticRouteEntry
	VRFs          []VRFSummary
	IsSPOF        bool
	ImpactDevices []string
}

// BuildTopology builds the multi-dimensional topology view for a single device.
func BuildTopology(db *store.DB, deviceID string) (DeviceTopology, error) {
	topo := DeviceTopology{DeviceID: deviceID}

	// Step 1: get device info
	dev, err := db.GetDevice(deviceID)
	if err != nil {
		return topo, err
	}
	topo.Hostname = dev.Hostname
	topo.Vendor = dev.Vendor

	// Step 2: get BGP peers and aggregate into PeerGroups
	var localAS int
	peerGroupMap := make(map[string]*PeerGroup)
	var peerGroupOrder []string // preserve insertion order

	peers, err := db.GetLatestBGPPeers(deviceID)
	if err == nil && len(peers) > 0 {
		for _, p := range peers {
			if localAS == 0 && p.LocalAS != 0 {
				localAS = p.LocalAS
			}
			pgName := p.PeerGroup
			if pgName == "" {
				pgName = p.PeerIP // use peer IP as group name if no group set
			}
			pg, exists := peerGroupMap[pgName]
			if !exists {
				pgType := "external"
				if p.RemoteAS != 0 && p.LocalAS != 0 && p.RemoteAS == p.LocalAS {
					pgType = "internal"
				}
				pg = &PeerGroup{
					Name:    pgName,
					Type:    pgType,
					LocalAS: p.LocalAS,
				}
				peerGroupMap[pgName] = pg
				peerGroupOrder = append(peerGroupOrder, pgName)
			}
			pg.Peers = append(pg.Peers, BGPPeerDetail{
				PeerIP:      p.PeerIP,
				RemoteAS:    p.RemoteAS,
				Description: p.Description,
			})
		}
	}
	topo.LocalAS = localAS

	// Step 4: get interfaces — separate LAGs, members, and physical links
	ifaces, err := db.GetInterfaces(deviceID)
	if err != nil {
		return topo, err
	}

	// Build parentID → []child interface name map
	parentChildren := make(map[string][]string) // parentID → child interface names
	for _, iface := range ifaces {
		if iface.ParentID != "" {
			parentChildren[iface.ParentID] = append(parentChildren[iface.ParentID], iface.Name)
		}
	}

	// Build interface ID → interface map for LAG lookup
	ifaceByID := make(map[string]model.Interface)
	for _, iface := range ifaces {
		ifaceByID[iface.ID] = iface
	}

	var physLinks []PhysicalLink

	for _, iface := range ifaces {
		switch iface.Type {
		case model.IfTypeEthTrunk:
			lag := LAGBundle{
				Name:        iface.Name,
				IP:          iface.IPAddress,
				Mask:        iface.Mask,
				Description: iface.Description,
			}
			// Resolve children by parentID = iface.ID
			if children, ok := parentChildren[iface.ID]; ok {
				lag.Members = children
			}
			topo.LAGs = append(topo.LAGs, lag)

		case model.IfTypePhysical:
			// Only include physical interfaces with an IP that are not trunk members
			if iface.IPAddress != "" && iface.ParentID == "" {
				physLinks = append(physLinks, PhysicalLink{
					Interface:   iface.Name,
					IP:          iface.IPAddress,
					Mask:        iface.Mask,
					Description: iface.Description,
				})
			}
		}
	}

	// Step 5: match BGP peer IPs to physical link interfaces
	// Build a set of interface names that are LAG members
	lagMemberIfaces := make(map[string]bool)
	for _, iface := range ifaces {
		if iface.ParentID != "" {
			lagMemberIfaces[iface.Name] = true
		}
	}

	for i := range physLinks {
		for pgName, pg := range peerGroupMap {
			for j := range pg.Peers {
				if pg.Peers[j].Interface == "" &&
					sameSubnet(physLinks[i].IP, pg.Peers[j].PeerIP, physLinks[i].Mask) {
					pg.Peers[j].Interface = physLinks[i].Interface
					if physLinks[i].PeerGroup == "" {
						physLinks[i].PeerGroup = pgName
					}
				}
			}
		}
	}
	topo.PhysicalLinks = physLinks

	// Step 6: scan config snapshots for protocol keywords and static routes
	protocolSet := make(map[string]bool)
	if len(peerGroupMap) > 0 {
		protocolSet["bgp"] = true
	}

	configSnapshots, _ := db.GetConfigSnapshots(deviceID)
	for _, cs := range configSnapshots {
		text := cs.ConfigText
		lower := strings.ToLower(text)
		if strings.Contains(lower, "ospf ") || strings.Contains(lower, "router ospf") {
			protocolSet["ospf"] = true
		}
		if strings.Contains(lower, "isis ") {
			protocolSet["isis"] = true
		}
		if strings.Contains(lower, "mpls ldp") {
			protocolSet["ldp"] = true
		}

		// Extract static routes
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "ip route-static ") {
				sr := parseStaticRoute(trimmed)
				if sr.Prefix != "" {
					topo.StaticRoutes = append(topo.StaticRoutes, sr)
				}
			}
		}
	}

	// Collect protocols in stable order
	for _, proto := range []string{"ospf", "isis", "ldp", "bgp"} {
		if protocolSet[proto] {
			topo.Protocols = append(topo.Protocols, proto)
		}
	}

	// Step 7: get VRF instances
	vrfs, _ := db.GetVRFInstances(deviceID)
	for _, v := range vrfs {
		topo.VRFs = append(topo.VRFs, VRFSummary{Name: v.VRFName, RD: v.RD})
	}

	// Step 8: infer PeerGroup roles and collect into topo.PeerGroups
	for _, pgName := range peerGroupOrder {
		pg := peerGroupMap[pgName]
		pg.Role = inferRole(pg, lagMemberIfaces)
		topo.PeerGroups = append(topo.PeerGroups, *pg)
	}

	// Step 9: graph analysis — SPOF and impact devices
	g, err := graph.BuildFromDB(db)
	if err == nil {
		// Check if device is a SPOF
		spofs := graph.FindSPOF(g, graph.NodeTypeDevice)
		for _, s := range spofs {
			if s == deviceID {
				topo.IsSPOF = true
				break
			}
		}

		// Impact analysis
		affected := graph.ImpactAnalysis(g, deviceID, graph.NodeTypeDevice)
		for _, affectedID := range affected {
			node, ok := g.GetNode(affectedID)
			if ok {
				if hostname := node.Props["hostname"]; hostname != "" {
					topo.ImpactDevices = append(topo.ImpactDevices, hostname)
				} else {
					topo.ImpactDevices = append(topo.ImpactDevices, affectedID)
				}
			}
		}
	}

	return topo, nil
}

// inferRole determines the role of a peer group based on type, name, and interface membership.
func inferRole(pg *PeerGroup, lagMemberIfaces map[string]bool) PeerGroupRole {
	if pg.Type == "internal" {
		return RoleManagement
	}
	nameLower := strings.ToLower(pg.Name)
	if strings.Contains(nameLower, "sdn") || strings.Contains(nameLower, "controller") {
		return RoleManagement
	}
	// Check if any peer's matched interface is a LAG member
	for _, p := range pg.Peers {
		if p.Interface != "" && lagMemberIfaces[p.Interface] {
			return RoleUplink
		}
	}
	return RoleDownlink
}

// parseStaticRoute extracts prefix, next-hop, interface, vrf from an "ip route-static" line.
// Huawei VRP format: ip route-static [vpn-instance <vrf>] <prefix> <mask> <nexthop|interface>
func parseStaticRoute(line string) StaticRouteEntry {
	// Strip the "ip route-static " prefix
	rest := strings.TrimPrefix(line, "ip route-static ")
	rest = strings.TrimSpace(rest)

	var sr StaticRouteEntry

	// Check for vpn-instance keyword
	if strings.HasPrefix(rest, "vpn-instance ") {
		rest = strings.TrimPrefix(rest, "vpn-instance ")
		parts := strings.Fields(rest)
		if len(parts) < 1 {
			return sr
		}
		sr.VRF = parts[0]
		rest = strings.Join(parts[1:], " ")
	}

	fields := strings.Fields(rest)
	// Expect: <dest-prefix> <mask> <nexthop-or-interface> [options...]
	if len(fields) < 3 {
		return sr
	}

	sr.Prefix = fields[0] + "/" + fields[1]
	// Third field is either next-hop IP or interface name
	third := fields[2]
	// Heuristic: if it looks like an IP address, it's a nexthop; otherwise it's an interface
	if looksLikeIP(third) {
		sr.NextHop = third
	} else {
		sr.Interface = third
		if len(fields) >= 4 && looksLikeIP(fields[3]) {
			sr.NextHop = fields[3]
		}
	}
	return sr
}

// looksLikeIP does a quick check for an IPv4 address pattern.
func looksLikeIP(s string) bool {
	dots := 0
	for _, c := range s {
		if c == '.' {
			dots++
		} else if c < '0' || c > '9' {
			return false
		}
	}
	return dots == 3
}

// sameSubnet returns true if ip1 and ip2 share the same network address under the given mask length.
// mask is a string prefix length like "30".
func sameSubnet(ip1, ip2, mask string) bool {
	net1 := networkAddr(ip1, mask)
	net2 := networkAddr(ip2, mask)
	if net1 == "" || net2 == "" {
		return false
	}
	return net1 == net2
}

// networkAddr computes the network address of an IP given a prefix length string.
func networkAddr(ip, mask string) string {
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

	return strings.Join([]string{
		itoa(int(ipBytes[0] & maskBytes[0])),
		itoa(int(ipBytes[1] & maskBytes[1])),
		itoa(int(ipBytes[2] & maskBytes[2])),
		itoa(int(ipBytes[3] & maskBytes[3])),
	}, ".")
}

// itoa converts an int to its decimal string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [10]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
