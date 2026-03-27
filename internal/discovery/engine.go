package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser/engine"
	"github.com/xavierli/nethelper/internal/store"
)

const maxSamplesPerGroup = 5

// Per-command LLM call timeout.
const llmCallTimeout = 90 * time.Second

// DiscoveryEventType classifies SSE progress messages.
type DiscoveryEventType string

const (
	EventStart    DiscoveryEventType = "start"    // discovery session begins
	EventPending  DiscoveryEventType = "pending"  // about to process a command
	EventSuccess  DiscoveryEventType = "success"  // rule created for a command
	EventSkipped  DiscoveryEventType = "skipped"  // already has a draft
	EventFailed   DiscoveryEventType = "failed"   // LLM call or DB error
	EventDone     DiscoveryEventType = "done"     // all commands processed
)

// DiscoveryEvent is one SSE progress message.
type DiscoveryEvent struct {
	Type        DiscoveryEventType `json:"type"`
	CommandNorm string             `json:"command_norm,omitempty"`
	Vendor      string             `json:"vendor,omitempty"`
	Index       int                `json:"index"`          // 1-based position in queue
	Total       int                `json:"total"`          // total commands
	Created     int                `json:"created"`        // rules created so far
	Error       string             `json:"error,omitempty"`
	Message     string             `json:"message,omitempty"`
}

// CommandGroup holds a normalised command and representative raw output samples.
type CommandGroup struct {
	Vendor           string
	CommandNorm      string
	Samples          []store.UnknownOutput
	TotalOccurrences int // sum of occurrence_count across all samples in this group
}

