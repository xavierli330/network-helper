package store

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// RuntimeRule represents a row in the runtime_rules table.
// Stores approved DSL rules for runtime interpretation (no go build needed).
type RuntimeRule struct {
	ID             int       `json:"id"`
	Vendor         string    `json:"vendor"`
	CommandPattern string    `json:"command_pattern"`
	ModelPattern   string    `json:"model_pattern"`   // regex for device model, default ".*"
	OSPattern      string    `json:"os_pattern"`       // regex for OS version, default ".*"
	CmdType        string    `json:"cmd_type"`
	OutputType     string    `json:"output_type"`
	DSLText        string    `json:"dsl_text"`
	Enabled        bool      `json:"enabled"`
	SourceRuleID   *int      `json:"source_rule_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CompiledRule is an in-memory compiled version of a RuntimeRule.
// The DSL regex patterns are pre-compiled at load time for fast matching.
type CompiledRule struct {
	RuntimeRule
	Patterns []*regexp.Regexp // pre-compiled regex patterns from DSL
	modelRe  *regexp.Regexp   // compiled model_pattern regex
	osRe     *regexp.Regexp   // compiled os_pattern regex
}

// RuntimeRegistry holds all compiled rules in memory, grouped by vendor.
// Thread-safe via RWMutex. Supports hot-reload on rule changes.
type RuntimeRegistry struct {
	mu    sync.RWMutex
	rules map[string][]CompiledRule // vendor → compiled rules
}

// NewRuntimeRegistry creates an empty registry. Call Reload to populate.
func NewRuntimeRegistry() *RuntimeRegistry {
	return &RuntimeRegistry{rules: make(map[string][]CompiledRule)}
}

// Reload loads all enabled runtime rules from DB and compiles their DSL patterns.
func (r *RuntimeRegistry) Reload(db *DB) error {
	dbRules, err := db.ListRuntimeRules("", true)
	if err != nil {
		return err
	}

	grouped := make(map[string][]CompiledRule)
	for _, rr := range dbRules {
		compiled := CompiledRule{RuntimeRule: rr}
		// Extract and compile regex patterns from DSL text
		compiled.Patterns = compileDSLPatterns(rr.DSLText)
		// Compile model and OS patterns; fall back to match-all on error.
		compiled.modelRe = compilePatternOrMatchAll(rr.ModelPattern)
		compiled.osRe = compilePatternOrMatchAll(rr.OSPattern)
		grouped[rr.Vendor] = append(grouped[rr.Vendor], compiled)
	}

	r.mu.Lock()
	r.rules = grouped
	r.mu.Unlock()
	return nil
}

// compilePatternOrMatchAll compiles a regex pattern string.
// Returns a match-all regex if pattern is empty or invalid.
func compilePatternOrMatchAll(pattern string) *regexp.Regexp {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == ".*" {
		return regexp.MustCompile(`.*`)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return regexp.MustCompile(`.*`)
	}
	return re
}

// Match finds the first matching runtime rule for a vendor + command output.
// Returns (rule, true) if matched, (zero, false) if not.
// Kept for backward compatibility — delegates to MatchWithContext with empty model/os.
func (r *RuntimeRegistry) Match(vendor, output string) (CompiledRule, bool) {
	return r.MatchWithContext(vendor, "", "", output)
}

// MatchWithContext finds the best matching runtime rule using 4D matching:
// vendor (exact) + model_pattern (regex) + os_pattern (regex) + DSL content patterns.
// When multiple rules match, the most specific rule wins (scored by model+os specificity).
// Scoring: model_pattern != ".*" → +2, os_pattern != ".*" → +1.
// Ties broken by lowest rule ID.
func (r *RuntimeRegistry) MatchWithContext(vendor, deviceModel, osVersion, output string) (CompiledRule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var bestRule CompiledRule
	bestScore := -1
	found := false

	for _, cr := range r.rules[vendor] {
		// Check DSL content patterns first (must match at least one)
		dslMatch := false
		for _, pat := range cr.Patterns {
			if pat.MatchString(output) {
				dslMatch = true
				break
			}
		}
		if !dslMatch {
			continue
		}

		// Check model pattern
		if deviceModel != "" && cr.modelRe != nil && !cr.modelRe.MatchString(deviceModel) {
			continue
		}

		// Check OS pattern
		if osVersion != "" && cr.osRe != nil && !cr.osRe.MatchString(osVersion) {
			continue
		}

		// Compute specificity score
		score := 0
		if cr.ModelPattern != ".*" && cr.ModelPattern != "" {
			score += 2
		}
		if cr.OSPattern != ".*" && cr.OSPattern != "" {
			score += 1
		}

		// Higher score wins; on tie, lower ID wins
		if score > bestScore || (score == bestScore && (!found || cr.ID < bestRule.ID)) {
			bestRule = cr
			bestScore = score
			found = true
		}
	}

	return bestRule, found
}

// GetByVendor returns all compiled rules for a vendor.
func (r *RuntimeRegistry) GetByVendor(vendor string) []CompiledRule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make([]CompiledRule, len(r.rules[vendor]))
	copy(cp, r.rules[vendor])
	return cp
}

// Count returns total number of enabled rules across all vendors.
func (r *RuntimeRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, rules := range r.rules {
		n += len(rules)
	}
	return n
}

// compileDSLPatterns extracts regex patterns from DSL text and compiles them.
// DSL format supports multiple patterns separated by newlines.
// Lines starting with # are comments. Empty lines are skipped.
func compileDSLPatterns(dsl string) []*regexp.Regexp {
	var compiled []*regexp.Regexp
	for _, line := range strings.Split(dsl, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		re, err := regexp.Compile(line)
		if err != nil {
			continue // skip invalid patterns
		}
		compiled = append(compiled, re)
	}
	return compiled
}

// ── Store CRUD Methods ───────────────────────────────────────────────────

// ListRuntimeRules returns rules filtered by vendor. If enabledOnly is true,
// only enabled rules are returned.
func (db *DB) ListRuntimeRules(vendor string, enabledOnly bool) ([]RuntimeRule, error) {
	q := `SELECT id, vendor, command_pattern, COALESCE(model_pattern,'.*'), COALESCE(os_pattern,'.*'),
	             cmd_type, output_type, dsl_text, enabled,
	             source_rule_id, created_at, updated_at
	      FROM runtime_rules WHERE 1=1`
	var args []any
	if vendor != "" {
		q += ` AND vendor = ?`
		args = append(args, vendor)
	}
	if enabledOnly {
		q += ` AND enabled = 1`
	}
	q += ` ORDER BY vendor, command_pattern`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuntimeRule
	for rows.Next() {
		var rr RuntimeRule
		var sourceID *int
		if err := rows.Scan(&rr.ID, &rr.Vendor, &rr.CommandPattern, &rr.ModelPattern, &rr.OSPattern,
			&rr.CmdType, &rr.OutputType,
			&rr.DSLText, &rr.Enabled, &sourceID, &rr.CreatedAt, &rr.UpdatedAt); err != nil {
			return nil, err
		}
		rr.SourceRuleID = sourceID
		out = append(out, rr)
	}
	return out, rows.Err()
}

// GetRuntimeRule fetches a single runtime rule by ID.
func (db *DB) GetRuntimeRule(id int) (RuntimeRule, error) {
	var rr RuntimeRule
	var sourceID *int
	err := db.QueryRow(`
		SELECT id, vendor, command_pattern, COALESCE(model_pattern,'.*'), COALESCE(os_pattern,'.*'),
		       cmd_type, output_type, dsl_text, enabled,
		       source_rule_id, created_at, updated_at
		FROM runtime_rules WHERE id = ?`, id).Scan(
		&rr.ID, &rr.Vendor, &rr.CommandPattern, &rr.ModelPattern, &rr.OSPattern,
		&rr.CmdType, &rr.OutputType,
		&rr.DSLText, &rr.Enabled, &sourceID, &rr.CreatedAt, &rr.UpdatedAt)
	rr.SourceRuleID = sourceID
	return rr, err
}

// UpsertRuntimeRule inserts or updates a runtime rule by (vendor, model_pattern, os_pattern, command_pattern).
func (db *DB) UpsertRuntimeRule(rr RuntimeRule) (int, error) {
	if rr.ModelPattern == "" {
		rr.ModelPattern = ".*"
	}
	if rr.OSPattern == "" {
		rr.OSPattern = ".*"
	}
	res, err := db.Exec(`
		INSERT INTO runtime_rules (vendor, command_pattern, model_pattern, os_pattern, cmd_type, output_type, dsl_text, enabled, source_rule_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(vendor, model_pattern, os_pattern, command_pattern) DO UPDATE SET
			cmd_type = excluded.cmd_type,
			output_type = excluded.output_type,
			dsl_text = excluded.dsl_text,
			enabled = excluded.enabled,
			source_rule_id = excluded.source_rule_id`,
		rr.Vendor, rr.CommandPattern, rr.ModelPattern, rr.OSPattern,
		rr.CmdType, rr.OutputType, rr.DSLText,
		boolToInt(rr.Enabled), rr.SourceRuleID)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// UpdateRuntimeRule updates a runtime rule by ID.
func (db *DB) UpdateRuntimeRule(rr RuntimeRule) error {
	if rr.ModelPattern == "" {
		rr.ModelPattern = ".*"
	}
	if rr.OSPattern == "" {
		rr.OSPattern = ".*"
	}
	_, err := db.Exec(`
		UPDATE runtime_rules SET vendor=?, command_pattern=?, model_pattern=?, os_pattern=?,
			cmd_type=?, output_type=?, dsl_text=?, enabled=? WHERE id=?`,
		rr.Vendor, rr.CommandPattern, rr.ModelPattern, rr.OSPattern,
		rr.CmdType, rr.OutputType, rr.DSLText,
		boolToInt(rr.Enabled), rr.ID)
	return err
}

// DeleteRuntimeRule removes a runtime rule by ID.
func (db *DB) DeleteRuntimeRule(id int) error {
	_, err := db.Exec(`DELETE FROM runtime_rules WHERE id=?`, id)
	return err
}

// UpdateRuntimeRuleVendor changes the vendor for a runtime rule (Phase 3E).
func (db *DB) UpdateRuntimeRuleVendor(id int, newVendor string) error {
	_, err := db.Exec(`UPDATE runtime_rules SET vendor=? WHERE id=?`, newVendor, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
