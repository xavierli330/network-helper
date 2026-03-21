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

	// Group blocks by hostname.
	type deviceBlocks struct {
		hostname string
		vendor   string
		blocks   []CommandBlock
	}
	deviceMap := make(map[string]*deviceBlocks)

	for i := range blocks {
		b := &blocks[i]
		if vp, ok := p.registry.Get(b.Vendor); ok {
			b.CmdType = vp.ClassifyCommand(b.Command)
		} else {
			b.CmdType = model.CmdUnknown
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
		for _, b := range db.blocks {
			cmdNames = append(cmdNames, b.Command)
		}
		snapshot := model.Snapshot{
			DeviceID:   deviceID,
			SourceFile: sourceFile,
			Commands:   `["` + strings.Join(cmdNames, `","`) + `"]`,
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
	return nil
}
