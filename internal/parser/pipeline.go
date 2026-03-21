package parser

import (
	"log/slog"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/store"
)

// IngestResult summarises what the pipeline processed.
type IngestResult struct {
	DevicesFound  int
	BlocksParsed  int
	BlocksFailed  int
	BlocksSkipped int
}

// Pipeline orchestrates split → detect → parse → store.
type Pipeline struct {
	db       *store.DB
	registry *Registry
}

// NewPipeline creates a Pipeline backed by the given DB and vendor registry.
func NewPipeline(db *store.DB, registry *Registry) *Pipeline {
	return &Pipeline{db: db, registry: registry}
}

// Ingest splits raw CLI output into command blocks, parses each one,
// and persists the results into the database.
func (p *Pipeline) Ingest(sourceFile, content string) (IngestResult, error) {
	var result IngestResult

	blocks := Split(content, p.registry)
	if len(blocks) == 0 {
		return result, nil
	}

	// Step 1: Initial ClassifyCommand pass using the vendor assigned by Split.
	for i := range blocks {
		b := &blocks[i]
		if strings.HasSuffix(strings.TrimRight(b.Command, " \t"), "?") {
			continue // skip help queries; counted below
		}
		if vp, ok := p.registry.Get(b.Vendor); ok {
			b.CmdType = vp.ClassifyCommand(b.Command)
		} else {
			b.CmdType = model.CmdUnknown
		}
	}

	// Step 2: Post-split reclassification — detect H3C devices misclassified as
	// Huawei (both share the <hostname> prompt format). Re-classifies commands
	// for any reclassified blocks using the H3C parser.
	reclassifyH3C(blocks, p.registry)

	// Step 3: Build per-device groups from the (possibly corrected) blocks.
	type deviceBlocks struct {
		hostname string
		vendor   string
		blocks   []CommandBlock
	}
	deviceMap := make(map[string]*deviceBlocks)
	for i := range blocks {
		b := &blocks[i]
		// Skip help queries — commands ending with '?' produce CLI help text, not useful data.
		if strings.HasSuffix(strings.TrimRight(b.Command, " \t"), "?") {
			result.BlocksSkipped++
			continue
		}
		key := strings.ToLower(b.Hostname)
		if _, exists := deviceMap[key]; !exists {
			deviceMap[key] = &deviceBlocks{hostname: b.Hostname, vendor: b.Vendor}
		}
		deviceMap[key].blocks = append(deviceMap[key].blocks, *b)
	}

	result.DevicesFound = len(deviceMap)

	for deviceID, db := range deviceMap {
		dev := model.Device{
			ID:       deviceID,
			Hostname: db.hostname,
			Vendor:   db.vendor,
			LastSeen: time.Now(),
		}
		if err := p.db.UpsertDevice(dev); err != nil {
			slog.Error("upsert device failed", "device", deviceID, "error", err)
			continue
		}

		var cmdNames []string
		var earliestAt time.Time
		for _, b := range db.blocks {
			cmdNames = append(cmdNames, b.Command)
			if !b.CapturedAt.IsZero() {
				if earliestAt.IsZero() || b.CapturedAt.Before(earliestAt) {
					earliestAt = b.CapturedAt
				}
			}
		}
		snapshot := model.Snapshot{
			DeviceID:   deviceID,
			SourceFile: sourceFile,
			Commands:   `["` + strings.Join(cmdNames, `","`) + `"]`,
			CapturedAt: earliestAt,
		}
		snapID, err := p.db.CreateSnapshot(snapshot)
		if err != nil {
			slog.Error("create snapshot failed", "device", deviceID, "error", err)
			continue
		}

		for _, b := range db.blocks {
			vp, ok := p.registry.Get(b.Vendor)
			if !ok {
				result.BlocksSkipped++
				continue
			}

			// Full-table commands (RIB/FIB/LFIB) → scratch pad instead of structured storage.
			// They're too large for permanent storage and not practical to show in full.
			// Specific object queries (e.g., "dis ip route 10.0.0.1") are small enough → also scratch.
			if isBulkTableCommand(b.CmdType) {
				category := cmdTypeToScratchCategory(b.CmdType)
				p.db.InsertScratch(model.ScratchEntry{
					DeviceID: deviceID, Category: category,
					Query: b.Command, Content: b.Output,
				})
				result.BlocksParsed++
				continue
			}

			parseResult, err := vp.ParseOutput(b.CmdType, b.Output)
			if err != nil {
				slog.Warn("parse failed, storing raw", "cmd", b.Command, "error", err)
				result.BlocksFailed++
				continue
			}

			if err := p.storeResult(deviceID, snapID, parseResult); err != nil {
				slog.Error("store result failed", "cmd", b.Command, "error", err)
				result.BlocksFailed++
				continue
			}
			result.BlocksParsed++
		}
	}
	return result, nil
}

