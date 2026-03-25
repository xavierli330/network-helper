package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/store"
)

const maxSamplesPerGroup = 5

// CommandGroup holds a normalised command and representative raw output samples.
type CommandGroup struct {
	Vendor           string
	CommandNorm      string
	Samples          []store.UnknownOutput
	TotalOccurrences int // sum of occurrence_count across all samples in this group
}

// LLMRuleResponse is the structured JSON response from the LLM.
type LLMRuleResponse struct {
	OutputType  string  `json:"output_type"`
	SchemaYAML  string  `json:"schema_yaml"`
	GoCodeDraft string  `json:"go_code_draft"`
	Confidence  float64 `json:"confidence"`
}

// Engine orchestrates clustering and LLM analysis.
type Engine struct {
	db     *store.DB
	router *llm.Router
}

// New creates a discovery Engine.
func New(db *store.DB, router *llm.Router) *Engine {
	return &Engine{db: db, router: router}
}

// ClusterByCommand groups unknown outputs by (vendor, command_norm) and selects
// representative samples. Exported for testing.
func ClusterByCommand(db *store.DB, vendor string) []CommandGroup {
	rows, err := db.ListUnknownOutputs(vendor, "new", 1000)
	if err != nil {
		return nil
	}
	groupMap := make(map[string]*CommandGroup)
	for _, row := range rows {
		key := row.Vendor + "\x00" + row.CommandNorm
		if _, ok := groupMap[key]; !ok {
			groupMap[key] = &CommandGroup{Vendor: row.Vendor, CommandNorm: row.CommandNorm}
		}
		g := groupMap[key]
		g.TotalOccurrences += row.OccurrenceCount
		if len(g.Samples) < maxSamplesPerGroup {
			g.Samples = append(g.Samples, row)
		}
	}
	groups := make([]CommandGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	return groups
}

// RunOnce clusters new unknown outputs for vendor and calls LLM to generate drafts.
// vendor="" processes all vendors.
func (e *Engine) RunOnce(ctx context.Context, vendor string) (int, error) {
	groups := ClusterByCommand(e.db, vendor)
	created := 0
	for _, g := range groups {
		_, err := e.db.GetPendingRuleByCommandNorm(g.Vendor, g.CommandNorm)
		if err != sql.ErrNoRows {
			continue // already has a draft
		}

		resp, err := e.callLLM(ctx, g)
		if err != nil {
			slog.Warn("discovery: LLM call failed", "cmd", g.CommandNorm, "error", err)
			continue
		}

		samples := make([]string, len(g.Samples))
		for i, s := range g.Samples {
			samples[i] = s.RawOutput
		}
		samplesJSON, _ := json.Marshal(samples)

		_, err = e.db.CreatePendingRule(store.PendingRule{
			Vendor:          g.Vendor,
			CommandPattern:  g.CommandNorm,
			OutputType:      resp.OutputType,
			SchemaYAML:      resp.SchemaYAML,
			GoCodeDraft:     resp.GoCodeDraft,
			SampleInputs:    string(samplesJSON),
			Confidence:      resp.Confidence,
			OccurrenceCount: g.TotalOccurrences,
			Status:          "draft",
		})
		if err != nil {
			slog.Warn("discovery: create rule failed", "cmd", g.CommandNorm, "error", err)
			continue
		}
		e.db.UpdateUnknownOutputStatus(g.Vendor, g.CommandNorm, "clustered")
		created++
	}
	return created, nil
}

func (e *Engine) callLLM(ctx context.Context, g CommandGroup) (LLMRuleResponse, error) {
	var sb strings.Builder
	for i, s := range g.Samples {
		fmt.Fprintf(&sb, "--- Sample %d ---\n%s\n\n", i+1, s.RawOutput)
	}

	system := `You are a network CLI output parser generator.
Analyse the provided samples and return JSON only (no markdown):
{
  "output_type": "table" | "hierarchical" | "raw",
  "schema_yaml": "<YAML for table type only>",
  "go_code_draft": "<Go function body for hierarchical/raw>",
  "confidence": 0.0-1.0
}
For table: schema_yaml has header_pattern, skip_lines, columns[].
For hierarchical/raw: go_code_draft is the body of func parseXxx(raw string) (model.ParseResult, error).`

	userMsg := fmt.Sprintf("Vendor: %s\nCommand: %s\n\n%s", g.Vendor, g.CommandNorm, sb.String())

	text, err := func() (string, error) {
		resp, err := e.router.Chat(ctx, llm.CapParse, llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "system", Content: system},
				{Role: "user", Content: userMsg},
			},
		})
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}()
	if err != nil {
		return LLMRuleResponse{}, err
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) > 2 {
			text = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	var resp LLMRuleResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return LLMRuleResponse{}, fmt.Errorf("parse LLM response: %w", err)
	}
	if resp.OutputType == "" {
		resp.OutputType = "raw"
	}
	return resp, nil
}

// GenerateForUnknown generates a pending rule draft for a single unknown output by ID.
// If a rule already exists for the same (vendor, command_norm), returns the existing rule ID.
// Returns the pending rule ID (new or existing).
func (e *Engine) GenerateForUnknown(ctx context.Context, unknownID int) (int, error) {
	u, err := e.db.GetUnknownOutputByID(unknownID)
	if err != nil {
		return 0, fmt.Errorf("load unknown output %d: %w", unknownID, err)
	}

	// Check for existing draft/testing rule for this (vendor, command_norm).
	existing, err := e.db.GetPendingRuleByCommandNorm(u.Vendor, u.CommandNorm)
	if err == nil {
		// Rule already exists — return existing ID without re-calling LLM.
		return existing.ID, nil
	}

	group := CommandGroup{
		Vendor:           u.Vendor,
		CommandNorm:      u.CommandNorm,
		Samples:          []store.UnknownOutput{u},
		TotalOccurrences: u.OccurrenceCount,
	}

	resp, err := e.callLLM(ctx, group)
	if err != nil {
		return 0, fmt.Errorf("LLM call for unknown %d: %w", unknownID, err)
	}

	samplesJSON, _ := json.Marshal([]string{u.RawOutput})
	ruleID, err := e.db.CreatePendingRule(store.PendingRule{
		Vendor:          u.Vendor,
		CommandPattern:  u.CommandNorm,
		OutputType:      resp.OutputType,
		SchemaYAML:      resp.SchemaYAML,
		GoCodeDraft:     resp.GoCodeDraft,
		SampleInputs:    string(samplesJSON),
		Confidence:      resp.Confidence,
		OccurrenceCount: u.OccurrenceCount,
		Status:          "draft",
	})
	if err != nil {
		return 0, fmt.Errorf("create pending rule: %w", err)
	}

	e.db.UpdateUnknownOutputStatus(u.Vendor, u.CommandNorm, "clustered")
	return ruleID, nil
}