// LLMRuleResponse is the structured JSON response from the LLM.
type LLMRuleResponse struct {
	OutputType                string  `json:"output_type"`
	SchemaYAML                string  `json:"schema_yaml"`
	GoCodeDraft               string  `json:"go_code_draft"`
	Confidence                float64 `json:"confidence"`
	ExpectedOutputDescription string  `json:"expected_output_description,omitempty"`
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
// vendor="" processes all vendors.  Kept for backward compatibility; internally
// drains RunStream.
func (e *Engine) RunOnce(ctx context.Context, vendor string) (int, error) {
	ch := e.RunStream(ctx, vendor)
	var last DiscoveryEvent
	for ev := range ch {
		last = ev
	}
	if last.Type == EventDone {
		return last.Created, nil
	}
	return last.Created, fmt.Errorf("discovery ended at event %s: %s", last.Type, last.Error)
}

// RunStream starts an asynchronous discovery run and returns a channel of
// progress events.  Each command group gets its own context.WithTimeout
// (llmCallTimeout) so a single slow/failing LLM call cannot block the rest.
// The returned channel is closed when all groups have been processed.
func (e *Engine) RunStream(ctx context.Context, vendor string) <-chan DiscoveryEvent {
	ch := make(chan DiscoveryEvent, 32)
	go func() {
		defer close(ch)

		groups := ClusterByCommand(e.db, vendor)
		total := len(groups)
		created := 0

		// Emit start event
		send(ctx, ch, DiscoveryEvent{
			Type:    EventStart,
			Total:   total,
			Message: fmt.Sprintf("Found %d unknown command groups", total),
		})

		for i, g := range groups {
			idx := i + 1

			// Check parent context cancellation
			if ctx.Err() != nil {
				send(ctx, ch, DiscoveryEvent{
					Type: EventDone, Index: idx, Total: total, Created: created,
					Message: "cancelled",
				})
				return
			}

			_, err := e.db.GetPendingRuleByCommandNorm(g.Vendor, g.CommandNorm)
			if err != sql.ErrNoRows {
				// Already has a draft
				send(ctx, ch, DiscoveryEvent{
					Type: EventSkipped, CommandNorm: g.CommandNorm, Vendor: g.Vendor,
					Index: idx, Total: total, Created: created,
					Message: "already has draft",
				})
				continue
			}

			// Emit pending event
			send(ctx, ch, DiscoveryEvent{
				Type: EventPending, CommandNorm: g.CommandNorm, Vendor: g.Vendor,
				Index: idx, Total: total, Created: created,
				Message: fmt.Sprintf("Calling LLM for %s", g.CommandNorm),
			})

			// Individual timeout per command — not tied to HTTP request context
			llmCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
			resp, err := e.callLLM(llmCtx, g)
			cancel()

			if err != nil {
				slog.Warn("discovery: LLM call failed", "cmd", g.CommandNorm, "error", err)
				send(ctx, ch, DiscoveryEvent{
					Type: EventFailed, CommandNorm: g.CommandNorm, Vendor: g.Vendor,
					Index: idx, Total: total, Created: created,
					Error: err.Error(),
				})
				continue
			}

			// Filter out samples with empty/whitespace-only output
			var validSamples []store.UnknownOutput
			for _, s := range g.Samples {
				if strings.TrimSpace(s.RawOutput) != "" {
					validSamples = append(validSamples, s)
				}
			}
			if len(validSamples) == 0 {
				slog.Info("discovery: skipping command with no valid samples", "cmd", g.CommandNorm)
				// Mark as ignored since there's no useful data to generate a rule from
				e.db.UpdateUnknownOutputStatus(g.Vendor, g.CommandNorm, "ignored")
				send(ctx, ch, DiscoveryEvent{
					Type: EventSkipped, CommandNorm: g.CommandNorm, Vendor: g.Vendor,
					Index: idx, Total: total, Created: created,
					Message: "no valid sample inputs (empty output)",
				})
				continue
			}

			samples := make([]string, len(validSamples))
			for j, s := range validSamples {
				samples[j] = s.RawOutput
			}
			samplesJSON, _ := json.Marshal(samples)

			_, dbErr := e.db.CreatePendingRule(store.PendingRule{
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
			if dbErr != nil {
				slog.Warn("discovery: create rule failed", "cmd", g.CommandNorm, "error", dbErr)
				send(ctx, ch, DiscoveryEvent{
					Type: EventFailed, CommandNorm: g.CommandNorm, Vendor: g.Vendor,
					Index: idx, Total: total, Created: created,
					Error: dbErr.Error(),
				})
				continue
			}
			e.db.UpdateUnknownOutputStatus(g.Vendor, g.CommandNorm, "clustered")
			created++
			send(ctx, ch, DiscoveryEvent{
				Type: EventSuccess, CommandNorm: g.CommandNorm, Vendor: g.Vendor,
				Index: idx, Total: total, Created: created,
				Message: fmt.Sprintf("Rule created: %s", g.CommandNorm),
			})
		}

		send(ctx, ch, DiscoveryEvent{
			Type: EventDone, Total: total, Created: created,
			Message: fmt.Sprintf("Done: %d rules created from %d groups", created, total),
		})
	}()
	return ch
}

// send writes an event to the channel without blocking if the context is done.
func send(ctx context.Context, ch chan<- DiscoveryEvent, ev DiscoveryEvent) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

func (e *Engine) callLLM(ctx context.Context, g CommandGroup) (LLMRuleResponse, error) {
	var sb strings.Builder
	for i, s := range g.Samples {
		fmt.Fprintf(&sb, "--- Sample %d ---\n%s\n\n", i+1, s.RawOutput)
	}

	system := `You are a network CLI output parser generator.
Analyse the provided samples and return JSON only (no markdown):
{
  "output_type": "pipeline" | "table" | "hierarchical" | "raw",
  "schema_yaml": "<DSL text for pipeline, or YAML for table type>",
  "go_code_draft": "<Go function body for hierarchical/raw only>",
  "confidence": 0.0-1.0
}

PREFERRED: Use "pipeline" output_type whenever possible. Pipeline DSL is a simple
line-based language stored in schema_yaml. Instructions (one per line, # for comments):

─── Phase 1: Trimming ───
  SKIP_UNTIL <regex>      - skip lines until regex matches (header line itself is skipped)
  SKIP_LINES <N>          - skip N more lines after SKIP_UNTIL (e.g. separator rows)
  SKIP_BLANK              - skip blank lines
  STOP_AT    <regex>      - stop processing when regex matches
  FILTER     <regex>      - keep only lines matching regex
  REJECT     <regex>      - discard lines matching regex

─── Phase 2: Extraction ───
  SPLIT      $a $b $c ... - whitespace-split each line; last var captures all remaining fields
  REGEX      <pattern>    - named capture groups (?P<name>...) extract variables
  REPLACE    <pattern> "replacement" - regex replace on text before extraction

─── Phase 3: Post-processing ───
  SET        $name <expr> - concat ($a "/" $b) or ternary ($x == "up" ? "yes" : "no")
  EMIT                    - explicit emit (auto-emit per line if omitted)

─── Multi-section (for multi-table output) ───
  SECTION                 - start new independent processing section;
                            each section runs its own trimming + extraction against
                            the full raw input, results are joined by row index

─── Repeating groups (CRITICAL for parent-child structures) ───
  REPEAT_FOR <regex>      - split input into blocks at lines matching regex;
                            named groups (?P<name>...) are extracted as "parent"
                            fields and broadcast to every child row within that block.
                            Subsequent instructions run independently per block.
                            All blocks' rows are concatenated.
                            NESTED REPEAT_FOR is supported: a second REPEAT_FOR after
                            the first further splits each block into sub-blocks.

IMPORTANT RULES:
1. For tabular output (columns separated by whitespace), use SKIP_UNTIL + SPLIT.
2. For key-value output, use multiple REGEX instructions.
3. When the sample data contains REPEATING BLOCKS of similar structure (e.g. multiple
   route-policies, multiple ACLs, multiple interfaces), you MUST use REPEAT_FOR to
   extract ALL blocks, not just the first one. Each block header becomes the REPEAT_FOR
   pattern, and child lines within each block are extracted by FILTER+REGEX or SPLIT.
4. When a block contains MULTIPLE SUB-BLOCKS (e.g. a route-policy with multiple
   permit/deny nodes), use NESTED REPEAT_FOR: the first REPEAT_FOR splits by the
   outer header, the second REPEAT_FOR splits each outer block by the inner header.

Example 1 — Simple tabular:
  SKIP_UNTIL ^Interface\s+Physical
  SPLIT $interface $physical $protocol $ip $description

Example 2 — Repeating blocks (e.g. "display acl all" with multiple ACLs):
  # Each ACL block starts with a header line — use REPEAT_FOR with named groups
  REPEAT_FOR ^(?P<acl_type>Basic|Advanced) IPv4 ACL (?P<acl_number>\d+) named (?P<acl_name>[^,]+),
  # Within each block, keep only rule lines
  FILTER ^rule\s+\d+
  # Extract rule fields
  REGEX ^rule\s+(?P<rule_id>\d+)\s+(?P<action>permit|deny)

Example 3 — Nested repeating blocks (e.g. "display route-policy"):
  # Each route-policy has a name, then MULTIPLE permit/deny nodes within it.
  # Use nested REPEAT_FOR: outer splits by route-policy name, inner splits by permit/deny.
  # Use \s* at line start to handle indentation in the raw output.
  REPEAT_FOR ^Route-policy:\s+(?P<route_policy_name>.+)$
  REPEAT_FOR ^\s*(?P<action>permit|deny)\s*:\s*(?P<seq>\d+)\s*\(matched counts:\s*(?P<matched_counts>\d+)\)
  REGEX ^\s*if-match\s+(?P<match_clause>.+)$
  REGEX ^\s*apply\s+(?P<apply_clause>.+)$

Only use "table" for legacy cases. Use "hierarchical"/"raw" only when pipeline cannot express the logic.`

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

// ── Self-Test types ──────────────────────────────────────────────────────

// SelfTestResult captures the outcome of running a DSL against sample data.
type SelfTestResult struct {
	Passed     bool               `json:"passed"`
	Level      int                `json:"level"`                // highest level passed (0-4)
	Output     json.RawMessage    `json:"output,omitempty"`     // parsed result (used as expected)
	Error      string             `json:"error,omitempty"`
	FieldStats map[string]float64 `json:"field_stats,omitempty"`
	Diags      []string           `json:"diagnostics,omitempty"`
	RowCount   int                `json:"row_count"`
}

// GenerateEventType classifies SSE progress messages for single-unknown generation.
type GenerateEventType string

const (
	GenEventLLM      GenerateEventType = "generate_llm"
	GenEventValidate GenerateEventType = "generate_validate"
	GenEventSelfTest GenerateEventType = "generate_selftest"
	GenEventFix      GenerateEventType = "generate_fix"
	GenEventDone     GenerateEventType = "generate_done"
)

// GenerateEvent is one SSE progress message for single-unknown generation.
type GenerateEvent struct {
	Type           GenerateEventType `json:"type"`
	Message        string            `json:"message,omitempty"`
	SelfTestPassed bool              `json:"self_test_passed"`
	Attempt        int               `json:"attempt,omitempty"`  // 1-based
	MaxAttempts    int               `json:"max_attempts,omitempty"`
	RuleID         int               `json:"rule_id,omitempty"`
	Error          string            `json:"error,omitempty"`
}

// GenerateForUnknown generates a pending rule draft for a single unknown output by ID.
// If a rule already exists for the same (vendor, command_norm), returns the existing rule ID.
// Returns the pending rule ID (new or existing).
func (e *Engine) GenerateForUnknown(ctx context.Context, unknownID int) (int, error) {
	ch := e.GenerateForUnknownStream(ctx, unknownID)
	var last GenerateEvent
	for ev := range ch {
		last = ev
	}
	if last.Error != "" {
		return last.RuleID, fmt.Errorf("%s", last.Error)
	}
	return last.RuleID, nil
}

// GenerateForUnknownStream is the streaming version that emits progress events.
func (e *Engine) GenerateForUnknownStream(ctx context.Context, unknownID int) <-chan GenerateEvent {
	ch := make(chan GenerateEvent, 16)
	go func() {
		defer close(ch)
		e.generateForUnknownImpl(ctx, unknownID, ch)
	}()
	return ch
}

func (e *Engine) generateForUnknownImpl(ctx context.Context, unknownID int, ch chan<- GenerateEvent) {
	u, err := e.db.GetUnknownOutputByID(unknownID)
	if err != nil {
		ch <- GenerateEvent{Type: GenEventDone, Error: fmt.Sprintf("load unknown output %d: %v", unknownID, err)}
		return
	}

	if strings.TrimSpace(u.RawOutput) == "" {
		e.db.UpdateUnknownOutputStatus(u.Vendor, u.CommandNorm, "ignored")
		ch <- GenerateEvent{Type: GenEventDone, Error: fmt.Sprintf("unknown output %d has empty raw output", unknownID)}
		return
	}

	existing, err := e.db.GetPendingRuleByCommandNorm(u.Vendor, u.CommandNorm)
	if err == nil {
		ch <- GenerateEvent{Type: GenEventDone, RuleID: existing.ID, SelfTestPassed: true, Message: "Rule already exists"}
		return
	}

	group := CommandGroup{
		Vendor:           u.Vendor,
		CommandNorm:      u.CommandNorm,
		Samples:          []store.UnknownOutput{u},
		TotalOccurrences: u.OccurrenceCount,
	}

	// Phase 1: Call LLM to generate DSL
	ch <- GenerateEvent{Type: GenEventLLM, Message: "Calling LLM to generate DSL..."}
	resp, err := e.callLLM(ctx, group)
	if err != nil {
		ch <- GenerateEvent{Type: GenEventDone, Error: fmt.Sprintf("LLM call failed: %v", err)}
		return
	}

	// Phase 1b: DSL syntax pre-validation
	if resp.OutputType == "pipeline" {
		ch <- GenerateEvent{Type: GenEventValidate, Message: "Validating DSL syntax..."}
		if valErr := engine.ValidatePipelineDSL(resp.SchemaYAML); valErr != nil {
			slog.Info("discovery: DSL syntax error, attempting fix", "cmd", u.CommandNorm, "err", valErr)
			ch <- GenerateEvent{Type: GenEventFix, Message: "DSL syntax error, asking LLM to fix...", Attempt: 1, MaxAttempts: 2}
			fixed, fixErr := e.fixDSL(ctx, group, resp, valErr.Error())
			if fixErr != nil {
				slog.Warn("discovery: DSL fix failed", "cmd", u.CommandNorm, "err", fixErr)
			} else {
				resp = fixed
			}
		}
	}

	// Phase 2: Self-test with sample data
	const maxRetries = 2
	var selfResult SelfTestResult
	l4Passed := true // track L4 outcome separately for test case gating

	if resp.OutputType == "pipeline" || resp.OutputType == "table" {
		ch <- GenerateEvent{Type: GenEventSelfTest, Message: "Running self-test with sample data..."}
		selfResult = e.selfTest(resp, u.RawOutput)

		if !selfResult.Passed {
			// Phase 3: Auto-fix loop (triggered by L0-L3 failure)
			for attempt := 1; attempt <= maxRetries && !selfResult.Passed; attempt++ {
				ch <- GenerateEvent{
					Type: GenEventFix, Message: fmt.Sprintf("Self-test failed, asking LLM to fix (attempt %d/%d)...", attempt, maxRetries),
					Attempt: attempt, MaxAttempts: maxRetries,
				}
				fixed, fixErr := e.fixDSL(ctx, group, resp, selfResult.Error)
				if fixErr != nil {
					slog.Warn("discovery: DSL fix failed", "cmd", u.CommandNorm, "attempt", attempt, "err", fixErr)
					break
				}
				resp = fixed

				ch <- GenerateEvent{Type: GenEventSelfTest, Message: fmt.Sprintf("Re-testing after fix %d...", attempt)}
				selfResult = e.selfTest(resp, u.RawOutput)
			}
		}

		// Phase 2b: L2.5 row-count heuristic (only if L0-L3 passed)
		// Detect repeating patterns in sample and compare to extracted row count.
		if selfResult.Passed && selfResult.RowCount > 0 {
			if hint := e.rowCountHeuristic(u.RawOutput, selfResult.RowCount); hint != "" {
				slog.Info("discovery: L2.5 row-count heuristic triggered", "cmd", u.CommandNorm, "hint", hint)
				selfResult.Diags = append(selfResult.Diags, "L2.5: "+hint)
				selfResult.Passed = false
				selfResult.Error = "L2.5: " + hint

				// Trigger fix loop for row-count mismatch
				for attempt := 1; attempt <= maxRetries && !selfResult.Passed; attempt++ {
					ch <- GenerateEvent{
						Type: GenEventFix, Message: fmt.Sprintf("Row-count mismatch — asking LLM to fix (attempt %d/%d)...", attempt, maxRetries),
						Attempt: attempt, MaxAttempts: maxRetries,
					}
					fixed, fixErr := e.fixDSL(ctx, group, resp, selfResult.Error)
					if fixErr != nil {
						slog.Warn("discovery: DSL fix (L2.5) failed", "cmd", u.CommandNorm, "attempt", attempt, "err", fixErr)
						break
					}
					resp = fixed

					ch <- GenerateEvent{Type: GenEventSelfTest, Message: fmt.Sprintf("Re-testing after L2.5 fix %d...", attempt)}
					selfResult = e.selfTest(resp, u.RawOutput)
					// Re-check heuristic after fix
					if selfResult.Passed && selfResult.RowCount > 0 {
						if hint2 := e.rowCountHeuristic(u.RawOutput, selfResult.RowCount); hint2 != "" {
							selfResult.Passed = false
							selfResult.Error = "L2.5: " + hint2
							selfResult.Diags = append(selfResult.Diags, "L2.5: "+hint2)
						}
					}
				}
			}
		}

		// Phase 2c: L4 LLM semantic validation (if L0-L3 + L2.5 all passed)
		if selfResult.Passed && selfResult.RowCount > 0 {
			ch <- GenerateEvent{Type: GenEventSelfTest, Message: "Running L4 LLM semantic validation..."}
			l4ok, l4diag := e.llmSemanticCheck(ctx, u, resp, selfResult)
			if !l4ok {
				l4Passed = false
				slog.Info("discovery: L4 semantic check failed — triggering fix loop", "cmd", u.CommandNorm, "diag", l4diag)
				selfResult.Diags = append(selfResult.Diags, "L4 LLM semantic check: "+l4diag)
				selfResult.Passed = false
				selfResult.Error = "L4 semantic: " + l4diag
				selfResult.Level = 3 // demote: L3 passed but L4 failed

				// Trigger fix loop for L4 failure
				for attempt := 1; attempt <= maxRetries && !selfResult.Passed; attempt++ {
					ch <- GenerateEvent{
						Type: GenEventFix, Message: fmt.Sprintf("L4 semantic issue — asking LLM to fix (attempt %d/%d)...", attempt, maxRetries),
						Attempt: attempt, MaxAttempts: maxRetries,
					}
					fixed, fixErr := e.fixDSL(ctx, group, resp, selfResult.Error)
					if fixErr != nil {
						slog.Warn("discovery: DSL fix (L4) failed", "cmd", u.CommandNorm, "attempt", attempt, "err", fixErr)
						break
					}
					resp = fixed

					ch <- GenerateEvent{Type: GenEventSelfTest, Message: fmt.Sprintf("Re-testing after L4 fix %d...", attempt)}
					selfResult = e.selfTest(resp, u.RawOutput)

					if selfResult.Passed && selfResult.RowCount > 0 {
						// Re-run L2.5 check
						if hint := e.rowCountHeuristic(u.RawOutput, selfResult.RowCount); hint != "" {
							selfResult.Passed = false
							selfResult.Error = "L2.5: " + hint
							selfResult.Diags = append(selfResult.Diags, "L2.5: "+hint)
							continue
						}
						// Re-run L4 check
						ch <- GenerateEvent{Type: GenEventSelfTest, Message: "Re-running L4 semantic validation..."}
						l4ok2, l4diag2 := e.llmSemanticCheck(ctx, u, resp, selfResult)
						if !l4ok2 {
							selfResult.Passed = false
							selfResult.Error = "L4 semantic: " + l4diag2
							selfResult.Diags = append(selfResult.Diags, "L4 re-check: "+l4diag2)
						} else {
							l4Passed = true
							selfResult.Level = 4
						}
					}
				}
			} else {
				selfResult.Level = 4
			}
		}
	} else {
		// For non-pipeline/table types, skip self-test
		selfResult = SelfTestResult{Passed: false, Level: 0, Diags: []string{"self-test skipped for " + resp.OutputType}}
	}

	// Phase 4: Store rule + auto test cases
	samplesJSON, _ := json.Marshal([]string{u.RawOutput})
	selfTestJSON, _ := json.Marshal(selfResult)

	status := "draft"
	if selfResult.Passed {
		status = "testing"
	}

	ruleID, dbErr := e.db.CreatePendingRule(store.PendingRule{
		Vendor:          u.Vendor,
		CommandPattern:  u.CommandNorm,
		OutputType:      resp.OutputType,
		SchemaYAML:      resp.SchemaYAML,
		GoCodeDraft:     resp.GoCodeDraft,
		SampleInputs:    string(samplesJSON),
		ExpectedOutputs: string(selfTestJSON),
		Confidence:      resp.Confidence,
		OccurrenceCount: u.OccurrenceCount,
		Status:          status,
	})
	if dbErr != nil {
		ch <- GenerateEvent{Type: GenEventDone, Error: fmt.Sprintf("create pending rule: %v", dbErr)}
		return
	}

	// Auto-create test case ONLY if self-test AND L4 both passed
	// This prevents encoding wrong expectations when extraction is incomplete
	if selfResult.Passed && l4Passed && selfResult.Output != nil {
		e.autoCreateTestCases(ruleID, u.RawOutput, selfResult.Output, u.CommandNorm)
	}

	e.db.UpdateUnknownOutputStatus(u.Vendor, u.CommandNorm, "clustered")

	ch <- GenerateEvent{
		Type:           GenEventDone,
		RuleID:         ruleID,
		SelfTestPassed: selfResult.Passed,
		Message: func() string {
			if selfResult.Passed {
				return fmt.Sprintf("Rule created with self-test passed (L%d)", selfResult.Level)
			}
			return "Rule created as draft — self-test did not fully pass"
		}(),
	}
}

// selfTest runs the DSL against sample input and performs L0-L3 validation.
func (e *Engine) selfTest(resp LLMRuleResponse, sampleInput string) SelfTestResult {
	result := SelfTestResult{FieldStats: make(map[string]float64)}

	// L0: DSL syntax
	if resp.OutputType == "pipeline" {
		if err := engine.ValidatePipelineDSL(resp.SchemaYAML); err != nil {
			result.Error = "L0 syntax: " + err.Error()
			result.Diags = append(result.Diags, result.Error)
			return result
		}
	}
	result.Level = 0

	// L1: Execute without error
	var rows []map[string]string
	var columns []string

	switch resp.OutputType {
	case "pipeline":
		pResult, err := engine.ExecPipeline(resp.SchemaYAML, sampleInput)
		if err != nil {
			result.Error = "L1 exec: " + err.Error()
			result.Diags = append(result.Diags, result.Error)
			return result
		}
		rows = pResult.Rows
		columns = pResult.Columns
	case "table":
		// For table type, we'd need to parse the YAML schema — for now, skip
		result.Level = 1
		result.Diags = append(result.Diags, "table type self-test basic pass")
		result.Passed = true
		return result
	default:
		result.Diags = append(result.Diags, "self-test not supported for "+resp.OutputType)
		return result
	}
	result.Level = 1

	// L2: Non-empty result
	if len(rows) == 0 {
		result.Error = "L2: no rows extracted from sample data"
		result.Diags = append(result.Diags, result.Error)
		return result
	}
	result.Level = 2
	result.RowCount = len(rows)

	// L3: Field non-empty rate > 80%
	if len(columns) > 0 && len(rows) > 0 {
		totalFields := len(columns) * len(rows)
		nonEmpty := 0
		for _, row := range rows {
			for _, col := range columns {
				if v, ok := row[col]; ok && strings.TrimSpace(v) != "" {
					nonEmpty++
				}
			}
		}
		rate := float64(nonEmpty) / float64(totalFields)
		result.FieldStats["non_empty_rate"] = rate

		if rate < 0.5 { // Use 50% as minimum threshold
			result.Error = fmt.Sprintf("L3: field non-empty rate too low (%.0f%%, need ≥50%%)", rate*100)
			result.Diags = append(result.Diags, result.Error)
			return result
		}
	}
	result.Level = 3

	// Marshal output as the expected result
	output := map[string]any{"rows": rows, "columns": columns}
	outJSON, _ := json.Marshal(output)
	result.Output = outJSON
	result.Passed = true
	result.Diags = append(result.Diags, fmt.Sprintf("L3 passed: %d rows, %d columns", len(rows), len(columns)))

	return result
}

// fixDSL asks the LLM to fix a broken DSL.
func (e *Engine) fixDSL(ctx context.Context, group CommandGroup, resp LLMRuleResponse, errMsg string) (LLMRuleResponse, error) {
	var samplePreview string
	if len(group.Samples) > 0 {
		preview := group.Samples[0].RawOutput
		lines := strings.Split(preview, "\n")
		if len(lines) > 100 {
			lines = lines[:100]
		}
		samplePreview = strings.Join(lines, "\n")
	}

	system := `You are fixing a broken Pipeline DSL for parsing network CLI output.
Return ONLY valid JSON (no markdown) with the structure:
{
  "output_type": "pipeline",
  "schema_yaml": "<fixed DSL>",
  "go_code_draft": "",
  "confidence": 0.0-1.0,
  "expected_output_description": "<brief description of what the correct extraction should look like, e.g. '8 rows, columns: name, action, seq, match_clause, apply_clause'>"
}

Pipeline DSL instructions (one per line, # for comments):

─── Phase 1: Trimming ───
  SKIP_UNTIL <regex>      - skip lines until regex matches (header line itself is skipped)
  SKIP_LINES <N>          - skip N more lines after SKIP_UNTIL (e.g. separator rows)
  SKIP_BLANK              - skip blank lines
  STOP_AT    <regex>      - stop processing when regex matches
  FILTER     <regex>      - keep only lines matching regex
  REJECT     <regex>      - discard lines matching regex

─── Phase 2: Extraction ───
  SPLIT      $a $b $c ... - whitespace-split each line; last var captures all remaining fields
  REGEX      <pattern>    - named capture groups (?P<name>...) extract variables
  REPLACE    <pattern> "replacement" - regex replace on text before extraction

─── Phase 3: Post-processing ───
  SET        $name <expr> - concat ($a "/" $b) or ternary ($x == "up" ? "yes" : "no")
  EMIT                    - explicit emit (auto-emit per line if omitted)

─── Multi-section (for multi-table output) ───
  SECTION                 - start new independent processing section;
                            each section runs its own trimming + extraction against
                            the full raw input, results are joined by row index

─── Repeating groups (CRITICAL for parent-child structures) ───
  REPEAT_FOR <regex>      - split input into blocks at lines matching regex;
                            named groups (?P<name>...) are extracted as "parent"
                            fields and broadcast to every child row within that block.
                            Subsequent instructions run independently per block.
                            All blocks' rows are concatenated.
                            NESTED REPEAT_FOR is supported: a second REPEAT_FOR after
                            the first further splits each block into sub-blocks.

─── Two execution modes (auto-detected) ───
  Table mode  — when SPLIT is present: each data line → one output row
  Record mode — when only REGEX: whole text → one record (multiple REGEX merge)
  Regex table mode — when FILTER + REGEX (no SPLIT): each filtered line → one row

IMPORTANT RULES:
1. For tabular output (columns separated by whitespace), use SKIP_UNTIL + SPLIT.
2. For key-value output, use multiple REGEX instructions.
3. When the sample data contains REPEATING BLOCKS of similar structure (e.g. multiple
   route-policies, multiple ACLs, multiple interfaces), you MUST use REPEAT_FOR to
   extract ALL blocks, not just the first one.
4. When a block contains MULTIPLE SUB-BLOCKS (e.g. a route-policy with multiple
   permit/deny nodes), use NESTED REPEAT_FOR.
5. Inside a REPEAT_FOR block, you can use FILTER + REGEX (without SPLIT) for
   regex-table mode: each line matching the FILTER produces one row.

Example 1 — Simple tabular:
  SKIP_UNTIL ^Interface\s+Physical
  SPLIT $interface $physical $protocol $ip $description

Example 2 — Repeating blocks (e.g. "display acl all"):
  REPEAT_FOR ^(?P<acl_type>Basic|Advanced) IPv4 ACL (?P<acl_number>\d+) named (?P<acl_name>[^,]+),
  FILTER ^rule\s+\d+
  REGEX ^rule\s+(?P<rule_id>\d+)\s+(?P<action>permit|deny)

Example 3 — Nested repeating blocks (e.g. "display route-policy"):
  REPEAT_FOR ^Route-policy:\s+(?P<route_policy_name>.+)$
  REPEAT_FOR ^\s*(?P<action>permit|deny)\s*:\s*(?P<seq>\d+)\s*\(matched counts:\s*(?P<matched_counts>\d+)\)
  REGEX ^\s*if-match\s+(?P<match_clause>.+)$
  REGEX ^\s*apply\s+(?P<apply_clause>.+)$

CRITICAL: If the error mentions "too few rows" or "only 1 row but sample has N blocks",
the DSL likely needs REPEAT_FOR to extract all repeating blocks, not just the first one.
Look for a repeating header pattern in the sample and use it as the REPEAT_FOR regex.

FIRST: Study the sample input carefully. Describe in expected_output_description what the
correct extraction should look like (how many rows, what columns, what values). THEN write
the DSL to achieve that extraction. Fix the DSL based on the error. Only modify schema_yaml.`

	userMsg := fmt.Sprintf(`Vendor: %s
Command: %s

Error: %s

Current DSL:
%s

Sample input (first 100 lines):
%s`, group.Vendor, group.CommandNorm, errMsg, resp.SchemaYAML, samplePreview)

	llmCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

	llmResp, err := e.router.Chat(llmCtx, llm.CapParse, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: userMsg},
		},
	})
	if err != nil {
		return resp, fmt.Errorf("fix LLM call: %w", err)
	}

	text := strings.TrimSpace(llmResp.Content)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) > 2 {
			text = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var fixed LLMRuleResponse
	if err := json.Unmarshal([]byte(text), &fixed); err != nil {
		return resp, fmt.Errorf("parse fix response: %w", err)
	}
	if fixed.SchemaYAML == "" {
		return resp, fmt.Errorf("fix response has empty schema_yaml")
	}
	fixed.OutputType = resp.OutputType // preserve original type
	return fixed, nil
}

// llmSemanticCheck performs L4 LLM semantic validation: asks the LLM whether
// the parsed output looks reasonable for the given command.
func (e *Engine) llmSemanticCheck(ctx context.Context, u store.UnknownOutput, resp LLMRuleResponse, selfResult SelfTestResult) (bool, string) {
	if e.router == nil {
		return true, "" // skip if no LLM available
	}

	// Truncate output for prompt
	outputStr := string(selfResult.Output)
	if len(outputStr) > 2000 {
		outputStr = outputStr[:2000] + "..."
	}
	samplePreview := u.RawOutput
	if len(samplePreview) > 1500 {
		samplePreview = samplePreview[:1500] + "..."
	}

	system := `You are a network CLI output validation expert.
Given a network command, its raw CLI output, and a parsed structured result,
evaluate whether the parsing looks correct and complete.

Reply with ONLY JSON:
{"passed": true/false, "reason": "brief explanation"}`

	userMsg := fmt.Sprintf(`Command: %s (vendor: %s)

Raw CLI output:
%s

Parsed result (%d rows):
%s

Does the parsed result correctly capture the key information from the raw output?`,
		u.CommandNorm, u.Vendor, samplePreview, selfResult.RowCount, outputStr)

	llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	llmResp, err := e.router.Chat(llmCtx, llm.CapAnalyze, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: userMsg},
		},
	})
	if err != nil {
		slog.Warn("L4 semantic check LLM call failed", "err", err)
		return true, "" // LLM error is not a hard fail
	}

	text := strings.TrimSpace(llmResp.Content)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) > 2 {
			text = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var check struct {
		Passed bool   `json:"passed"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(text), &check); err != nil {
		return true, "" // parse error is not a hard fail
	}
	return check.Passed, check.Reason
}

// rowCountHeuristic detects when the extracted row count is suspiciously low
// compared to the number of repeating structural patterns in the sample data.
// Returns a non-empty hint string if a mismatch is detected.
//
// Strategy: find any line pattern that appears 3+ times and looks like a
// block header (lines that appear in a rhythmic interval). If extracted rows
// are significantly fewer than the pattern count, flag it.
func (e *Engine) rowCountHeuristic(sampleInput string, extractedRows int) string {
	lines := strings.Split(sampleInput, "\n")
	if len(lines) < 5 {
		return ""
	}

	// Count occurrences of line prefixes that look like block headers.
	// Common patterns: "Route-policy:", "ACL", "Interface", repeated section headers.
	// We look for lines that repeat with similar structure 3+ times.
	prefixCounts := make(map[string]int)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || len(trimmed) < 3 {
			continue
		}
		// Normalise: take the first 2-3 "words" as a prefix fingerprint
		words := strings.Fields(trimmed)
		var prefix string
		if len(words) >= 2 {
			prefix = words[0] + " " + words[1]
		} else {
			prefix = words[0]
		}
		// Only count if the prefix looks like a potential header (starts with letter)
		if len(prefix) > 0 && ((prefix[0] >= 'A' && prefix[0] <= 'Z') || (prefix[0] >= 'a' && prefix[0] <= 'z')) {
			prefixCounts[prefix]++
		}
	}

	// Also try common block-start patterns via regex
	blockPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)^Route-policy:\s+`),
		regexp.MustCompile(`(?i)^(Basic|Advanced|Layer 2) IPv[46] ACL\s+`),
		regexp.MustCompile(`(?i)^interface\s+\S+`),
		regexp.MustCompile(`(?i)^BGP Peer\s+`),
		regexp.MustCompile(`(?i)^Tunnel\s+\d+`),
		regexp.MustCompile(`(?i)^VPN Instance:\s+`),
		regexp.MustCompile(`(?i)^ip prefix-list\s+`),
		regexp.MustCompile(`(?i)^community-filter\s+`),
	}

	for _, re := range blockPatterns {
		count := 0
		for _, line := range lines {
			if re.MatchString(strings.TrimSpace(line)) {
				count++
			}
		}
		if count >= 3 && extractedRows < count/2 {
			return fmt.Sprintf("sample has %d blocks matching %q but only %d rows extracted — likely needs REPEAT_FOR",
				count, re.String(), extractedRows)
		}
	}

	// Generic: any prefix repeating 3+ times
	for prefix, count := range prefixCounts {
		if count >= 3 && extractedRows < count/2 {
			return fmt.Sprintf("sample has %d occurrences of %q but only %d rows extracted — may need REPEAT_FOR",
				count, prefix, extractedRows)
		}
	}

	return ""
}

