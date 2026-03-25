package parser

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	"github.com/xavierli/nethelper/internal/store"
)

// Collector captures CommandBlocks with CmdUnknown type into unknown_outputs.
// All errors are logged and swallowed — never fails the pipeline.
type Collector struct {
	db *store.DB
}

// NewCollector creates a Collector. If db is nil, Collect is a no-op.
func NewCollector(db *store.DB) *Collector {
	return &Collector{db: db}
}

// Collect records an unknown block. Safe to call from the pipeline.
func (c *Collector) Collect(block CommandBlock) error {
	if c.db == nil {
		return nil
	}
	norm := normaliseCommand(block.Vendor, block.Command)
	hash := hashContent(block.Output)

	entry := store.UnknownOutput{
		DeviceID:    block.Hostname,
		Vendor:      block.Vendor,
		CommandRaw:  block.Command,
		CommandNorm: norm,
		RawOutput:   block.Output,
		ContentHash: hash,
	}
	if err := c.db.UpsertUnknownOutput(entry); err != nil {
		slog.Warn("collector: failed to upsert unknown output", "cmd", block.Command, "error", err)
	}
	return nil
}

// normaliseCommand expands the leading verb abbreviation then lowercases and
// collapses whitespace. Interior abbreviations (e.g. "int") are NOT expanded.
// "dis int brief" → "display int brief" (huawei/h3c)
// "sh ip route"   → "show ip route"    (cisco)
func normaliseCommand(vendor, cmd string) string {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch vendor {
	case "huawei", "h3c":
		if strings.HasPrefix(lower, "dis ") && !strings.HasPrefix(lower, "display ") {
			lower = "display " + lower[4:]
		}
	case "cisco":
		if strings.HasPrefix(lower, "sh ") && !strings.HasPrefix(lower, "show ") {
			lower = "show " + lower[3:]
		}
	}
	return strings.Join(strings.Fields(lower), " ")
}

// hashContent returns the first 16 hex chars of SHA256(s).
func hashContent(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:8])
}
