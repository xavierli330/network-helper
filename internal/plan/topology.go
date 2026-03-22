package plan

import "github.com/xavierli/nethelper/internal/store"

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
	Name  string
	Type  string        // "external" / "internal"
	Role  PeerGroupRole
	Peers []BGPPeerDetail
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

// BuildTopology stub — implemented in Task 2
func BuildTopology(db *store.DB, deviceID string) (DeviceTopology, error) {
	return DeviceTopology{}, nil
}
