package model

import "fmt"

type RIBEntry struct {
	ID                int    `json:"id"`
	DeviceID          string `json:"device_id"`
	VRF               string `json:"vrf"`
	Prefix            string `json:"prefix"`
	MaskLen           int    `json:"mask_len"`
	Protocol          string `json:"protocol"`
	NextHop           string `json:"next_hop"`
	OutgoingInterface string `json:"outgoing_interface"`
	Preference        int    `json:"preference"`
	Metric            int    `json:"metric"`
	Tag               int    `json:"tag"`
	SnapshotID        int    `json:"snapshot_id"`
}

func (r RIBEntry) PrefixString() string {
	return fmt.Sprintf("%s/%d", r.Prefix, r.MaskLen)
}

type FIBEntry struct {
	ID                int    `json:"id"`
	DeviceID          string `json:"device_id"`
	VRF               string `json:"vrf"`
	Prefix            string `json:"prefix"`
	MaskLen           int    `json:"mask_len"`
	NextHop           string `json:"next_hop"`
	OutgoingInterface string `json:"outgoing_interface"`
	LabelAction       string `json:"label_action"`
	OutLabel          string `json:"out_label"`
	TunnelID          string `json:"tunnel_id"`
	SnapshotID        int    `json:"snapshot_id"`
}

func (f FIBEntry) PrefixString() string {
	return fmt.Sprintf("%s/%d", f.Prefix, f.MaskLen)
}

type LFIBEntry struct {
	ID                int    `json:"id"`
	DeviceID          string `json:"device_id"`
	InLabel           int    `json:"in_label"`
	Action            string `json:"action"`
	OutLabel          string `json:"out_label"`
	NextHop           string `json:"next_hop"`
	OutgoingInterface string `json:"outgoing_interface"`
	FEC               string `json:"fec"`
	Protocol          string `json:"protocol"`
	SnapshotID        int    `json:"snapshot_id"`
}
