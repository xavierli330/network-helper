package parser

import (
	"strings"

	"github.com/xavierli/nethelper/internal/model"
)

// AnalyzedCommand represents a single command extracted from a session log,
// annotated with metadata for human review. Unlike the normal pipeline,
// nothing is filtered — everything is preserved and flagged.
type AnalyzedCommand struct {
	Raw             string `json:"raw"`               // original command text from log
	Normalized      string `json:"normalized"`         // verb-expanded, lowercased
	Pattern         string `json:"pattern"`            // with args replaced by placeholders
	Vendor          string `json:"vendor"`             // detected vendor
	Hostname        string `json:"hostname"`           // detected hostname
	Category        string `json:"category"`           // guessed category (bgp/ospf/acl/…)
	HasOutput       bool   `json:"has_output"`         // non-empty output
	OutputLines     int    `json:"output_lines"`       // line count
	OutputPreview   string `json:"output_preview"`     // first N chars of output
	FullOutput      string `json:"full_output"`        // complete output text
	IsControl       bool   `json:"is_control"`         // navigation/management command
	IsHelp          bool   `json:"is_help"`            // help/completion echo
	IsError         bool   `json:"is_error"`           // CLI error output
	HasExistingRule bool   `json:"has_existing_rule"`  // already covered by a parser
	ExistingCmdType string `json:"existing_cmd_type"`  // the matched CommandType (if any)
	Selected        bool   `json:"selected"`           // AI recommends generating a rule
}

// LogAnalysis is the result of analyzing a session log file.
type LogAnalysis struct {
	Vendor   string            `json:"vendor"`
	Hostname string            `json:"hostname"`
	Total    int               `json:"total_blocks"`
	Commands []AnalyzedCommand `json:"commands"`
}

// AnalyzeSessionLog splits a session log and returns a structured command list.
// Unlike the normal pipeline, it does NOT filter aggressively — it preserves
// all commands and marks them with metadata for human review.
//
// Key design principle: 只标记不过滤 (mark, don't filter).
// The filtering decision is left to the user via check/uncheck in the UI.
func AnalyzeSessionLog(reg *Registry, raw string) (*LogAnalysis, error) {
	if strings.TrimSpace(raw) == "" {
		return &LogAnalysis{}, nil
	}

	// 1. Split into CommandBlocks (reuse existing splitter)
	blocks := Split(raw, reg)
	if len(blocks) == 0 {
		return &LogAnalysis{}, nil
	}

	// 2. For each block, classify but DON'T filter
	var commands []AnalyzedCommand
	for _, b := range blocks {
		trimmedOutput := strings.TrimSpace(b.Output)

		cmd := AnalyzedCommand{
			Raw:           b.Command,
			Vendor:        b.Vendor,
			Hostname:      b.Hostname,
			HasOutput:     trimmedOutput != "",
			OutputLines:   countLines(trimmedOutput),
			OutputPreview: truncateStr(trimmedOutput, 200),
			FullOutput:    b.Output,
		}

		// Normalize command
		cmd.Normalized = NormaliseCommand(b.Vendor, b.Command)

		// Strip args to get pattern
		cmd.Pattern = StripArgs(cmd.Normalized)

		// Mark control commands (but don't skip them)
		cmd.IsControl = IsControlCommand(b.Vendor, b.Command)

		// Mark help/error output
		if trimmedOutput != "" {
			cmd.IsHelp = IsHelpEcho(trimmedOutput)
			cmd.IsError = IsErrorOutput(trimmedOutput)
		}

		// Check if existing parser rule covers this
		if reg != nil {
			if p, ok := reg.Get(b.Vendor); ok {
				cmdType := p.ClassifyCommand(b.Command)
				cmd.HasExistingRule = cmdType != model.CmdUnknown
				cmd.ExistingCmdType = string(cmdType)
			}
		}

		// Auto-select: has output + not control + not help/error + no existing rule
		cmd.Selected = cmd.HasOutput && !cmd.IsControl && !cmd.IsHelp && !cmd.IsError && !cmd.HasExistingRule

		// Guess category from command keywords
		cmd.Category = guessCategory(cmd.Normalized)

		commands = append(commands, cmd)
	}

	// Deduplicate by pattern, keeping the longest output per pattern
	commands = deduplicateCommands(commands)

	result := &LogAnalysis{
		Vendor:   blocks[0].Vendor,
		Hostname: blocks[0].Hostname,
		Total:    len(commands),
		Commands: commands,
	}
	return result, nil
}

// guessCategory infers a high-level category from command keywords.
func guessCategory(normalized string) string {
	lower := strings.ToLower(normalized)
	switch {
	case strings.Contains(lower, "bgp"):
		return "bgp"
	case strings.Contains(lower, "ospf"):
		return "ospf"
	case strings.Contains(lower, "isis"):
		return "isis"
	case strings.Contains(lower, "mpls") || strings.Contains(lower, "ldp") || strings.Contains(lower, "rsvp"):
		return "mpls"
	case strings.Contains(lower, "acl") || strings.Contains(lower, "access-list"):
		return "acl"
	case strings.Contains(lower, "interface") || strings.Contains(lower, "eth-trunk") ||
		strings.Contains(lower, "link-aggregation") || strings.Contains(lower, "bridge-aggregation"):
		return "interface"
	case strings.Contains(lower, "vlan"):
		return "vlan"
	case strings.Contains(lower, "vpn-instance") || strings.Contains(lower, "vrf"):
		return "vpn"
	case strings.Contains(lower, "routing-table") || strings.Contains(lower, "route"):
		return "routing"
	case strings.Contains(lower, "segment-routing") || strings.Contains(lower, "srv6"):
		return "sr"
	case strings.Contains(lower, "configuration") || strings.Contains(lower, "running-config"):
		return "config"
	case strings.Contains(lower, "tunnel"):
		return "tunnel"
	case strings.Contains(lower, "neighbor") || strings.Contains(lower, "lldp"):
		return "neighbor"
	case strings.Contains(lower, "fib") || strings.Contains(lower, "forwarding"):
		return "fib"
	case strings.Contains(lower, "policy") || strings.Contains(lower, "route-policy") ||
		strings.Contains(lower, "route-map") || strings.Contains(lower, "prefix-list"):
		return "policy"
	default:
		return "other"
	}
}

// deduplicateCommands groups commands by pattern and keeps the one with the
// longest output for each pattern. Commands without output are kept individually.
func deduplicateCommands(commands []AnalyzedCommand) []AnalyzedCommand {
	type entry struct {
		cmd   AnalyzedCommand
		index int // original order
	}
	seen := make(map[string]*entry)
	var order []string

	for i, cmd := range commands {
		key := cmd.Vendor + "\x00" + cmd.Pattern

		if existing, ok := seen[key]; ok {
			// Keep the one with longer output (more useful for rule generation)
			if len(cmd.FullOutput) > len(existing.cmd.FullOutput) {
				existing.cmd = cmd
			}
			// Merge selection: if either thinks it should be selected, keep selected
			if cmd.Selected {
				existing.cmd.Selected = true
			}
		} else {
			seen[key] = &entry{cmd: cmd, index: i}
			order = append(order, key)
		}
	}

	result := make([]AnalyzedCommand, 0, len(order))
	for _, key := range order {
		result = append(result, seen[key].cmd)
	}
	return result
}

// countLines counts non-empty lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// truncateStr truncates a string to at most n bytes.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
