# Parser Rule Studio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a self-improving parser layer that silently collects unrecognised command outputs, uses LLM to draft parsing rules, and lets network engineers review/test/approve rules in a local web UI that generates real Go code as PRs.

**Architecture:** Five phases delivered in dependency order: (P0) collector → (P1) discovery engine → (P2) Rule Studio web UI → (P3) schema-driven table engine → (P4) code generator + PR automation. Each phase is independently useful. Existing handwritten parsers are never replaced; new parsers are additive only.

**Tech Stack:** Go stdlib (`net/http`, `embed`, `crypto/sha256`, `os/exec`), SQLite via existing `store` package, HTMX (vendored local), `gopkg.in/yaml.v3` (already in go.mod), existing `llm.Router` for LLM calls, `gh` CLI for PR creation.

**Spec:** `docs/superpowers/specs/2026-03-25-parser-rule-studio-design.md`

---

## File Map

### New files

| File | Responsibility |
|---|---|
| `internal/parser/collector.go` | `UnknownOutputCollector` — normalise + hash + upsert `unknown_outputs` |
| `internal/parser/collector_test.go` | Unit tests for collector |
| `internal/store/rule_store.go` | DB methods for `unknown_outputs`, `pending_rules`, `rule_test_cases` |
| `internal/store/rule_store_test.go` | Unit tests for rule store |
| `internal/discovery/engine.go` | Clustering + LLM prompt dispatch → `pending_rules` |
| `internal/discovery/engine_test.go` | Unit tests for clustering logic |
| `internal/parser/engine/table.go` | `ParseTable()` schema-driven engine + `TableSchema`/`ColumnDef` types |
| `internal/parser/engine/table_test.go` | Unit tests for table engine |
| `internal/studio/server.go` | Embedded HTTP server, route registration |
| `internal/studio/handlers.go` | HTTP handlers for list / editor / sandbox / API endpoints |
| `internal/studio/static/htmx.min.js` | Vendored HTMX (download once, committed) |
| `internal/studio/templates/` | HTML templates (layout, list, editor, sandbox) |
| `internal/codegen/generator.go` | Code Generator: schema→Go file, test file, `_generated.go` patch, git+PR |
| `internal/codegen/generator_test.go` | Unit tests for template output |
| `internal/parser/huawei/huawei_generated.go` | One-time stub: `classifyGenerated()` + `parseGenerated()` for Huawei |
| `internal/parser/cisco/cisco_generated.go` | Same stub for Cisco |
| `internal/parser/h3c/h3c_generated.go` | Same stub for H3C |
| `internal/parser/juniper/juniper_generated.go` | Same stub for Juniper |
| `internal/cli/rule.go` | Cobra subcommands: `studio`, `discover`, `regen`, `history`, `list` |

### Modified files

| File | Change |
|---|---|
| `internal/store/migrations.go` | Append 3 new table DDLs + 1 trigger + indexes |
| `internal/parser/pipeline.go` | Call `collector.Collect()` for `CmdUnknown` blocks |
| `internal/parser/huawei/huawei.go` | Add fallback call to `classifyGenerated()` / `parseGenerated()` |
| `internal/parser/cisco/cisco.go` | Same fallback |
| `internal/parser/h3c/h3c.go` | Same fallback |
| `internal/parser/juniper/juniper.go` | Same fallback |
| `internal/config/config.go` | Add `RuleConfig` struct with `DiscoveryInterval` field |
| `internal/cli/root.go` | Register `newRuleCmd()` |

---

## Task 1: DB Migrations — 3 New Tables

**Files:**
- Modify: `internal/store/migrations.go`
- Modify: `internal/store/rule_store.go` (create new)
- Test: `internal/store/rule_store_test.go` (create new)

- [ ] **Step 1.1: Write failing test for migration**

```go
// internal/store/rule_store_test.go
package store_test

import (
    "testing"
    "github.com/xavierli/nethelper/internal/store"
)

func TestRuleStoreTables(t *testing.T) {
    db, err := store.Open(":memory:")
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    // unknown_outputs table exists
    _, err = db.Exec(`INSERT INTO unknown_outputs (device_id, vendor, command_raw, command_norm, raw_output, content_hash) VALUES ('d1','huawei','dis int','display interface','output','abc123')`)
    if err != nil {
        t.Errorf("unknown_outputs insert: %v", err)
    }

    // pending_rules table exists
    _, err = db.Exec(`INSERT INTO pending_rules (vendor, command_pattern, output_type, sample_inputs) VALUES ('huawei','display traffic-policy','table','[]')`)
    if err != nil {
        t.Errorf("pending_rules insert: %v", err)
    }

    // rule_test_cases with FK
    var ruleID int
    db.QueryRow(`SELECT id FROM pending_rules LIMIT 1`).Scan(&ruleID)
    _, err = db.Exec(`INSERT INTO rule_test_cases (rule_id, input, expected) VALUES (?, 'raw', '{}')`, ruleID)
    if err != nil {
        t.Errorf("rule_test_cases insert: %v", err)
    }
}
```

- [ ] **Step 1.2: Run test to confirm it fails**

```bash
go test ./internal/store/... -run TestRuleStoreTables -v
```

Expected: FAIL — `no such table: unknown_outputs`

- [ ] **Step 1.3: Add migrations**

Append to the `migrations` slice in `internal/store/migrations.go`:

