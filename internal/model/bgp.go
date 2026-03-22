package model

// BGPPeer represents a BGP neighbor extracted from running-config,
// including per-address-family policy bindings.
type BGPPeer struct {
	ID                 int    `json:"id"`
	DeviceID           string `json:"device_id"`
	VRF                string `json:"vrf"`
	LocalAS            int    `json:"local_as"`
	PeerIP             string `json:"peer_ip"`
	RemoteAS           int    `json:"remote_as"`
	PeerGroup          string `json:"peer_group"`
	Description        string `json:"description"`
	UpdateSource       string `json:"update_source"`
	EBGPMultihop       int    `json:"ebgp_multihop"`
	BFDEnabled         int    `json:"bfd_enabled"`
	Shutdown           int    `json:"shutdown"`
	AddressFamily      string `json:"address_family"`
	ImportPolicy       string `json:"import_policy"`
	ExportPolicy       string `json:"export_policy"`
	AdvertiseCommunity int    `json:"advertise_community"`
	NextHopSelf        int    `json:"next_hop_self"`
	SoftReconfig       int    `json:"soft_reconfig"`
	Enabled            int    `json:"enabled"`
	SnapshotID         int    `json:"snapshot_id"`
}

// VRFInstance represents a VPN instance / VRF extracted from running-config.
type VRFInstance struct {
	ID            int    `json:"id"`
	DeviceID      string `json:"device_id"`
	VRFName       string `json:"vrf_name"`
	RD            string `json:"rd"`
	ImportRT      string `json:"import_rt"`  // JSON array
	ExportRT      string `json:"export_rt"`  // JSON array
	ImportPolicy  string `json:"import_policy"`
	ExportPolicy  string `json:"export_policy"`
	TunnelPolicy  string `json:"tunnel_policy"`
	LabelMode     string `json:"label_mode"`
	AddressFamily string `json:"address_family"`
	SnapshotID    int    `json:"snapshot_id"`
}

// RoutePolicy represents a route-policy / policy-statement header.
type RoutePolicy struct {
	ID         int               `json:"id"`
	DeviceID   string            `json:"device_id"`
	PolicyName string            `json:"policy_name"`
	VendorType string            `json:"vendor_type"`
	RawText    string            `json:"raw_text"`
	Nodes      []RoutePolicyNode `json:"nodes,omitempty"`
	SnapshotID int               `json:"snapshot_id"`
}

// RoutePolicyNode represents a single node/term in a route-policy.
type RoutePolicyNode struct {
	ID           int    `json:"id"`
	PolicyID     int    `json:"policy_id"`
	Sequence     int    `json:"sequence"`
	TermName     string `json:"term_name"`
	Action       string `json:"action"`
	MatchClauses string `json:"match_clauses"` // JSON array
	ApplyClauses string `json:"apply_clauses"` // JSON array
}