func (p *Pipeline) storeResult(deviceID string, snapID int, pr model.ParseResult) error {
	for i := range pr.Interfaces {
		iface := &pr.Interfaces[i]
		iface.DeviceID = deviceID
		if iface.ID == "" {
			iface.ID = deviceID + ":" + iface.Name
		}
		iface.LastUpdated = time.Now()
		if err := p.db.UpsertInterface(*iface); err != nil {
			return err
		}
	}
	if len(pr.RIBEntries) > 0 {
		for i := range pr.RIBEntries {
			pr.RIBEntries[i].DeviceID = deviceID
			pr.RIBEntries[i].SnapshotID = snapID
		}
		if err := p.db.InsertRIBEntries(pr.RIBEntries); err != nil {
			return err
		}
	}
	if len(pr.FIBEntries) > 0 {
		for i := range pr.FIBEntries {
			pr.FIBEntries[i].DeviceID = deviceID
			pr.FIBEntries[i].SnapshotID = snapID
		}
		if err := p.db.InsertFIBEntries(pr.FIBEntries); err != nil {
			return err
		}
	}
	if len(pr.LFIBEntries) > 0 {
		for i := range pr.LFIBEntries {
			pr.LFIBEntries[i].DeviceID = deviceID
			pr.LFIBEntries[i].SnapshotID = snapID
		}
		if err := p.db.InsertLFIBEntries(pr.LFIBEntries); err != nil {
			return err
		}
	}
	if len(pr.Neighbors) > 0 {
		for i := range pr.Neighbors {
			pr.Neighbors[i].DeviceID = deviceID
			pr.Neighbors[i].SnapshotID = snapID
		}
		if err := p.db.InsertNeighbors(pr.Neighbors); err != nil {
			return err
		}
	}
	if len(pr.Tunnels) > 0 {
		for i := range pr.Tunnels {
			pr.Tunnels[i].DeviceID = deviceID
			pr.Tunnels[i].SnapshotID = snapID
		}
		if err := p.db.InsertTunnels(pr.Tunnels); err != nil {
			return err
		}
	}
	if len(pr.SRMappings) > 0 {
		for i := range pr.SRMappings {
			pr.SRMappings[i].DeviceID = deviceID
			pr.SRMappings[i].SnapshotID = snapID
		}
		if err := p.db.InsertSRMappings(pr.SRMappings); err != nil {
			return err
		}
	}

	// Store config snapshots
	if pr.ConfigText != "" {
		format := "hierarchical"
		if pr.Type == model.CmdConfigSet {
			format = "set"
		}
		cs := model.ConfigSnapshot{
			DeviceID:   deviceID,
			ConfigText: pr.ConfigText,
			SourceFile: "", // will be set by caller if needed
			Format:     format,
		}
		// Compute diff from previous config
		prevConfigs, _ := p.db.GetConfigSnapshots(deviceID)
		if len(prevConfigs) > 0 {
			// Simple diff indicator — full diff available via 'nethelper diff config'
			if prevConfigs[0].ConfigText != pr.ConfigText {
				cs.DiffFromPrev = "changed"
			} else {
				cs.DiffFromPrev = "no change"
			}
		}
		if _, err := p.db.InsertConfigSnapshot(cs); err != nil {
			return err
		}
	}

	return nil
}

// isBulkTableCommand returns true for commands that produce large table outputs
// (full routing/forwarding/label tables) which should go to scratch pad.
func isBulkTableCommand(cmdType model.CommandType) bool {
	switch cmdType {
	case model.CmdRIB, model.CmdFIB, model.CmdLFIB:
		return true
	default:
		return false
	}
}

func cmdTypeToScratchCategory(cmdType model.CommandType) string {
	switch cmdType {
	case model.CmdRIB:
		return "route"
	case model.CmdFIB:
		return "fib"
	case model.CmdLFIB:
		return "label"
	default:
		return "raw"
	}
}

// reclassifyH3C detects H3C devices that were initially assigned vendor="huawei"
// because both platforms share the <hostname> prompt format, and Huawei is
// registered first. It scans config output for H3C-specific signatures:
//   - "version 7." prefix  (H3C Comware 7 header)
//   - "mdc admin id"       (H3C MDC marker)
//
// When a signature is found in any block for a hostname, every block belonging
// to that hostname is flipped to vendor="h3c" and its CmdType is re-classified
// using the H3C parser.
func reclassifyH3C(blocks []CommandBlock, reg *Registry) {
	h3cHostnames := map[string]bool{}
	for _, b := range blocks {
		if b.Vendor != "huawei" {
			continue
		}
		lines := strings.SplitN(b.Output, "\n", 10)
		for _, l := range lines {
			lower := strings.ToLower(strings.TrimSpace(l))
			if strings.HasPrefix(lower, "version 7.") || strings.HasPrefix(lower, "mdc admin id") {
				h3cHostnames[strings.ToLower(b.Hostname)] = true
				break
			}
		}
	}
	if len(h3cHostnames) == 0 {
		return
	}
	h3cParser, ok := reg.Get("h3c")
	for i := range blocks {
		if h3cHostnames[strings.ToLower(blocks[i].Hostname)] {
			blocks[i].Vendor = "h3c"
			if ok {
				blocks[i].CmdType = h3cParser.ClassifyCommand(blocks[i].Command)
			}
		}
	}
}
