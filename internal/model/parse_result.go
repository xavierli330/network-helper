package model

type CommandType string

const (
	CmdRIB       CommandType = "rib"
	CmdFIB       CommandType = "fib"
	CmdLFIB      CommandType = "lfib"
	CmdInterface CommandType = "interface"
	CmdNeighbor  CommandType = "neighbor"
	CmdTunnel    CommandType = "tunnel"
	CmdSRMapping CommandType = "sr_mapping"
	CmdConfig    CommandType = "config"
	CmdUnknown   CommandType = "unknown"
)

type ParseResult struct {
	Type        CommandType    `json:"type"`
	Interfaces  []Interface    `json:"interfaces,omitempty"`
	RIBEntries  []RIBEntry     `json:"rib_entries,omitempty"`
	FIBEntries  []FIBEntry     `json:"fib_entries,omitempty"`
	LFIBEntries []LFIBEntry    `json:"lfib_entries,omitempty"`
	Neighbors   []NeighborInfo `json:"neighbors,omitempty"`
	Tunnels     []TunnelInfo   `json:"tunnels,omitempty"`
	SRMappings  []SRMapping    `json:"sr_mappings,omitempty"`
	ConfigText  string         `json:"config_text,omitempty"`
	RawText     string         `json:"raw_text"`
}

func (pr ParseResult) IsEmpty() bool {
	return len(pr.Interfaces) == 0 &&
		len(pr.RIBEntries) == 0 &&
		len(pr.FIBEntries) == 0 &&
		len(pr.LFIBEntries) == 0 &&
		len(pr.Neighbors) == 0 &&
		len(pr.Tunnels) == 0 &&
		len(pr.SRMappings) == 0
}