```go
// Rule Studio: collect unknown command outputs
`CREATE TABLE IF NOT EXISTS unknown_outputs (
    id               INTEGER PRIMARY KEY,
    device_id        TEXT NOT NULL,
    vendor           TEXT NOT NULL,
    command_raw      TEXT NOT NULL,
    command_norm     TEXT NOT NULL,
    raw_output       TEXT NOT NULL,
    content_hash     TEXT NOT NULL,
    first_seen       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    status           TEXT NOT NULL DEFAULT 'new'
                     CHECK(status IN ('new','clustered','promoted','ignored'))
)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_unknown_dedup ON unknown_outputs(vendor, command_norm, content_hash)`,
`CREATE INDEX IF NOT EXISTS idx_unknown_vendor_cmd ON unknown_outputs(vendor, command_norm, status)`,
`CREATE INDEX IF NOT EXISTS idx_unknown_hash ON unknown_outputs(content_hash)`,

// Rule Studio: LLM-generated parser rule drafts
`CREATE TABLE IF NOT EXISTS pending_rules (
    id               INTEGER PRIMARY KEY,
    vendor           TEXT NOT NULL,
    command_pattern  TEXT NOT NULL,
    output_type      TEXT NOT NULL CHECK(output_type IN ('table','hierarchical','raw')),
    schema_yaml      TEXT,
    go_code_draft    TEXT,
    sample_inputs    TEXT NOT NULL DEFAULT '[]',
    expected_outputs TEXT,
    confidence       REAL,
    status           TEXT NOT NULL DEFAULT 'draft'
                     CHECK(status IN ('draft','testing','approved','rejected')),
    approved_by      TEXT,
    approved_at      DATETIME,
    pr_url           TEXT,
    merged_at        DATETIME,
    go_file_path     TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE TRIGGER IF NOT EXISTS pending_rules_updated_at
 AFTER UPDATE ON pending_rules
 BEGIN
     UPDATE pending_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
 END`,

// Rule Studio: test cases per rule (sandbox-saved samples)
`CREATE TABLE IF NOT EXISTS rule_test_cases (
    id          INTEGER PRIMARY KEY,
    rule_id     INTEGER NOT NULL REFERENCES pending_rules(id) ON DELETE CASCADE,
    description TEXT,
    input       TEXT NOT NULL,
    expected    TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
```

- [ ] **Step 1.4: Run test to confirm it passes**

```bash
go test ./internal/store/... -run TestRuleStoreTables -v
```

Expected: PASS

- [ ] **Step 1.5: Commit**

```bash
git add internal/store/migrations.go internal/store/rule_store_test.go
git commit -m "feat(store): add unknown_outputs, pending_rules, rule_test_cases tables"
```

---

## Task 2: Rule Store Methods

**Files:**
- Create: `internal/store/rule_store.go`
- Modify: `internal/store/rule_store_test.go`

- [ ] **Step 2.1: Write failing tests for store methods**

Add to `internal/store/rule_store_test.go`:

```go
func TestUpsertUnknownOutput(t *testing.T) {
    db, _ := store.Open(":memory:")
    defer db.Close()

    entry := store.UnknownOutput{
        DeviceID:    "dev1",
        Vendor:      "huawei",
        CommandRaw:  "dis int brief",
        CommandNorm: "display interface brief",
        RawOutput:   "PHY...",
        ContentHash: "hash1",
    }

    // First insert
    if err := db.UpsertUnknownOutput(entry); err != nil {
        t.Fatal(err)
    }
    // Duplicate insert — should increment occurrence_count
    if err := db.UpsertUnknownOutput(entry); err != nil {
        t.Fatal(err)
    }

    rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
    if len(rows) != 1 {
        t.Fatalf("want 1 row, got %d", len(rows))
    }
    if rows[0].OccurrenceCount != 2 {
        t.Fatalf("want occurrence_count=2, got %d", rows[0].OccurrenceCount)
    }
}

func TestCreateAndGetPendingRule(t *testing.T) {
    db, _ := store.Open(":memory:")
    defer db.Close()

    id, err := db.CreatePendingRule(store.PendingRule{
        Vendor:         "huawei",
        CommandPattern: "display traffic-policy",
        OutputType:     "table",
        SampleInputs:   `["sample1"]`,
        Status:         "draft",
    })
    if err != nil {
        t.Fatal(err)
    }

    rule, err := db.GetPendingRule(id)
    if err != nil {
        t.Fatal(err)
    }
    if rule.Vendor != "huawei" {
        t.Errorf("want vendor=huawei, got %s", rule.Vendor)
    }
}
```

- [ ] **Step 2.2: Run to confirm failure**

```bash
go test ./internal/store/... -run "TestUpsertUnknownOutput|TestCreateAndGetPendingRule" -v
```

Expected: FAIL — methods not defined

- [ ] **Step 2.3: Implement `internal/store/rule_store.go`**

```go
package store

import (
    "database/sql"
    "time"
)

// UnknownOutput represents a row in the unknown_outputs table.
type UnknownOutput struct {
    ID              int
    DeviceID        string
    Vendor          string
    CommandRaw      string
    CommandNorm     string
    RawOutput       string
    ContentHash     string
    FirstSeen       time.Time
    LastSeen        time.Time
    OccurrenceCount int
    Status          string
}

// PendingRule represents a row in the pending_rules table.
type PendingRule struct {
    ID              int
    Vendor          string
    CommandPattern  string
    OutputType      string
    SchemaYAML      string
    GoCodeDraft     string
    SampleInputs    string
    ExpectedOutputs string
    Confidence      float64
    Status          string
    ApprovedBy      string
    ApprovedAt      *time.Time
    PRURL           string
    MergedAt        *time.Time
    GoFilePath      string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

// RuleTestCase represents a row in the rule_test_cases table.
type RuleTestCase struct {
    ID          int
    RuleID      int
    Description string
    Input       string
    Expected    string
    CreatedAt   time.Time
}

// UpsertUnknownOutput inserts a new unknown output or increments occurrence_count
// if the same (vendor, command_norm, content_hash) already exists.
func (db *DB) UpsertUnknownOutput(u UnknownOutput) error {
    _, err := db.Exec(`
        INSERT INTO unknown_outputs (device_id, vendor, command_raw, command_norm, raw_output, content_hash)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(vendor, command_norm, content_hash) DO UPDATE SET
            occurrence_count = occurrence_count + 1,
            last_seen = CURRENT_TIMESTAMP
    `, u.DeviceID, u.Vendor, u.CommandRaw, u.CommandNorm, u.RawOutput, u.ContentHash)
    return err
}

// ListUnknownOutputs returns unknown outputs filtered by vendor and status.
func (db *DB) ListUnknownOutputs(vendor, status string, limit int) ([]UnknownOutput, error) {
    q := `SELECT id, device_id, vendor, command_raw, command_norm, raw_output, content_hash,
                 first_seen, last_seen, occurrence_count, status
          FROM unknown_outputs WHERE 1=1`
    var args []any
    if vendor != "" {
        q += " AND vendor = ?"
        args = append(args, vendor)
    }
    if status != "" {
        q += " AND status = ?"
        args = append(args, status)
    }
    q += " ORDER BY occurrence_count DESC LIMIT ?"
    args = append(args, limit)

    rows, err := db.Query(q, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []UnknownOutput
    for rows.Next() {
        var u UnknownOutput
        if err := rows.Scan(&u.ID, &u.DeviceID, &u.Vendor, &u.CommandRaw, &u.CommandNorm,
            &u.RawOutput, &u.ContentHash, &u.FirstSeen, &u.LastSeen, &u.OccurrenceCount, &u.Status); err != nil {
            return nil, err
        }
        out = append(out, u)
    }
    return out, rows.Err()
}

// UpdateUnknownOutputStatus sets the status for all outputs matching (vendor, command_norm).
func (db *DB) UpdateUnknownOutputStatus(vendor, commandNorm, status string) error {
    _, err := db.Exec(`UPDATE unknown_outputs SET status = ? WHERE vendor = ? AND command_norm = ?`,
        status, vendor, commandNorm)
    return err
}

// CreatePendingRule inserts a new pending rule and returns its ID.
func (db *DB) CreatePendingRule(r PendingRule) (int, error) {
    res, err := db.Exec(`
        INSERT INTO pending_rules (vendor, command_pattern, output_type, schema_yaml, go_code_draft,
                                   sample_inputs, expected_outputs, confidence, status)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        r.Vendor, r.CommandPattern, r.OutputType, r.SchemaYAML, r.GoCodeDraft,
        r.SampleInputs, r.ExpectedOutputs, r.Confidence, r.Status)
    if err != nil {
        return 0, err
    }
    id, _ := res.LastInsertId()
    return int(id), nil
}

// GetPendingRule fetches a single pending rule by ID.
func (db *DB) GetPendingRule(id int) (PendingRule, error) {
    var r PendingRule
    err := db.QueryRow(`
        SELECT id, vendor, command_pattern, output_type, COALESCE(schema_yaml,''), COALESCE(go_code_draft,''),
               sample_inputs, COALESCE(expected_outputs,''), COALESCE(confidence,0), status,
               COALESCE(approved_by,''), COALESCE(pr_url,''), COALESCE(go_file_path,''),
               created_at, updated_at
        FROM pending_rules WHERE id = ?`, id).Scan(
        &r.ID, &r.Vendor, &r.CommandPattern, &r.OutputType, &r.SchemaYAML, &r.GoCodeDraft,
        &r.SampleInputs, &r.ExpectedOutputs, &r.Confidence, &r.Status,
        &r.ApprovedBy, &r.PRURL, &r.GoFilePath, &r.CreatedAt, &r.UpdatedAt)
    return r, err
}

// ListPendingRules returns rules filtered by status, sorted by creation time desc.
func (db *DB) ListPendingRules(status string, limit int) ([]PendingRule, error) {
    q := `SELECT id, vendor, command_pattern, output_type, COALESCE(confidence,0), status, created_at
          FROM pending_rules WHERE 1=1`
    var args []any
    if status != "" {
        q += " AND status = ?"
        args = append(args, status)
    }
    q += " ORDER BY created_at DESC LIMIT ?"
    args = append(args, limit)

    rows, err := db.Query(q, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []PendingRule
    for rows.Next() {
        var r PendingRule
        if err := rows.Scan(&r.ID, &r.Vendor, &r.CommandPattern, &r.OutputType,
            &r.Confidence, &r.Status, &r.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, r)
    }
    return out, rows.Err()
}

// UpdatePendingRule updates editable fields of a rule.
func (db *DB) UpdatePendingRule(r PendingRule) error {
    _, err := db.Exec(`
        UPDATE pending_rules SET schema_yaml=?, go_code_draft=?, status=?,
               approved_by=?, pr_url=?, go_file_path=?
        WHERE id=?`,
        r.SchemaYAML, r.GoCodeDraft, r.Status, r.ApprovedBy, r.PRURL, r.GoFilePath, r.ID)
    return err
}

// CreateRuleTestCase inserts a test case for a rule.
func (db *DB) CreateRuleTestCase(tc RuleTestCase) (int, error) {
    res, err := db.Exec(`INSERT INTO rule_test_cases (rule_id, description, input, expected) VALUES (?,?,?,?)`,
        tc.RuleID, tc.Description, tc.Input, tc.Expected)
    if err != nil {
        return 0, err
    }
    id, _ := res.LastInsertId()
    return int(id), nil
}

// ListRuleTestCases returns all test cases for a rule.
func (db *DB) ListRuleTestCases(ruleID int) ([]RuleTestCase, error) {
    rows, err := db.Query(`SELECT id, rule_id, COALESCE(description,''), input, expected, created_at
                           FROM rule_test_cases WHERE rule_id = ? ORDER BY id`, ruleID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []RuleTestCase
    for rows.Next() {
        var tc RuleTestCase
        if err := rows.Scan(&tc.ID, &tc.RuleID, &tc.Description, &tc.Input, &tc.Expected, &tc.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, tc)
    }
    return out, rows.Err()
}

// DeleteRuleTestCase removes a test case by ID.
func (db *DB) DeleteRuleTestCase(id int) error {
    _, err := db.Exec(`DELETE FROM rule_test_cases WHERE id = ?`, id)
    return err
}

// CountRuleTestCases returns the number of test cases for a rule.
func (db *DB) CountRuleTestCases(ruleID int) (int, error) {
    var n int
    err := db.QueryRow(`SELECT COUNT(*) FROM rule_test_cases WHERE rule_id = ?`, ruleID).Scan(&n)
    return n, err
}

// GetPendingRuleByCommandNorm returns an existing draft rule for (vendor, command_norm) or sql.ErrNoRows.
func (db *DB) GetPendingRuleByCommandNorm(vendor, commandNorm string) (PendingRule, error) {
    var r PendingRule
    err := db.QueryRow(`SELECT id, vendor, command_pattern, output_type, status FROM pending_rules
                        WHERE vendor = ? AND command_pattern = ? AND status IN ('draft','testing')
                        LIMIT 1`, vendor, commandNorm).Scan(
        &r.ID, &r.Vendor, &r.CommandPattern, &r.OutputType, &r.Status)
    if err == sql.ErrNoRows {
        return r, sql.ErrNoRows
    }
    return r, err
}
```

- [ ] **Step 2.4: Run tests to confirm pass**

```bash
go test ./internal/store/... -run "TestUpsertUnknownOutput|TestCreateAndGetPendingRule" -v
```

Expected: PASS

- [ ] **Step 2.5: Run full store tests to check no regressions**

```bash
go test ./internal/store/... -v
```

Expected: all PASS

- [ ] **Step 2.6: Commit**

```bash
git add internal/store/rule_store.go internal/store/rule_store_test.go
git commit -m "feat(store): rule store CRUD methods for unknown_outputs, pending_rules, rule_test_cases"
```

---

## Task 3: Unknown Output Collector (P0)

**Files:**
- Create: `internal/parser/collector.go`
- Create: `internal/parser/collector_test.go`
- Modify: `internal/parser/pipeline.go`

- [ ] **Step 3.1: Write failing tests for collector**

```go
// internal/parser/collector_test.go
package parser_test

import (
    "testing"
    "github.com/xavierli/nethelper/internal/parser"
    "github.com/xavierli/nethelper/internal/store"
)

func TestCollectorNormalisesCommand(t *testing.T) {
    db, _ := store.Open(":memory:")
    defer db.Close()

    c := parser.NewCollector(db)

    block := parser.CommandBlock{
        Hostname: "R1", Vendor: "huawei",
        Command: "dis int brief", Output: "PHY Protocol",
    }
    if err := c.Collect(block); err != nil {
        t.Fatal(err)
    }

    rows, err := db.ListUnknownOutputs("huawei", "new", 10)
    if err != nil {
        t.Fatal(err)
    }
    if len(rows) != 1 {
        t.Fatalf("want 1 row, got %d", len(rows))
    }
    // Abbreviation must be expanded
    if rows[0].CommandNorm != "display interface brief" {
        t.Errorf("want 'display interface brief', got %q", rows[0].CommandNorm)
    }
}

func TestCollectorDeduplicates(t *testing.T) {
    db, _ := store.Open(":memory:")
    defer db.Close()
    c := parser.NewCollector(db)

    block := parser.CommandBlock{
        Vendor: "huawei", Command: "display traffic-policy", Output: "identical output",
    }
    c.Collect(block)
    c.Collect(block) // same content

    rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
    if len(rows) != 1 {
        t.Errorf("want 1 deduplicated row, got %d", len(rows))
    }
    if rows[0].OccurrenceCount != 2 {
        t.Errorf("want occurrence_count=2, got %d", rows[0].OccurrenceCount)
    }
}

func TestCollectorSilentOnError(t *testing.T) {
    // Collector with nil db must not panic
    c := parser.NewCollector(nil)
    block := parser.CommandBlock{Vendor: "huawei", Command: "display foo", Output: "x"}
    if err := c.Collect(block); err != nil {
        t.Errorf("expected silent error swallow, got %v", err)
    }
}
```

- [ ] **Step 3.2: Run to confirm failure**

```bash
go test ./internal/parser/... -run "TestCollector" -v
```

Expected: FAIL — `NewCollector` undefined

- [ ] **Step 3.3: Implement `internal/parser/collector.go`**

```go
package parser

import (
    "crypto/sha256"
    "fmt"
    "log/slog"
    "strings"

    "github.com/xavierli/nethelper/internal/store"
)

// Collector captures CommandBlocks with CmdUnknown type into the unknown_outputs table.
// All errors are logged and swallowed — the collector must never fail the pipeline.
type Collector struct {
    db *store.DB
}

// NewCollector creates a Collector backed by db. If db is nil, Collect is a no-op.
func NewCollector(db *store.DB) *Collector {
    return &Collector{db: db}
}

// Collect records a CmdUnknown block. Safe to call from pipeline; never returns a fatal error.
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

// normaliseCommand applies vendor-specific abbreviation expansion then lowercases
// and collapses whitespace. Mirrors the logic in each vendor's ClassifyCommand().
func normaliseCommand(vendor, cmd string) string {
    lower := strings.ToLower(strings.TrimSpace(cmd))
    switch vendor {
    case "huawei", "h3c":
        // Expand "dis " → "display "
        if strings.HasPrefix(lower, "dis ") && !strings.HasPrefix(lower, "display ") {
            lower = "display " + lower[4:]
        }
    case "cisco":
        // Expand "sh " → "show "
        if strings.HasPrefix(lower, "sh ") && !strings.HasPrefix(lower, "show ") {
            lower = "show " + lower[3:]
        }
    }
    // Collapse multiple spaces
    return strings.Join(strings.Fields(lower), " ")
}

// hashContent returns the first 16 hex chars of the SHA256 of s.
func hashContent(s string) string {
    sum := sha256.Sum256([]byte(s))
    return fmt.Sprintf("%x", sum[:8])
}
```

- [ ] **Step 3.4: Run tests to confirm pass**

```bash
go test ./internal/parser/... -run "TestCollector" -v
```

Expected: PASS

- [ ] **Step 3.5: Write failing integration test for pipeline → collector**

Add to `internal/parser/pipeline_test.go` (or new file `internal/parser/collector_integration_test.go`):

```go
func TestPipelineCollectsUnknownCommands(t *testing.T) {
    db, _ := store.Open(":memory:")
    defer db.Close()

    reg := parser.NewRegistry()
    reg.Register(huawei.New())
    pipe := parser.NewPipelineWithCollector(db, reg, parser.NewCollector(db))

    // "display traffic-policy" is not classified by Huawei parser → CmdUnknown
    log := "<R1>display traffic-policy\nPolicy: p1  Applied: GE0/0/1\n<R1>display version\nHuawei VRP\n"
    _, err := pipe.Ingest("test.log", log)
    if err != nil {
        t.Fatal(err)
    }

    rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
    if len(rows) != 1 {
        t.Errorf("want 1 unknown output collected, got %d", len(rows))
    }
    if rows[0].CommandNorm != "display traffic-policy" {
        t.Errorf("unexpected command_norm: %q", rows[0].CommandNorm)
    }
}
```

- [ ] **Step 3.6: Run to confirm failure**

```bash
go test ./internal/parser/... -run TestPipelineCollectsUnknownCommands -v
```

Expected: FAIL — `NewPipelineWithCollector` undefined

- [ ] **Step 3.7: Wire collector into pipeline**

In `internal/parser/pipeline.go`, modify `Pipeline` struct and `NewPipeline`:

```go
type Pipeline struct {
    db        *store.DB
    registry  *Registry
    collector *Collector  // may be nil
}

// NewPipeline creates a Pipeline with no collector (backwards-compatible).
func NewPipeline(db *store.DB, registry *Registry) *Pipeline {
    return &Pipeline{db: db, registry: registry}
}

// NewPipelineWithCollector creates a Pipeline that captures CmdUnknown outputs.
func NewPipelineWithCollector(db *store.DB, registry *Registry, c *Collector) *Pipeline {
    return &Pipeline{db: db, registry: registry, collector: c}
}
```

Inside `processBlocks()`, after the `CmdUnknown` skip-logic, add:

```go
// Collect unknown outputs for later analysis
if b.CmdType == model.CmdUnknown && p.collector != nil {
    p.collector.Collect(*b)
}
```

- [ ] **Step 3.8: Wire collector in `internal/cli/root.go`**

In `PersistentPreRunE`, after `pipeline = parser.NewPipeline(...)`, change to:

```go
pipeline = parser.NewPipelineWithCollector(db, registry, parser.NewCollector(db))
```

- [ ] **Step 3.9: Run all tests**

```bash
go test ./internal/parser/... -v
```

Expected: all PASS

- [ ] **Step 3.10: Commit**

```bash
git add internal/parser/collector.go internal/parser/collector_test.go internal/parser/pipeline.go internal/cli/root.go
git commit -m "feat(parser): P0 — unknown output collector integrated into pipeline"
```

---

## Task 4: Schema-Driven Table Engine (P3)

**Files:**
- Create: `internal/parser/engine/table.go`
- Create: `internal/parser/engine/table_test.go`

- [ ] **Step 4.1: Write failing tests**

```go
// internal/parser/engine/table_test.go
package engine_test

import (
    "testing"
    "github.com/xavierli/nethelper/internal/parser/engine"
)

var briefOutput = `Interface         PHY      Protocol  InUti  OutUti  inErrors outErrors
GigabitEthernet0/0/0  up       up        0.01%  0.01%       0        0
GigabitEthernet0/0/1  down     down         --     --       0        0
Eth-Trunk1            up       up        1.23%  0.45%       0        0
`

func TestParseTableBasic(t *testing.T) {
    schema := engine.TableSchema{
        HeaderPattern: `Interface\s+PHY`,
        SkipLines:     0,
        Columns: []engine.ColumnDef{
            {Name: "interface", Index: 0, Type: "string"},
            {Name: "phy_status", Index: 1, Type: "string"},
            {Name: "proto_status", Index: 2, Type: "string"},
        },
    }

    result, err := engine.ParseTable(schema, briefOutput)
    if err != nil {
        t.Fatal(err)
    }
    if len(result.Rows) != 3 {
        t.Fatalf("want 3 rows, got %d", len(result.Rows))
    }
    if result.Rows[0]["interface"] != "GigabitEthernet0/0/0" {
        t.Errorf("unexpected interface: %q", result.Rows[0]["interface"])
    }
    if result.Rows[1]["phy_status"] != "down" {
        t.Errorf("unexpected phy_status: %q", result.Rows[1]["phy_status"])
    }
}

func TestParseTableNoHeaderMatch(t *testing.T) {
    schema := engine.TableSchema{
        HeaderPattern: `NONEXISTENT_HEADER`,
        Columns:       []engine.ColumnDef{{Name: "col", Index: 0, Type: "string"}},
    }
    result, err := engine.ParseTable(schema, briefOutput)
    if err != nil {
        t.Fatal(err) // no header is not an error; result just has no rows
    }
    if len(result.Rows) != 0 {
        t.Errorf("want 0 rows when header not found, got %d", len(result.Rows))
    }
}
```

- [ ] **Step 4.2: Run to confirm failure**

```bash
go test ./internal/parser/engine/... -v
```

Expected: FAIL — package does not exist

- [ ] **Step 4.3: Implement `internal/parser/engine/table.go`**

```go
package engine

import (
    "regexp"
    "strings"
)

// ColumnDef describes a single column in a table output.
type ColumnDef struct {
    Name     string // field name in result row
    Index    int    // 0-based column index in whitespace-split fields
    Type     string // "string" | "int" | "ip" | "bytes" (reserved for future coercion)
    Optional bool   // if true, missing column does not cause an error
}

// TableSchema describes how to parse a fixed-column CLI table output.
type TableSchema struct {
    HeaderPattern string      // regex that matches the header line
    SkipLines     int         // number of lines to skip after the header (e.g. separator lines)
    Columns       []ColumnDef // columns to extract
}

// TableResult holds the parsed rows.
type TableResult struct {
    Rows []map[string]string
}

// ParseTable parses raw CLI output into rows using schema.
// It scans for the header line, then extracts fields from every subsequent non-empty line.
func ParseTable(schema TableSchema, raw string) (TableResult, error) {
    headerRe, err := regexp.Compile(schema.HeaderPattern)
    if err != nil {
        return TableResult{}, err
    }

    lines := strings.Split(raw, "\n")
    headerIdx := -1
    for i, line := range lines {
        if headerRe.MatchString(line) {
            headerIdx = i
            break
        }
    }

    var result TableResult
    if headerIdx < 0 {
        return result, nil // no header found — return empty, not an error
    }

    dataStart := headerIdx + 1 + schema.SkipLines
    for _, line := range lines[dataStart:] {
        trimmed := strings.TrimRight(line, "\r \t")
        if trimmed == "" {
            continue
        }
        fields := strings.Fields(trimmed)
        row := make(map[string]string, len(schema.Columns))
        for _, col := range schema.Columns {
            if col.Index < len(fields) {
                row[col.Name] = fields[col.Index]
            } else if !col.Optional {
                row[col.Name] = ""
            }
        }
        result.Rows = append(result.Rows, row)
    }
    return result, nil
}
```

- [ ] **Step 4.4: Run tests to confirm pass**

```bash
go test ./internal/parser/engine/... -v
```

Expected: PASS

- [ ] **Step 4.5: Commit**

```bash
git add internal/parser/engine/table.go internal/parser/engine/table_test.go
git commit -m "feat(parser/engine): P3 — schema-driven table parser engine"
```

---

## Task 5: Discovery Engine (P1)

**Files:**
- Create: `internal/discovery/engine.go`
- Create: `internal/discovery/engine_test.go`

- [ ] **Step 5.1: Write failing tests**

```go
// internal/discovery/engine_test.go
package discovery_test

import (
    "testing"
    "github.com/xavierli/nethelper/internal/discovery"
    "github.com/xavierli/nethelper/internal/store"
)

func TestClusterGroups(t *testing.T) {
    db, _ := store.Open(":memory:")
    defer db.Close()

    // Insert 3 outputs for same command, 1 for a different command
    for i := 0; i < 3; i++ {
        db.UpsertUnknownOutput(store.UnknownOutput{
            DeviceID: "dev1", Vendor: "huawei",
            CommandRaw: "display traffic-policy", CommandNorm: "display traffic-policy",
            RawOutput:   fmt.Sprintf("output variant %d", i),
            ContentHash: fmt.Sprintf("hash%d", i),
        })
    }
    db.UpsertUnknownOutput(store.UnknownOutput{
        DeviceID: "dev2", Vendor: "huawei",
        CommandRaw: "display qos", CommandNorm: "display qos",
        RawOutput: "qos output", ContentHash: "hashqos",
    })

    groups := discovery.ClusterByCommand(db, "huawei")
    if len(groups) != 2 {
        t.Fatalf("want 2 command groups, got %d", len(groups))
    }
    for _, g := range groups {
        if g.CommandNorm == "display traffic-policy" && len(g.Samples) != 3 {
            t.Errorf("want 3 samples for traffic-policy, got %d", len(g.Samples))
        }
    }
}
```

- [ ] **Step 5.2: Run to confirm failure**

```bash
go test ./internal/discovery/... -v
```

Expected: FAIL — package does not exist

- [ ] **Step 5.3: Implement `internal/discovery/engine.go`**

```go
package discovery

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "strings"

    "github.com/xavierli/nethelper/internal/llm"
    "github.com/xavierli/nethelper/internal/store"
)

const maxSamplesPerGroup = 5

// CommandGroup holds a normalised command and its representative raw output samples.
type CommandGroup struct {
    Vendor      string
    CommandNorm string
    Samples     []store.UnknownOutput
}

// LLMRuleResponse is the structured response we expect from the LLM.
type LLMRuleResponse struct {
    OutputType  string  `json:"output_type"`   // "table" | "hierarchical" | "raw"
    SchemaYAML  string  `json:"schema_yaml"`   // for table type
    GoCodeDraft string  `json:"go_code_draft"` // for hierarchical/raw
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

// ClusterByCommand groups unknown outputs by (vendor, command_norm) and returns
// representative samples for each group. Exported for testing.
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

// RunOnce clusters all new unknown outputs and calls LLM to generate draft rules.
// Each resulting rule is written to pending_rules. Already-drafted commands are skipped.
func (e *Engine) RunOnce(ctx context.Context, vendor string) (created int, err error) {
    groups := ClusterByCommand(e.db, vendor)
    for _, g := range groups {
        // Skip if a draft already exists for this command
        _, lookupErr := e.db.GetPendingRuleByCommandNorm(g.Vendor, g.CommandNorm)
        if lookupErr == nil {
            continue // already has a draft or testing rule
        }

        resp, llmErr := e.callLLM(ctx, g)
        if llmErr != nil {
            slog.Warn("discovery: LLM call failed", "cmd", g.CommandNorm, "error", llmErr)
            continue
        }

        samples := make([]string, len(g.Samples))
        for i, s := range g.Samples {
            samples[i] = s.RawOutput
        }
        samplesJSON, _ := json.Marshal(samples)

        _, createErr := e.db.CreatePendingRule(store.PendingRule{
            Vendor:         g.Vendor,
            CommandPattern: g.CommandNorm,
            OutputType:     resp.OutputType,
            SchemaYAML:     resp.SchemaYAML,
            GoCodeDraft:    resp.GoCodeDraft,
            SampleInputs:   string(samplesJSON),
            Confidence:     resp.Confidence,
            Status:         "draft",
        })
        if createErr != nil {
            slog.Warn("discovery: create pending rule failed", "cmd", g.CommandNorm, "error", createErr)
            continue
        }

        // Mark samples as clustered
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

    systemPrompt := `You are a network CLI output parser generator.
Analyse the provided command output samples from a network device and produce a structured JSON parsing rule.

Output format (JSON only, no markdown):
{
  "output_type": "table" | "hierarchical" | "raw",
  "schema_yaml": "<YAML column definitions, only for table type>",
  "go_code_draft": "<Go function body, only for hierarchical/raw>",
  "confidence": 0.0-1.0
}

For table outputs, schema_yaml should follow this format:
header_pattern: "<regex matching the header line>"
skip_lines: 0
columns:
  - name: "<field_name>"
    index: <0-based column index>
    type: "string"

For hierarchical/raw, go_code_draft should be a valid Go function body for:
func parseXxx(raw string) (model.ParseResult, error)`

    userMsg := fmt.Sprintf("Vendor: %s\nCommand: %s\n\n%s", g.Vendor, g.CommandNorm, sb.String())

    responseText, err := e.router.Complete(ctx, systemPrompt, userMsg)
    if err != nil {
        return LLMRuleResponse{}, fmt.Errorf("LLM complete: %w", err)
    }

    // Strip possible markdown code fences
    responseText = strings.TrimSpace(responseText)
    if strings.HasPrefix(responseText, "```") {
        lines := strings.Split(responseText, "\n")
        if len(lines) > 2 {
            responseText = strings.Join(lines[1:len(lines)-1], "\n")
        }
    }

    var resp LLMRuleResponse
    if err := json.Unmarshal([]byte(responseText), &resp); err != nil {
        return LLMRuleResponse{}, fmt.Errorf("parse LLM response: %w", err)
    }
    if resp.OutputType == "" {
        resp.OutputType = "raw"
    }
    return resp, nil
}
```

- [ ] **Step 5.4: Run tests to confirm pass**

```bash
go test ./internal/discovery/... -v
```

Expected: PASS

- [ ] **Step 5.5: Commit**

```bash
git add internal/discovery/engine.go internal/discovery/engine_test.go
git commit -m "feat(discovery): P1 — clustering + LLM draft generation engine"
```

---

## Task 6: Config Extension

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 6.1: Add `RuleConfig` to config**

In `internal/config/config.go`, add after existing structs:

```go
// RuleConfig configures the Rule Studio and Discovery Engine.
type RuleConfig struct {
    // DiscoveryInterval is how often the studio server auto-runs discovery.
    // Empty string or "0" means disabled. Example: "30m", "1h".
    DiscoveryInterval string `yaml:"discovery_interval"`
    // StudioPort is the default port for `nethelper rule studio`.
    StudioPort int `yaml:"studio_port"`
}
```

Add `Rule RuleConfig` to the `Config` struct alongside existing fields.

In `DefaultConfig()`, add:
```go
Rule: RuleConfig{StudioPort: 7070},
```

- [ ] **Step 6.2: Run tests**

```bash
go test ./internal/config/... -v
go vet ./...
```

Expected: PASS

- [ ] **Step 6.3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add RuleConfig for discovery interval and studio port"
```

---

## Task 7: Rule Studio Web Server (P2)

**Files:**
- Create: `internal/studio/server.go`
- Create: `internal/studio/handlers.go`
- Create: `internal/studio/static/htmx.min.js` (downloaded)
- Create: `internal/studio/templates/layout.html`
- Create: `internal/studio/templates/list.html`
- Create: `internal/studio/templates/editor.html`
- Create: `internal/studio/templates/sandbox.html`

- [ ] **Step 7.1: Download and vendor HTMX**

```bash
mkdir -p internal/studio/static
curl -sL https://unpkg.com/htmx.org@1.9.12/dist/htmx.min.js -o internal/studio/static/htmx.min.js
# Verify it downloaded (should be ~50KB)
wc -c internal/studio/static/htmx.min.js
```

Expected: file exists, size ~45000–55000 bytes

- [ ] **Step 7.2: Write server bootstrap test**

```go
// internal/studio/server_test.go
package studio_test

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/xavierli/nethelper/internal/store"
    "github.com/xavierli/nethelper/internal/studio"
)

func TestServerRoutes(t *testing.T) {
    db, _ := store.Open(":memory:")
    defer db.Close()

    srv := studio.NewServer(db, nil, nil) // nil discovery engine, nil LLM router

    // Root list page returns 200
    req := httptest.NewRequest("GET", "/", nil)
    w := httptest.NewRecorder()
    srv.ServeHTTP(w, req)
    if w.Code != http.StatusOK {
        t.Errorf("GET / want 200, got %d", w.Code)
    }

    // Static HTMX file served
    req2 := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
    w2 := httptest.NewRecorder()
    srv.ServeHTTP(w2, req2)
    if w2.Code != http.StatusOK {
        t.Errorf("GET /static/htmx.min.js want 200, got %d", w2.Code)
    }
}
```

- [ ] **Step 7.3: Run to confirm failure**

```bash
go test ./internal/studio/... -v
```

Expected: FAIL — package does not exist

- [ ] **Step 7.4: Implement `internal/studio/server.go`**

```go
package studio

import (
    _ "embed"
    "net/http"

    "github.com/xavierli/nethelper/internal/discovery"
    "github.com/xavierli/nethelper/internal/llm"
    "github.com/xavierli/nethelper/internal/store"
)

//go:embed static/htmx.min.js
var htmxJS []byte

// Server is the Rule Studio HTTP server.
type Server struct {
    mux   *http.ServeMux
    db    *store.DB
    eng   *discovery.Engine
    llmR  *llm.Router
}

// NewServer creates a Rule Studio server.
func NewServer(db *store.DB, eng *discovery.Engine, llmR *llm.Router) *Server {
    s := &Server{mux: http.NewServeMux(), db: db, eng: eng, llmR: llmR}
    s.registerRoutes()
    return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
    // Static files
    s.mux.HandleFunc("/static/htmx.min.js", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/javascript")
        w.Write(htmxJS)
    })

    h := &handlers{db: s.db, eng: s.eng}

    s.mux.HandleFunc("/", h.list)
    s.mux.HandleFunc("/rule/", h.ruleDispatch)        // /rule/:id and /rule/:id/sandbox
    s.mux.HandleFunc("/api/rule/", h.apiDispatch)     // /api/rule/:id/test, /api/rule/:id/approve
    s.mux.HandleFunc("/api/discover", h.apiDiscover)
}
```

- [ ] **Step 7.5: Implement `internal/studio/handlers.go`**

```go
package studio

import (
    "encoding/json"
    "fmt"
    "html/template"
    "net/http"
    "strconv"
    "strings"

    "github.com/xavierli/nethelper/internal/discovery"
    "github.com/xavierli/nethelper/internal/parser/engine"
    "github.com/xavierli/nethelper/internal/store"
    "gopkg.in/yaml.v3"
)

type handlers struct {
    db  *store.DB
    eng *discovery.Engine
}

// list renders the draft rules list page.
func (h *handlers) list(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        http.NotFound(w, r)
        return
    }
    rules, err := h.db.ListPendingRules("", 100)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    tmpl := listTemplate()
    tmpl.Execute(w, rules)
}

// ruleDispatch routes /rule/:id and /rule/:id/sandbox
func (h *handlers) ruleDispatch(w http.ResponseWriter, r *http.Request) {
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/rule/"), "/")
    if len(parts) == 0 || parts[0] == "" {
        http.Redirect(w, r, "/", http.StatusFound)
        return
    }
    id, err := strconv.Atoi(parts[0])
    if err != nil {
        http.NotFound(w, r)
        return
    }

    if len(parts) == 2 && parts[1] == "sandbox" {
        h.sandbox(w, r, id)
        return
    }
    h.editor(w, r, id)
}

func (h *handlers) editor(w http.ResponseWriter, r *http.Request, id int) {
    rule, err := h.db.GetPendingRule(id)
    if err != nil {
        http.Error(w, "rule not found", 404)
        return
    }
    if r.Method == "POST" {
        r.ParseForm()
        rule.SchemaYAML = r.FormValue("schema_yaml")
        rule.GoCodeDraft = r.FormValue("go_code_draft")
        rule.Status = "testing"
        h.db.UpdatePendingRule(rule)
        http.Redirect(w, r, fmt.Sprintf("/rule/%d/sandbox", id), http.StatusFound)
        return
    }
    tmpl := editorTemplate()
    tmpl.Execute(w, rule)
}

func (h *handlers) sandbox(w http.ResponseWriter, r *http.Request, id int) {
    rule, err := h.db.GetPendingRule(id)
    if err != nil {
        http.Error(w, "rule not found", 404)
        return
    }
    count, _ := h.db.CountRuleTestCases(id)
    data := struct {
        Rule       store.PendingRule
        TestCount  int
        ParseResult interface{}
    }{Rule: rule, TestCount: count}
    tmpl := sandboxTemplate()
    tmpl.Execute(w, data)
}

// apiDispatch routes /api/rule/:id/<action>
func (h *handlers) apiDispatch(w http.ResponseWriter, r *http.Request) {
    // path: /api/rule/:id/test or /api/rule/:id/approve or /api/rule/:id/testcase
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/rule/"), "/")
    if len(parts) < 2 {
        http.NotFound(w, r)
        return
    }
    id, err := strconv.Atoi(parts[0])
    if err != nil {
        http.NotFound(w, r)
        return
    }

    switch parts[1] {
    case "test":
        h.apiTest(w, r, id)
    case "testcase":
        h.apiSaveTestCase(w, r, id)
    case "ignore":
        h.apiIgnore(w, r, id)
    default:
        http.NotFound(w, r)
    }
}

// apiTest executes a table-type rule against posted input.
func (h *handlers) apiTest(w http.ResponseWriter, r *http.Request, id int) {
    rule, err := h.db.GetPendingRule(id)
    if err != nil {
        jsonError(w, "rule not found", 404)
        return
    }
    if rule.OutputType != "table" {
        jsonError(w, "live test only supported for table-type rules", 400)
        return
    }

    input := r.FormValue("input")
    if input == "" {
        r.ParseForm()
        input = r.FormValue("input")
    }

    // Parse YAML schema
    var schemaDef struct {
        HeaderPattern string `yaml:"header_pattern"`
        SkipLines     int    `yaml:"skip_lines"`
        Columns       []struct {
            Name     string `yaml:"name"`
            Index    int    `yaml:"index"`
            Type     string `yaml:"type"`
            Optional bool   `yaml:"optional"`
        } `yaml:"columns"`
    }
    if err := yaml.Unmarshal([]byte(rule.SchemaYAML), &schemaDef); err != nil {
        jsonError(w, "invalid schema YAML: "+err.Error(), 400)
        return
    }

    cols := make([]engine.ColumnDef, len(schemaDef.Columns))
    for i, c := range schemaDef.Columns {
        cols[i] = engine.ColumnDef{Name: c.Name, Index: c.Index, Type: c.Type, Optional: c.Optional}
    }
    schema := engine.TableSchema{
        HeaderPattern: schemaDef.HeaderPattern,
        SkipLines:     schemaDef.SkipLines,
        Columns:       cols,
    }

    result, err := engine.ParseTable(schema, input)
    if err != nil {
        jsonError(w, err.Error(), 400)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// apiSaveTestCase saves a test case from the sandbox.
func (h *handlers) apiSaveTestCase(w http.ResponseWriter, r *http.Request, id int) {
    if r.Method != "POST" {
        http.Error(w, "POST only", 405)
        return
    }
    r.ParseForm()
    input := r.FormValue("input")
    expected := r.FormValue("expected")
    desc := r.FormValue("description")
    if input == "" || expected == "" {
        jsonError(w, "input and expected are required", 400)
        return
    }
    tcID, err := h.db.CreateRuleTestCase(store.RuleTestCase{
        RuleID:      id,
        Description: desc,
        Input:       input,
        Expected:    expected,
    })
    if err != nil {
        jsonError(w, err.Error(), 500)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]int{"id": tcID})
}

// apiIgnore marks a rule as rejected.
func (h *handlers) apiIgnore(w http.ResponseWriter, r *http.Request, id int) {
    rule, err := h.db.GetPendingRule(id)
    if err != nil {
        jsonError(w, "rule not found", 404)
        return
    }
    rule.Status = "rejected"
    h.db.UpdatePendingRule(rule)
    http.Redirect(w, r, "/", http.StatusFound)
}

// apiDiscover triggers the discovery engine manually.
func (h *handlers) apiDiscover(w http.ResponseWriter, r *http.Request) {
    if h.eng == nil {
        jsonError(w, "discovery engine not configured", 503)
        return
    }
    vendor := r.URL.Query().Get("vendor")
    n, err := h.eng.RunOnce(r.Context(), vendor)
    if err != nil {
        jsonError(w, err.Error(), 500)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]int{"created": n})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ── Inline HTML templates ────────────────────────────────────────────────────

func listTemplate() *template.Template {
    return template.Must(template.New("list").Parse(`<!DOCTYPE html>
<html><head><title>Rule Studio</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:0.4rem 0.8rem;border-bottom:1px solid #ddd}
th{background:#f5f5f5}.badge{padding:2px 8px;border-radius:4px;font-size:0.85em}
.draft{background:#fff3cd}.testing{background:#cce5ff}.approved{background:#d4edda}
a{color:#0066cc}</style></head>
<body>
<h1>🔬 Rule Studio</h1>
<p>Pending rules discovered from unknown command outputs. Sort: highest occurrence first.</p>
<button hx-post="/api/discover" hx-swap="none">🔄 Run Discovery Now</button>
<table>
<tr><th>Vendor</th><th>Command Pattern</th><th>Type</th><th>Confidence</th><th>Status</th><th>Created</th><th>Actions</th></tr>
{{range .}}
<tr>
  <td>{{.Vendor}}</td>
  <td><code>{{.CommandPattern}}</code></td>
  <td>{{.OutputType}}</td>
  <td>{{printf "%.0f%%" (mul .Confidence 100.0)}}</td>
  <td><span class="badge {{.Status}}">{{.Status}}</span></td>
  <td>{{.CreatedAt.Format "2006-01-02"}}</td>
  <td><a href="/rule/{{.ID}}">Edit</a> · <a href="/rule/{{.ID}}/sandbox">Sandbox</a></td>
</tr>
{{else}}<tr><td colspan="7">No pending rules. Run discovery to populate.</td></tr>
{{end}}
</table>
</body></html>`))
}

func editorTemplate() *template.Template {
    return template.Must(template.New("editor").Parse(`<!DOCTYPE html>
<html><head><title>Rule Editor — {{.CommandPattern}}</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:1rem}
textarea{width:100%;height:400px;font-family:monospace;font-size:0.9em}
</style></head>
<body>
<h1>✏️ Rule Editor</h1>
<p><strong>Vendor:</strong> {{.Vendor}} &nbsp; <strong>Command:</strong> <code>{{.CommandPattern}}</code>
   &nbsp; <strong>Type:</strong> {{.OutputType}} &nbsp; <strong>Confidence:</strong> {{printf "%.0f%%" (mul .Confidence 100.0)}}</p>
<form method="POST">
<div class="grid">
  <div>
    <h3>{{if eq .OutputType "table"}}Schema YAML{{else}}Go Code Draft{{end}}</h3>
    {{if eq .OutputType "table"}}
    <textarea name="schema_yaml">{{.SchemaYAML}}</textarea>
    {{else}}
    <textarea name="go_code_draft">{{.GoCodeDraft}}</textarea>
    <p><em>⚠️ Go code rules: live test unavailable. Save → test after PR merge.</em></p>
    {{end}}
  </div>
  <div>
    <h3>Sample Inputs (from discovery)</h3>
    <pre style="overflow:auto;max-height:400px;background:#f8f8f8;padding:1rem">{{.SampleInputs}}</pre>
  </div>
</div>
<br>
<button type="submit">💾 Save &amp; Go to Sandbox</button>
&nbsp;<a href="/">← Back to List</a>
</form>
</body></html>`))
}

func sandboxTemplate() *template.Template {
    return template.Must(template.New("sandbox").Parse(`<!DOCTYPE html>
<html><head><title>Sandbox — {{.Rule.CommandPattern}}</title>
<script src="/static/htmx.min.js"></script>
<style>body{font-family:monospace;max-width:1200px;margin:2rem auto;padding:0 1rem}
textarea{width:100%;height:200px;font-family:monospace}
#result{background:#f0f8ff;padding:1rem;min-height:100px;white-space:pre-wrap}
table{width:100%;border-collapse:collapse}th,td{padding:0.3rem 0.6rem;border:1px solid #ddd}th{background:#eee}
</style></head>
<body>
<h1>🧪 Sandbox — <code>{{.Rule.CommandPattern}}</code></h1>
<p><strong>Vendor:</strong> {{.Rule.Vendor}} &nbsp; <strong>Type:</strong> {{.Rule.OutputType}}
   &nbsp; <strong>Test cases saved:</strong> {{.TestCount}}</p>

{{if eq .Rule.OutputType "table"}}
<h3>Paste device output:</h3>
<textarea id="input-area" placeholder="Paste raw CLI output here..."></textarea>
<br>
<button hx-post="/api/rule/{{.Rule.ID}}/test"
        hx-include="#input-area"
        hx-target="#result"
        hx-vals='{"input": ""}'>▶ Run Parse</button>
<div id="result"></div>

<h3>Save as test case:</h3>
<input type="text" id="tc-desc" placeholder="Description (optional)" style="width:300px">
<textarea id="tc-expected" placeholder='Expected JSON result, e.g. {"rows":[...]}'></textarea>
<button hx-post="/api/rule/{{.Rule.ID}}/testcase"
        hx-include="#input-area,#tc-desc,#tc-expected"
        hx-target="#tc-status">💾 Save Test Case</button>
<span id="tc-status"></span>

{{else}}
<p><em>⚠️ This is a Go code rule. Live execution is not available. Review the draft code, save test cases manually with expected output, then approve.</em></p>
<h3>Go Code Draft (read-only):</h3>
<pre style="background:#f8f8f8;padding:1rem;overflow:auto">{{.Rule.GoCodeDraft}}</pre>

<h3>Save test case (manual expected output):</h3>
<textarea id="input-area" placeholder="Paste raw CLI output here..."></textarea>
<textarea id="tc-expected" placeholder='Expected JSON ParseResult...'></textarea>
<button hx-post="/api/rule/{{.Rule.ID}}/testcase"
        hx-include="#input-area,#tc-expected"
        hx-target="#tc-status">💾 Save Test Case</button>
<span id="tc-status"></span>
{{end}}

{{if gt .TestCount 0}}
<br><br>
<form method="POST" action="/api/rule/{{.Rule.ID}}/approve">
  <button type="submit" style="background:#28a745;color:white;padding:0.5rem 1rem;border:none;cursor:pointer">
    ✅ Approve &amp; Generate PR ({{.TestCount}} test case{{if gt .TestCount 1}}s{{end}})
  </button>
</form>
{{else}}
<p><em>Save at least one test case to enable approve.</em></p>
{{end}}
<br><a href="/">← Back to List</a>
</body></html>`))
}
```

**Note:** The `mul` template function is not built-in. Register it on the template, or replace `{{printf "%.0f%%" (mul .Confidence 100.0)}}` with `{{printf "%.0f%%" .Confidence}}` in listTemplate and editorTemplate as a simpler workaround for now.

- [ ] **Step 7.6: Run tests to confirm pass**

```bash
go test ./internal/studio/... -v
```

Expected: PASS

- [ ] **Step 7.7: Verify build compiles**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 7.8: Commit**

```bash
git add internal/studio/
git commit -m "feat(studio): P2 — embedded web server with list/editor/sandbox views"
```

---

## Task 8: `_generated.go` Stubs + Vendor Fallback Hooks

**Files:**
- Create: `internal/parser/huawei/huawei_generated.go`
- Create: `internal/parser/cisco/cisco_generated.go`
- Create: `internal/parser/h3c/h3c_generated.go`
- Create: `internal/parser/juniper/juniper_generated.go`
- Modify: `internal/parser/huawei/huawei.go`
- Modify: `internal/parser/cisco/cisco.go`
- Modify: `internal/parser/h3c/h3c.go`
- Modify: `internal/parser/juniper/juniper.go`

- [ ] **Step 8.1: Write test for fallback dispatch**

```go
// internal/parser/huawei/generated_test.go
package huawei_test

import (
    "testing"
    "github.com/xavierli/nethelper/internal/model"
    "github.com/xavierli/nethelper/internal/parser/huawei"
)

func TestClassifyCommandFallsBackToGenerated(t *testing.T) {
    p := huawei.New()
    // "display traffic-policy" is not in the main switch — should return CmdUnknown (not panic)
    ct := p.ClassifyCommand("display traffic-policy")
    if ct != model.CmdUnknown {
        t.Errorf("expected CmdUnknown for unregistered command, got %v", ct)
    }
}
```

- [ ] **Step 8.2: Run — this should already pass (baseline)**

```bash
go test ./internal/parser/huawei/... -run TestClassifyCommandFallsBackToGenerated -v
```

Expected: PASS (behaviour already correct; test is a regression guard)

- [ ] **Step 8.3: Create `huawei_generated.go` stub**

```go
// Code generated by nethelper rule-studio. DO NOT EDIT.
// This file is maintained by the Code Generator. Add rules via `nethelper rule studio`.
package huawei

import (
    "strings"
    "github.com/xavierli/nethelper/internal/model"
)

// classifyGenerated returns the CommandType for commands added via Rule Studio.
// Called as a fallback from ClassifyCommand() when the main switch returns CmdUnknown.
func classifyGenerated(cmd string) model.CommandType {
    // Rule Studio-generated cases will be inserted here.
    _ = strings.HasPrefix // keep import used
    return model.CmdUnknown
}

// parseGenerated dispatches to Rule Studio-generated parsers.
// Called as a fallback from ParseOutput() when cmdType is unhandled.
func parseGenerated(cmdType model.CommandType, raw string) (model.ParseResult, error) {
    // Rule Studio-generated dispatch will be inserted here.
    return model.ParseResult{Type: cmdType, RawText: raw}, nil
}
```

Create the same stub for cisco, h3c, and juniper (change `package` name accordingly). For Cisco: replace `strings.HasPrefix` usage note with `_ = cmd`.

- [ ] **Step 8.4: Add fallback call in `huawei.go`**

In `ClassifyCommand()`, at the end of the switch, before `return model.CmdUnknown`:

```go
default:
    if ct := classifyGenerated(lower); ct != model.CmdUnknown {
        return ct
    }
    return model.CmdUnknown
```

In `ParseOutput()`, add a default case:

```go
default:
    return parseGenerated(cmdType, raw)
```

Do the same for cisco, h3c, and juniper parsers.

- [ ] **Step 8.5: Run all parser tests**

```bash
go test ./internal/parser/... -v
```

Expected: all PASS — no regressions

- [ ] **Step 8.6: Commit**

```bash
git add internal/parser/huawei/ internal/parser/cisco/ internal/parser/h3c/ internal/parser/juniper/
git commit -m "feat(parser): add _generated.go stubs and fallback hooks to all vendor parsers"
```

---

## Task 9: Code Generator (P4)

**Files:**
- Create: `internal/codegen/generator.go`
- Create: `internal/codegen/generator_test.go`

- [ ] **Step 9.1: Write failing tests for template output**

```go
// internal/codegen/generator_test.go
package codegen_test

import (
    "strings"
    "testing"
    "github.com/xavierli/nethelper/internal/codegen"
    "github.com/xavierli/nethelper/internal/store"
)

func TestGenerateParserFile_Table(t *testing.T) {
    rule := store.PendingRule{
        ID:             42,
        Vendor:         "huawei",
        CommandPattern: "display traffic-policy statistics interface",
        OutputType:     "table",
        SchemaYAML: `header_pattern: "Interface\\s+Policy"
skip_lines: 0
columns:
  - name: interface
    index: 0
    type: string
  - name: policy
    index: 1
    type: string`,
        ApprovedBy: "zhangsan",
    }

    src, err := codegen.GenerateParserFile(rule)
    if err != nil {
        t.Fatal(err)
    }

    if !strings.Contains(src, "func ParseHuaweiTrafficPolicyStatisticsInterface") {
        t.Error("expected function name in generated source")
    }
    if !strings.Contains(src, "engine.ParseTable") {
        t.Error("expected engine.ParseTable call in generated source")
    }
    if !strings.Contains(src, "Rule ID:   42") {
        t.Error("expected rule ID in header comment")
    }
}

func TestGenerateTestFile(t *testing.T) {
    rule := store.PendingRule{
        ID: 42, Vendor: "huawei",
        CommandPattern: "display traffic-policy statistics interface",
        OutputType:     "table",
    }
    testCases := []store.RuleTestCase{
        {ID: 1, RuleID: 42, Input: "raw output", Expected: `{"rows":[]}`},
    }

    src, err := codegen.GenerateTestFile(rule, testCases)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(src, "TestParseHuaweiTrafficPolicyStatisticsInterface") {
        t.Error("expected test function name")
    }
}

func TestCmdNameToGoIdent(t *testing.T) {
    cases := []struct{ in, want string }{
        {"display traffic-policy statistics interface", "TrafficPolicyStatisticsInterface"},
        {"show ip route", "IpRoute"},
        {"display interface", "Interface"},
    }
    for _, c := range cases {
        got := codegen.CmdNameToGoIdent(c.in)
        if got != c.want {
            t.Errorf("CmdNameToGoIdent(%q) = %q, want %q", c.in, got, c.want)
        }
    }
}
```

- [ ] **Step 9.2: Run to confirm failure**

```bash
go test ./internal/codegen/... -v
```

Expected: FAIL — package does not exist

- [ ] **Step 9.3: Implement `internal/codegen/generator.go`**

```go
package codegen

import (
    "bytes"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "text/template"
    "time"
    "os/user"

    "gopkg.in/yaml.v3"
    "github.com/xavierli/nethelper/internal/store"
)

// CmdNameToGoIdent converts a command string to a Go identifier fragment.
// "display traffic-policy statistics interface" → "TrafficPolicyStatisticsInterface"
func CmdNameToGoIdent(cmd string) string {
    // Remove vendor-specific verbs (display/show/dis/sh)
    lower := strings.ToLower(strings.TrimSpace(cmd))
    for _, prefix := range []string{"display ", "show ", "dis ", "sh "} {
        lower = strings.TrimPrefix(lower, prefix)
    }
    parts := strings.FieldsFunc(lower, func(r rune) bool {
        return r == ' ' || r == '-' || r == '_'
    })
    var sb strings.Builder
    for _, p := range parts {
        if len(p) > 0 {
            sb.WriteString(strings.ToUpper(p[:1]) + p[1:])
        }
    }
    return sb.String()
}

// GoFuncName returns the generated parser function name.
// vendor="huawei", cmd="display traffic-policy" → "ParseHuaweiTrafficPolicy"
func GoFuncName(vendor, cmd string) string {
    v := strings.ToUpper(vendor[:1]) + vendor[1:]
    return "Parse" + v + CmdNameToGoIdent(cmd)
}

// TargetFilePath returns the repo-relative path for the generated parser file.
func TargetFilePath(vendor, cmd string) string {
    ident := strings.ToLower(CmdNameToGoIdent(cmd))
    ident = strings.NewReplacer(" ", "_", "-", "_").Replace(ident)
    return fmt.Sprintf("internal/parser/%s/%s.go", vendor, ident)
}

// GenerateParserFile generates the Go source for a parser based on rule.
func GenerateParserFile(rule store.PendingRule) (string, error) {
    approvedBy := rule.ApprovedBy
    if approvedBy == "" {
        if u, err := user.Current(); err == nil {
            approvedBy = u.Username
        }
    }
    funcName := GoFuncName(rule.Vendor, rule.CommandPattern)

    var body string
    if rule.OutputType == "table" {
        tableBody, err := generateTableBody(rule.SchemaYAML, funcName)
        if err != nil {
            return "", fmt.Errorf("generate table body: %w", err)
        }
        body = tableBody
    } else {
        body = rule.GoCodeDraft
    }

    tpl := template.Must(template.New("parser").Parse(`// Code generated by nethelper rule-studio. DO NOT EDIT.
// Command:   {{.Command}}
// Vendor:    {{.Vendor}}
// Rule ID:   {{.RuleID}}
// Approved:  {{.Date}} by {{.ApprovedBy}}
// Regenerate: nethelper rule regen {{.RuleID}}
package {{.Vendor}}

import (
    "github.com/xavierli/nethelper/internal/model"
    "github.com/xavierli/nethelper/internal/parser/engine"
)

{{.Body}}
`))

    var buf bytes.Buffer
    err := tpl.Execute(&buf, map[string]any{
        "Command":    rule.CommandPattern,
        "Vendor":     rule.Vendor,
        "RuleID":     rule.ID,
        "Date":       time.Now().Format("2006-01-02"),
        "ApprovedBy": approvedBy,
        "Body":       body,
    })
    return buf.String(), err
}

func generateTableBody(schemaYAML, funcName string) (string, error) {
    var def struct {
        HeaderPattern string `yaml:"header_pattern"`
        SkipLines     int    `yaml:"skip_lines"`
        Columns       []struct {
            Name     string `yaml:"name"`
            Index    int    `yaml:"index"`
            Type     string `yaml:"type"`
            Optional bool   `yaml:"optional"`
        } `yaml:"columns"`
    }
    if err := yaml.Unmarshal([]byte(schemaYAML), &def); err != nil {
        return "", err
    }

    var cols strings.Builder
    for _, c := range def.Columns {
        cols.WriteString(fmt.Sprintf(
            "\t\t{Name: %q, Index: %d, Type: %q, Optional: %v},\n",
            c.Name, c.Index, c.Type, c.Optional))
    }

    return fmt.Sprintf(`func %s(raw string) (model.ParseResult, error) {
    schema := engine.TableSchema{
        HeaderPattern: %q,
        SkipLines:     %d,
        Columns: []engine.ColumnDef{
%s
        },
    }
    tableResult, err := engine.ParseTable(schema, raw)
    if err != nil {
        return model.ParseResult{RawText: raw}, err
    }
    _ = tableResult // TODO: map rows to model fields
    return model.ParseResult{Type: model.CmdUnknown, RawText: raw}, nil
}`, funcName, def.HeaderPattern, def.SkipLines, cols.String()), nil
}

// GenerateTestFile generates a _test.go file with one test per test case.
func GenerateTestFile(rule store.PendingRule, testCases []store.RuleTestCase) (string, error) {
    funcName := GoFuncName(rule.Vendor, rule.CommandPattern)
    tpl := template.Must(template.New("test").Parse(`// Code generated by nethelper rule-studio. DO NOT EDIT.
package {{.Vendor}}_test

import (
    "testing"
    "github.com/xavierli/nethelper/internal/parser/{{.Vendor}}"
)

{{range $i, $tc := .TestCases}}
func Test{{$.FuncName}}_Case{{$i}}(t *testing.T) {
    input := ` + "`" + `{{$tc.Input}}` + "`" + `
    _, err := {{$.Vendor}}.{{$.FuncName}}(input)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // Expected: {{$tc.Expected}}
    // TODO: add field assertions after reviewing generated output
}
{{end}}
`))

    var buf bytes.Buffer
    err := tpl.Execute(&buf, map[string]any{
        "Vendor":    rule.Vendor,
        "FuncName":  funcName,
        "TestCases": testCases,
    })
    return buf.String(), err
}

// GeneratorOptions configures a code generation run.
type GeneratorOptions struct {
    RepoRoot   string // absolute path to repo root
    DryRun     bool   // if true, print files but don't write or create PR
}

// Generate performs the full code generation, commit, and PR creation for a rule.
func Generate(rule store.PendingRule, testCases []store.RuleTestCase, opts GeneratorOptions) error {
    // Pre-flight: gh CLI available
    if !opts.DryRun {
        if err := checkGH(); err != nil {
            return err
        }
    }

    // Generate files
    parserSrc, err := GenerateParserFile(rule)
    if err != nil {
        return fmt.Errorf("generate parser: %w", err)
    }
    testSrc, err := GenerateTestFile(rule, testCases)
    if err != nil {
        return fmt.Errorf("generate test: %w", err)
    }

    parserPath := filepath.Join(opts.RepoRoot, TargetFilePath(rule.Vendor, rule.CommandPattern))
    testPath := strings.TrimSuffix(parserPath, ".go") + "_test.go"

    if opts.DryRun {
        fmt.Printf("=== %s ===\n%s\n\n=== %s ===\n%s\n", parserPath, parserSrc, testPath, testSrc)
        return nil
    }

    // Write files
    if err := os.MkdirAll(filepath.Dir(parserPath), 0755); err != nil {
        return err
    }
    if err := os.WriteFile(parserPath, []byte(parserSrc), 0644); err != nil {
        return err
    }
    if err := os.WriteFile(testPath, []byte(testSrc), 0644); err != nil {
        return err
    }

    ident := strings.ToLower(CmdNameToGoIdent(rule.CommandPattern))
    branch := fmt.Sprintf("rule/%s-%s-%d", rule.Vendor, strings.ReplaceAll(ident, " ", "-"), rule.ID)

    run := func(args ...string) error {
        cmd := exec.Command(args[0], args[1:]...)
        cmd.Dir = opts.RepoRoot
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        return cmd.Run()
    }

    if err := run("git", "checkout", "-b", branch); err != nil {
        return fmt.Errorf("git checkout: %w", err)
    }
    relParser := TargetFilePath(rule.Vendor, rule.CommandPattern)
    relTest := strings.TrimSuffix(relParser, ".go") + "_test.go"
    if err := run("git", "add", relParser, relTest); err != nil {
        return fmt.Errorf("git add: %w", err)
    }
    msg := fmt.Sprintf("feat(parser): add %s parser for %q", rule.Vendor, rule.CommandPattern)
    if err := run("git", "commit", "-m", msg); err != nil {
        return fmt.Errorf("git commit: %w", err)
    }
    if err := run("git", "push", "-u", "origin", branch); err != nil {
        return fmt.Errorf("git push: %w", err)
    }

    approvedBy := rule.ApprovedBy
    body := fmt.Sprintf("Auto-generated by nethelper rule-studio.\n\n**Vendor:** %s\n**Command:** %s\n**Rule ID:** %d\n**Approved by:** %s\n**Test cases:** %d",
        rule.Vendor, rule.CommandPattern, rule.ID, approvedBy, len(testCases))
    return run("gh", "pr", "create",
        "--title", fmt.Sprintf("feat(parser): %s %s parser", rule.Vendor, rule.CommandPattern),
        "--body", body)
}

func checkGH() error {
    if _, err := exec.LookPath("gh"); err != nil {
        return fmt.Errorf("gh CLI not found — install from https://cli.github.com and run 'gh auth login'")
    }
    cmd := exec.Command("gh", "auth", "status")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("gh CLI not authenticated — run 'gh auth login'")
    }
    return nil
}
```

- [ ] **Step 9.4: Run tests to confirm pass**

```bash
go test ./internal/codegen/... -v
```

Expected: PASS

- [ ] **Step 9.5: Commit**

```bash
git add internal/codegen/
git commit -m "feat(codegen): P4 — code generator with template output and PR creation"
```

---

## Task 10: CLI Commands

**Files:**
- Create: `internal/cli/rule.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 10.1: Implement `internal/cli/rule.go`**

```go
package cli

import (
    "fmt"
    "os"
    "strconv"
    "time"

    "github.com/spf13/cobra"
    "github.com/xavierli/nethelper/internal/codegen"
    "github.com/xavierli/nethelper/internal/discovery"
    "github.com/xavierli/nethelper/internal/studio"
)

func newRuleCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "rule",
        Short: "Parser Rule Studio — manage adaptive parser rules",
    }
    cmd.AddCommand(newRuleStudioCmd())
    cmd.AddCommand(newRuleDiscoverCmd())
    cmd.AddCommand(newRuleListCmd())
    cmd.AddCommand(newRuleRegenCmd())
    cmd.AddCommand(newRuleHistoryCmd())
    return cmd
}

func newRuleStudioCmd() *cobra.Command {
    var port int
    cmd := &cobra.Command{
        Use:   "studio",
        Short: "Start the Rule Studio web UI",
        RunE: func(cmd *cobra.Command, args []string) error {
            if port == 0 && cfg.Rule.StudioPort != 0 {
                port = cfg.Rule.StudioPort
            }
            if port == 0 {
                port = 7070
            }
            eng := discovery.New(db, llmRouter)
            srv := studio.NewServer(db, eng, llmRouter)
            addr := fmt.Sprintf(":%d", port)
            fmt.Printf("🔬 Rule Studio running at http://localhost%s\n", addr)
            fmt.Println("Press Ctrl+C to stop.")
            return srv.ListenAndServe(addr)
        },
    }
    cmd.Flags().IntVar(&port, "port", 0, "HTTP port (default: 7070 or config)")
    return cmd
}

func newRuleDiscoverCmd() *cobra.Command {
    var vendor string
    var limit int
    cmd := &cobra.Command{
        Use:   "discover",
        Short: "Run discovery engine to generate rule drafts from collected unknown outputs",
        RunE: func(cmd *cobra.Command, args []string) error {
            eng := discovery.New(db, llmRouter)
            n, err := eng.RunOnce(cmd.Context(), vendor)
            if err != nil {
                return err
            }
            fmt.Printf("Discovery complete: %d new rule drafts created.\n", n)
            return nil
        },
    }
    cmd.Flags().StringVar(&vendor, "vendor", "", "Limit to specific vendor (default: all)")
    cmd.Flags().IntVar(&limit, "limit", 20, "Max rules to create")
    return cmd
}

func newRuleListCmd() *cobra.Command {
    var status string
    cmd := &cobra.Command{
        Use:   "list",
        Short: "List pending rules",
        RunE: func(cmd *cobra.Command, args []string) error {
            rules, err := db.ListPendingRules(status, 50)
            if err != nil {
                return err
            }
            if len(rules) == 0 {
                fmt.Println("No rules found.")
                return nil
            }
            fmt.Printf("%-6s %-10s %-45s %-12s %-10s\n", "ID", "Vendor", "Command Pattern", "Type", "Status")
            fmt.Println(string(make([]byte, 90)))
            for _, r := range rules {
                fmt.Printf("%-6d %-10s %-45s %-12s %-10s\n",
                    r.ID, r.Vendor, truncate(r.CommandPattern, 44), r.OutputType, r.Status)
            }
            return nil
        },
    }
    cmd.Flags().StringVar(&status, "status", "", "Filter by status (draft|testing|approved|rejected)")
    return cmd
}

func newRuleRegenCmd() *cobra.Command {
    var force bool
    cmd := &cobra.Command{
        Use:   "regen <rule-id>",
        Short: "Regenerate Go files for an approved rule",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            id, err := strconv.Atoi(args[0])
            if err != nil {
                return fmt.Errorf("invalid rule id: %s", args[0])
            }
            rule, err := db.GetPendingRule(id)
            if err != nil {
                return fmt.Errorf("rule %d not found: %w", id, err)
            }
            if rule.MergedAt != nil && !force {
                return fmt.Errorf("rule was merged at %s — use --force to regenerate",
                    rule.MergedAt.Format(time.RFC3339))
            }
            testCases, err := db.ListRuleTestCases(id)
            if err != nil {
                return err
            }
            repoRoot, _ := os.Getwd()
            return codegen.Generate(rule, testCases, codegen.GeneratorOptions{
                RepoRoot: repoRoot,
                DryRun:   false,
            })
        },
    }
    cmd.Flags().BoolVar(&force, "force", false, "Force regeneration even if already merged")
    return cmd
}

func newRuleHistoryCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "history <vendor> <command>",
        Short: "Show history for a command pattern",
        Args:  cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            rules, err := db.ListPendingRules("", 100)
            if err != nil {
                return err
            }
            vendor, command := args[0], args[1]
            found := false
            for _, r := range rules {
                if r.Vendor == vendor && r.CommandPattern == command {
                    fmt.Printf("ID: %d  Status: %s  PR: %s  Created: %s\n",
                        r.ID, r.Status, r.PRURL, r.CreatedAt.Format("2006-01-02 15:04"))
                    found = true
                }
            }
            if !found {
                fmt.Println("No rule history found.")
            }
            return nil
        },
    }
}

func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n-1] + "…"
}
```

- [ ] **Step 10.2: Add `ListenAndServe` method to studio.Server**

In `internal/studio/server.go`, add:

```go
// ListenAndServe starts the HTTP server on addr (e.g. ":7070").
func (s *Server) ListenAndServe(addr string) error {
    return http.ListenAndServe(addr, s)
}
```

- [ ] **Step 10.3: Register in root.go**

In `internal/cli/root.go`, `NewRootCmd()`, add after existing `AddCommand` calls:

```go
root.AddCommand(newRuleCmd())
```

- [ ] **Step 10.4: Build and smoke test**

```bash
go build -o nethelper ./cmd/nethelper
./nethelper rule --help
./nethelper rule list
./nethelper rule discover --help
```

Expected: help text prints, `list` returns "No rules found." without errors

- [ ] **Step 10.5: Run all tests**

```bash
go test ./...
```

Expected: all PASS

- [ ] **Step 10.6: Commit**

```bash
git add internal/cli/rule.go internal/cli/root.go internal/studio/server.go
git commit -m "feat(cli): add 'nethelper rule' command group — studio, discover, list, regen, history"
```

---

## Task 11: End-to-End Smoke Test

- [ ] **Step 11.1: Build the binary**

```bash
go build -o nethelper ./cmd/nethelper
```

- [ ] **Step 11.2: Ingest a log file with an unknown command**

Create a test log file `/tmp/test_unknown.log`:

```
<RouterA>display traffic-policy statistics interface GigabitEthernet0/0/1
 Policy: test-policy
 Applied: GigabitEthernet0/0/1 (Inbound)
  Classifier: BE
    Matched packets: 1000
