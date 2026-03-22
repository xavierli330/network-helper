package plan

// CommandGenerator produces ordered CLI commands for each phase of a device isolation plan.
type CommandGenerator interface {
	CollectionCommands(target string, links []Link) []DeviceCommand
	PreCheckCommands(target string, links []Link) []DeviceCommand
	ProtocolIsolateCommands(target string, links []Link) []DeviceCommand
	InterfaceIsolateCommands(target string, links []Link) []DeviceCommand
	PostCheckCommands(target string, links []Link) []DeviceCommand
	RollbackCommands(target string, links []Link) []DeviceCommand
}

// GeneratorForVendor returns the appropriate CommandGenerator for the given vendor string.
// Unknown vendors fall back to HuaweiGenerator.
func GeneratorForVendor(vendor string) CommandGenerator {
	switch vendor {
	case "h3c":
		return &H3CGenerator{}
	default:
		return &HuaweiGenerator{}
	}
}

// collectProtocols returns a set of all distinct protocols referenced across all links.
func collectProtocols(links []Link) map[string]bool {
	protos := make(map[string]bool)
	for _, l := range links {
		for _, p := range l.Protocols {
			protos[p] = true
		}
	}
	return protos
}

// uniquePeers returns a deduplicated, ordered list of peer device IDs from links.
func uniquePeers(links []Link) []string {
	seen := make(map[string]bool)
	var peers []string
	for _, l := range links {
		if l.PeerDevice != "" && !seen[l.PeerDevice] {
			seen[l.PeerDevice] = true
			peers = append(peers, l.PeerDevice)
		}
	}
	return peers
}