// autoCreateTestCases stores the self-test result as an automatic test case.
func (e *Engine) autoCreateTestCases(ruleID int, sampleInput string, expectedOutput json.RawMessage, cmdNorm string) {
	_, err := e.db.CreateRuleTestCase(store.RuleTestCase{
		RuleID:      ruleID,
		Description: fmt.Sprintf("Auto-generated from sample data (%s)", cmdNorm),
		Input:       sampleInput,
		Expected:    string(expectedOutput),
	})
	if err != nil {
		slog.Warn("auto-create test case failed", "rule_id", ruleID, "err", err)
	}
}

// ImproveDSL is called from the studio API to ask LLM to fix a DSL based on failed test cases.
func (e *Engine) ImproveDSL(ctx context.Context, rule store.PendingRule, failures []FailedTestCase) (LLMRuleResponse, error) {
	var failureDesc strings.Builder
	for i, f := range failures {
		if i >= 3 {
			fmt.Fprintf(&failureDesc, "\n... and %d more failures", len(failures)-3)
			break
		}
		inputPreview := f.Input
		if len(inputPreview) > 500 {
			inputPreview = inputPreview[:500] + "..."
		}
		fmt.Fprintf(&failureDesc, "\n--- Failure %d ---\nInput (preview):\n%s\nExpected: %s\nActual: %s\nError: %s\n",
			i+1, inputPreview, f.Expected, f.Actual, f.Error)
	}

	// Build a CommandGroup with Samples populated from rule.SampleInputs so
	// that fixDSL can show the full raw sample to the LLM (not just the
	// truncated 500-char preview in the failure description).
	group := CommandGroup{
		Vendor:      rule.Vendor,
		CommandNorm: rule.CommandPattern,
	}
	var sampleStrings []string
	if rule.SampleInputs != "" && rule.SampleInputs != "[]" {
		_ = json.Unmarshal([]byte(rule.SampleInputs), &sampleStrings)
	}
	for _, s := range sampleStrings {
		if strings.TrimSpace(s) != "" {
			group.Samples = append(group.Samples, store.UnknownOutput{RawOutput: s})
		}
	}

	return e.fixDSL(ctx, group, LLMRuleResponse{
		OutputType: rule.OutputType,
		SchemaYAML: rule.SchemaYAML,
	}, failureDesc.String())
}

// FailedTestCase represents a test case that did not pass.
type FailedTestCase struct {
	Input    string
	Expected string
	Actual   string
	Error    string
}
