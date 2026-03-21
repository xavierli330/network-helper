package model

type NeighborInfo struct {
	ID             int    `json:"id"`
	DeviceID       string `json:"device_id"`
	Protocol       string `json:"protocol"`
	LocalID        string `json:"local_id"`
	RemoteID       string `json:"remote_id"`
	LocalInterface string `json:"local_interface"`
	RemoteAddress  string `json:"remote_address"`
	State          string `json:"state"`
	AreaID         string `json:"area_id"`
	ASNumber       int    `json:"as_number"`
	Uptime         string `json:"uptime"`
	SnapshotID     int    `json:"snapshot_id"`
}
