package model

type TunnelInfo struct {
	ID           int    `json:"id"`
	DeviceID     string `json:"device_id"`
	TunnelName   string `json:"tunnel_name"`
	Type         string `json:"type"`
	Destination  string `json:"destination"`
	State        string `json:"state"`
	SignaledBW   string `json:"signaled_bw"`
	ExplicitPath string `json:"explicit_path"`
	ActualPath   string `json:"actual_path"`
	BindingSID   int    `json:"binding_sid"`
	SnapshotID   int    `json:"snapshot_id"`
}

type SRMapping struct {
	ID         int    `json:"id"`
	DeviceID   string `json:"device_id"`
	Prefix     string `json:"prefix"`
	SIDIndex   int    `json:"sid_index"`
	SIDLabel   int    `json:"sid_label"`
	Algorithm  int    `json:"algorithm"`
	Flags      string `json:"flags"`
	Source     string `json:"source"`
	SnapshotID int    `json:"snapshot_id"`
}
