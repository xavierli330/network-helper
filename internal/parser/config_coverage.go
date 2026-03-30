package parser

import (
	"strings"
	"time"
)

// CoverageStatus indicates whether a command is recognized by the parser.
type CoverageStatus string

const (
	CoverageCovered   CoverageStatus = "covered"   // ClassifyCommand returns non-unknown
	CoverageUncovered CoverageStatus = "uncovered" // ClassifyCommand returns CmdUnknown
)

// CoverageItem represents one row of a coverage check result.
type CoverageItem struct {
	Command    string         `json:"command"`
	Category   string         `json:"category"`
	Reason     string         `json:"reason"`
	Priority   string         `json:"priority"`
	Status     CoverageStatus `json:"status"`
	CmdType    string         `json:"cmd_type"`    // resolved CommandType or "unknown"
}

// CoverageReport summarises a self-check run for a single device.
type CoverageReport struct {
	DeviceID    string         `json:"device_id"`
	Vendor      string         `json:"vendor"`
	CheckedAt   time.Time      `json:"checked_at"`
	Items       []CoverageItem `json:"items"`
	TotalCount  int            `json:"total_count"`
	CoveredCount int           `json:"covered_count"`
	CoveragePct  float64       `json:"coverage_pct"` // 0–100
}

// CheckCoverage runs the self-check engine for a device:
//  1. Infer which commands the device should support from its config.
//  2. For each inferred command, check ClassifyCommand to see if it's recognized.
//  3. Return a structured report.
func CheckCoverage(vendor, configText string, registry *Registry) *CoverageReport {
	inferred := InferCommands(vendor, configText)
	if len(inferred) == 0 {
		return nil
	}

	vp, ok := registry.Get(vendor)
	if !ok {
		return nil
	}

	report := &CoverageReport{
		Vendor:    vendor,
		CheckedAt: time.Now(),
	}

	for _, ic := range inferred {
		cmdType := vp.ClassifyCommand(ic.Command)
		status := CoverageCovered
		if string(cmdType) == "unknown" {
			status = CoverageUncovered
		}
		report.Items = append(report.Items, CoverageItem{
			Command:  ic.Command,
			Category: ic.Category,
			Reason:   ic.Reason,
			Priority: ic.Priority,
			Status:   status,
			CmdType:  string(cmdType),
		})
	}

	report.TotalCount = len(report.Items)
	for _, item := range report.Items {
		if item.Status == CoverageCovered {
			report.CoveredCount++
		}
	}
	if report.TotalCount > 0 {
		report.CoveragePct = float64(report.CoveredCount) / float64(report.TotalCount) * 100
	}

	return report
}

// GenerateSSHScript produces a pasteable SSH command list for uncovered commands.
// The output is vendor-appropriate and can be directly pasted into a terminal session.
func GenerateSSHScript(report *CoverageReport) string {
	if report == nil {
		return ""
	}

	var uncovered []CoverageItem
	for _, item := range report.Items {
		if item.Status == CoverageUncovered {
			uncovered = append(uncovered, item)
		}
	}

	if len(uncovered) == 0 {
		return "# All inferred commands are covered — no additional commands needed.\n"
	}

	var sb strings.Builder
	sb.WriteString("# ────────────────────────────────────────────────\n")
	sb.WriteString("# nethelper Self-Check: Uncovered Commands\n")
	sb.WriteString("# Device vendor: " + report.Vendor + "\n")
	sb.WriteString("# Paste the following commands into the device CLI\n")
	sb.WriteString("# to collect data for parser coverage expansion.\n")
	sb.WriteString("# ────────────────────────────────────────────────\n\n")

	// Group by category
	catMap := make(map[string][]CoverageItem)
	var catOrder []string
	for _, item := range uncovered {
		if _, exists := catMap[item.Category]; !exists {
			catOrder = append(catOrder, item.Category)
		}
		catMap[item.Category] = append(catMap[item.Category], item)
	}

	for _, cat := range catOrder {
		items := catMap[cat]
		sb.WriteString("# --- " + cat + " ---\n")
		for _, item := range items {
			sb.WriteString(item.Command + "\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