<RouterA>display version
Huawei Versatile Routing Platform Software
VRP (R) software, Version 8.200
```

```bash
./nethelper watch ingest /tmp/test_unknown.log
```

- [ ] **Step 11.3: Verify unknown output was collected**

```bash
sqlite3 ~/.nethelper/nethelper.db \
  "SELECT vendor, command_norm, occurrence_count FROM unknown_outputs LIMIT 10;"
```

Expected: row with `huawei | display traffic-policy statistics interface gigabitethernet0/0/1 | 1`

- [ ] **Step 11.4: Start Rule Studio and verify UI loads**

```bash
./nethelper rule studio &
# Open http://localhost:7070 in browser
# Should show list page (empty or with rules if discovery ran)
curl -s http://localhost:7070/ | grep "Rule Studio"
```

Expected: HTML contains "Rule Studio"

- [ ] **Step 11.5: Final commit**

```bash
go test ./...
git add -A
git commit -m "test: end-to-end smoke test for parser rule studio P0-P4"
```

---

## Summary

| Task | Phase | Component | Key files |
|---|---|---|---|
| 1 | — | DB migrations | `store/migrations.go` |
| 2 | — | Rule store CRUD | `store/rule_store.go` |
| 3 | P0 | Collector + pipeline integration | `parser/collector.go`, `parser/pipeline.go` |
| 4 | P3 | Schema-driven table engine | `parser/engine/table.go` |
| 5 | P1 | Discovery + LLM engine | `discovery/engine.go` |
| 6 | — | Config extension | `config/config.go` |
| 7 | P2 | Rule Studio web server | `studio/server.go`, `studio/handlers.go` |
| 8 | — | `_generated.go` stubs + hooks | All vendor parsers |
| 9 | P4 | Code generator + PR | `codegen/generator.go` |
| 10 | — | CLI commands | `cli/rule.go` |
| 11 | — | E2E smoke test | — |
