package model

import "time"

type InterfaceType string

const (
	IfTypePhysical     InterfaceType = "physical"
	IfTypeLoopback     InterfaceType = "loopback"
	IfTypeVlanif       InterfaceType = "vlanif"
	IfTypeEthTrunk     InterfaceType = "eth-trunk"
	IfTypeTunnelTE     InterfaceType = "tunnel-te"
	IfTypeTunnelSR     InterfaceType = "tunnel-sr"
	IfTypeTunnelGRE    InterfaceType = "tunnel-gre"
	IfTypeNVE          InterfaceType = "nve"
	IfTypeNull         InterfaceType = "null"
	IfTypeSubInterface InterfaceType = "sub-interface"
)

var validInterfaceTypes = map[InterfaceType]bool{
	IfTypePhysical: true, IfTypeLoopback: true, IfTypeVlanif: true,
	IfTypeEthTrunk: true, IfTypeTunnelTE: true, IfTypeTunnelSR: true,
	IfTypeTunnelGRE: true, IfTypeNVE: true, IfTypeNull: true,
	IfTypeSubInterface: true,
}

func (t InterfaceType) Valid() bool {
	return validInterfaceTypes[t]
}

type Device struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	Vendor    string    `json:"vendor"`
	Model     string    `json:"model"`
	OSVersion string    `json:"os_version"`
	MgmtIP    string    `json:"mgmt_ip"`
	RouterID  string    `json:"router_id"`
	MPLSLsrID string    `json:"mpls_lsr_id"`
	LastSeen  time.Time `json:"last_seen"`
}

type Interface struct {
	ID          string        `json:"id"`
	DeviceID    string        `json:"device_id"`
	Name        string        `json:"name"`
	Type        InterfaceType `json:"type"`
	Status      string        `json:"status"`
	IPAddress   string        `json:"ip_address"`
	Mask        string        `json:"mask"`
	VLAN        int           `json:"vlan"`
	Bandwidth   string        `json:"bandwidth"`
	Description string        `json:"description"`
	ParentID    string        `json:"parent_id"`
	LastUpdated time.Time     `json:"last_updated"`
}
