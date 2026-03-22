package plan

import "time"

// Link represents a discovered interconnection between two devices.
type Link struct {
	LocalDevice    string   // local device ID (lowercase slug)
	LocalInterface string   // local interface name (resolved to parent for trunk members)
	LocalIP        string   // local IP address
	PeerDevice     string   // peer device ID
	PeerInterface  string   // peer interface name (may be empty)
	PeerIP         string   // peer IP (may be empty)
	Protocols      []string // inferred protocols: "ospf", "bgp", "ldp"
	Sources        []string // discovery source: "description", "subnet", "config"
	VRF            string   // VRF name, empty = global
}

// DeviceCommand is an ordered list of CLI commands to run on one device.
type DeviceCommand struct {
	DeviceID   string   // device ID (lowercase slug)
	DeviceHost string   // display hostname
	Vendor     string   // "huawei", "h3c", etc.
	Commands   []string // ordered CLI commands
	Purpose    string   // human-readable purpose
}

// Phase is one stage of the isolation change plan.
type Phase struct {
	Number      int             // 0-5
	Name        string          // e.g. "方案规划"
	Description string          // what this phase does
	Steps       []DeviceCommand // commands grouped by device
	Notes       []string        // warnings, wait instructions
}

// Plan is a complete device isolation change plan.
type Plan struct {
	TargetDevice   string    // target device ID
	TargetHostname string    // target device hostname
	TargetVendor   string    // target device vendor
	Links          []Link    // discovered interconnections
	IsSPOF         bool      // whether target is a single point of failure
	ImpactDevices  []string  // device hostnames that would be isolated
	Phases         []Phase   // ordered phases (0-5)
	GeneratedAt    time.Time // when plan was generated
}

// PreCheckResult holds the baseline state from Phase 1 pre-check.
type PreCheckResult struct {
	DeviceID       string   `json:"device_id"`
	OSPFPeerCount  int      `json:"ospf_peer_count"`
	OSPFAllFull    bool     `json:"ospf_all_full"`
	BGPPeerCount   int      `json:"bgp_peer_count"`
	BGPAllEstab    bool     `json:"bgp_all_established"`
	InterfaceUp    int      `json:"interface_up_count"`
	InterfaceTotal int      `json:"interface_total"`
	Safe           bool     `json:"safe"`
	Warnings       []string `json:"warnings,omitempty"`
}
