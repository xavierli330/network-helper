# Parser Rule Studio — Design Spec

**Date:** 2026-03-25
**Status:** Draft
**Scope:** Adaptive parser discovery, rule authoring, and code generation for nethelper

---

## 1. Problem Statement

The current parser layer is fully hardcoded: `ClassifyCommand()` uses `strings.HasPrefix` rules and `ParseOutput()` calls hand-written Go functions. When the pipeline encounters an unknown command (`CmdUnknown`) or a new vendor, it silently skips the output. There is no mechanism to learn from unseen data.

Key pain points:
- New commands require a developer to write a new Go parser file and wire it into dispatch — zero automation
- Unknown vendors need a new `VendorParser` implementation from scratch
- No tooling for engineers to test, validate, or manage parser rules
- No regression safety net when parsers are modified

---

## 2. Goals

1. **Automatic discovery** — detect and collect unrecognised command outputs during normal ingestion, without blocking the pipeline
2. **LLM-assisted drafting** — use LLM to analyse collected samples and generate schema/code drafts
3. **Engineer-owned approval** — network engineers review drafts in a web UI, test against real samples, and approve
4. **Code-first permanence** — approved rules generate real Go code via PR; rules become first-class code, not DB entries
5. **Regression safety** — every approved sample becomes a test case; CI enforces correctness

---

## 3. Non-Goals

- Real-time / inline LLM parsing (no dynamic parsing at ingest time)
- A general-purpose low-code rule engine (YAML-only)
- Replacing existing hardcoded parsers (they remain; new parsers are additive)
- Automatic merge without human review
- New-vendor discovery (unrecognised prompt formats are out of scope for this feature; see §11)

---

## 4. Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                   RUNTIME PIPELINE                       │
│  终端日志 → Split() → ClassifyCommand() → ParseOutput()  │
│                              ↓ (CmdUnknown)              │
│                    UnknownOutputCollector                 │
│                    (writes unknown_outputs table)        │
└───────────────────────┬─────────────────────────────────┘
                        │ offline, async
                        ↓
┌─────────────────────────────────────────────────────────┐
│                  DISCOVERY ENGINE                        │
│  1. Cluster unknown_outputs by (vendor, command_norm)   │
│  2. LLM: classify output type + generate schema/code    │
│  3. Write pending_rules (status=draft)                  │
└───────────────────────┬─────────────────────────────────┘
                        │
                        ↓
┌─────────────────────────────────────────────────────────┐
│                 RULE STUDIO  (localhost Web UI)          │
│  - Draft list (sorted by occurrence_count)              │
│  - Rule editor: YAML schema | Go code draft             │
│  - Test sandbox: paste output → live parse result       │
│  - Test case management → regression suite              │
│  - Approve button → triggers Code Generator             │
└───────────────────────┬─────────────────────────────────┘
                        │ approve
                        ↓
┌─────────────────────────────────────────────────────────┐
│                   CODE GENERATOR                         │
│  schema/code → internal/parser/<vendor>/<cmd>.go        │
│             → internal/parser/<vendor>/<cmd>_test.go    │
│  add to <vendor>_generated.go (ClassifyCommand fallback)│
│  git branch → commit → gh pr create                     │
└─────────────────────────────────────────────────────────┘
```

---

## 5. Component Design

### 5.1 Unknown Output Collector

**Trigger:** Inside `pipeline.processBlocks()`, when a block has `CmdType == CmdUnknown`.

**Behaviour:**
- Insert or upsert into `unknown_outputs` table
- Non-blocking — errors are logged and silently swallowed, never failing the main pipeline
- Deduplicates on `(vendor, command_norm, content_hash)` via a UNIQUE index; increments `occurrence_count` on repeat via `INSERT OR REPLACE` or `ON CONFLICT DO UPDATE`

**`command_norm` definition:**
`command_norm` is computed by applying the vendor parser's abbreviation expansion (the same logic as `ClassifyCommand()`) then lowercasing and collapsing whitespace. Example: `"dis int brief"` → `"display interface brief"`. This ensures semantically identical commands entered with different abbreviations cluster together.

**New DB table: `unknown_outputs`**

```sql
CREATE TABLE unknown_outputs (
    id              INTEGER PRIMARY KEY,
    device_id       TEXT NOT NULL,
    vendor          TEXT NOT NULL,
    command_raw     TEXT NOT NULL,
    command_norm    TEXT NOT NULL,
    raw_output      TEXT NOT NULL,
    content_hash    TEXT NOT NULL,   -- SHA256 of raw_output
    first_seen      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'new'
                    CHECK(status IN ('new','clustered','promoted','ignored'))
);
-- Deduplication constraint: same vendor + normalised command + content hash = same entry
CREATE UNIQUE INDEX idx_unknown_dedup ON unknown_outputs(vendor, command_norm, content_hash);
CREATE INDEX idx_unknown_vendor_cmd  ON unknown_outputs(vendor, command_norm, status);
CREATE INDEX idx_unknown_hash        ON unknown_outputs(content_hash);
```

---

### 5.2 Discovery Engine

**Trigger:** Offline only. Two modes:
- `nethelper rule discover` — manual, on-demand
- Background goroutine **scoped exclusively to the `rule studio` server process** — active only while `nethelper rule studio` is running; opt-in via config (`rule.discovery_interval: 30m`); default: disabled

**Clustering:**
1. Group `unknown_outputs` where `status='new'` by `(vendor, command_norm)`
2. Within each group, sub-cluster by structural signature: presence of header line, column count, indentation depth, separator pattern
3. Select up to 5 representative samples per cluster (most recent + most structurally distinct)
4. Mark selected records `status='clustered'`

**LLM Prompt Strategy:**
- System: "You are a network CLI output parser generator. Analyse the provided command output samples and produce a structured parsing rule."
- User: vendor name, command string, 3–5 raw output samples
- LLM must return structured JSON with:
  - `output_type`: `"table"` | `"hierarchical"` | `"raw"`
  - `schema_yaml`: declarative column/field definitions (for `"table"` type only)
  - `go_code_draft`: Go function body (for `"hierarchical"` and `"raw"` types)
  - `field_map`: inferred field names and types
  - `confidence`: 0–1 float

**Output types defined:**
- `"table"` — output has a detectable header line and fixed-column rows (e.g., `display interface brief`)
- `"hierarchical"` — output has labelled multi-line records, indented blocks, or multi-section layout (e.g., `display ospf peer verbose`)
- `"raw"` — unstructured or narrative output; parser returns `RawText` only and routes to scratch pad

**New DB table: `pending_rules`**

```sql
CREATE TABLE pending_rules (
    id               INTEGER PRIMARY KEY,
    vendor           TEXT NOT NULL,
    command_pattern  TEXT NOT NULL,
    output_type      TEXT NOT NULL CHECK(output_type IN ('table','hierarchical','raw')),
    schema_yaml      TEXT,           -- populated for output_type='table'
    go_code_draft    TEXT,           -- populated for output_type='hierarchical'|'raw'
    sample_inputs    TEXT NOT NULL,  -- JSON array of raw strings
    expected_outputs TEXT,           -- JSON array of ParseResult previews (LLM-inferred)
    confidence       REAL,
    status           TEXT NOT NULL DEFAULT 'draft'
                     CHECK(status IN ('draft','testing','approved','rejected')),
    approved_by      TEXT,           -- OS username at time of approval (os/user.Current())
    approved_at      DATETIME,
    pr_url           TEXT,
    merged_at        DATETIME,
    go_file_path     TEXT,           -- repo-relative path, e.g. internal/parser/huawei/traffic_policy.go
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER pending_rules_updated_at
AFTER UPDATE ON pending_rules
BEGIN
    UPDATE pending_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
```

---

### 5.3 Rule Studio (Web UI)

**Start command:** `nethelper rule studio [--port 7070]`
Launches an embedded HTTP server; opens `http://localhost:7070` in the default browser.

**Tech stack:**
- Backend: Go (`net/http`), reuses existing SQLite DB and parser packages
- Frontend: server-rendered HTML + HTMX. HTMX is vendored as an embedded static file (`internal/studio/static/htmx.min.js`) served via `embed.FS` — **no CDN dependency**. This ensures the tool works in air-gapped data centre environments.
- No JS framework; no new runtime dependencies

**Three views:**

#### View 1 — Draft List (`/`)
- Table of `pending_rules` where `status IN ('draft','testing')`
- Columns: vendor, command pattern, output type, confidence, occurrence count, first seen
- Sorted by occurrence count DESC (highest priority first)
- Actions: Open editor | Ignore

#### View 2 — Rule Editor (`/rule/:id`)
- Left pane: editable YAML schema (for `output_type='table'`) or Go code draft (for `hierarchical`/`raw`) — plain `<textarea>` with monospace font
- Right pane: LLM analysis summary + raw sample outputs (tab through multiple samples)
- Save button: persists edits to `pending_rules`, sets `status='testing'`

#### View 3 — Test Sandbox (`/rule/:id/sandbox`)

**Execution model:**

- **Table rules (`output_type='table'`):** The sandbox executes the rule live. The YAML schema is parsed server-side and passed to `parser.ParseTable()`. Results are rendered immediately.
- **Hierarchical/raw rules (`output_type='hierarchical'|'raw'`):** The sandbox shows a **read-only preview** of the Go code draft. Live execution is not available because running arbitrary generated Go code requires compilation. A notice is shown: "Validation happens after the PR merges and CI runs." Engineers can still save test cases (input + manually-entered expected output) and approve.

**Sandbox UI:**
- Input area: paste any real device output
- Run button: for table rules, calls `/api/rule/:id/test` and renders parsed fields with colour highlights; disabled for Go code rules (shows preview notice instead)
- "Save as test case" button: stores input + expected output into `rule_test_cases` table
- "Approve" button: enabled when ≥1 test case is saved; triggers Code Generator

**New DB table: `rule_test_cases`**

```sql
CREATE TABLE rule_test_cases (
    id          INTEGER PRIMARY KEY,
    rule_id     INTEGER NOT NULL REFERENCES pending_rules(id) ON DELETE CASCADE,
    description TEXT,
    input       TEXT NOT NULL,
    expected    TEXT NOT NULL,   -- JSON of model.ParseResult
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

---

### 5.4 `model.ParseResult` Extensibility Policy

Generated parsers must return `model.ParseResult`. The existing struct has 7 structured data fields (`Interfaces`, `RIBEntries`, `FIBEntries`, `LFIBEntries`, `Neighbors`, `Tunnels`, `SRMappings`).

**Policy:**

- **Structured mapping:** If the discovered command maps cleanly to an existing `ParseResult` field (e.g., a new interface-listing command populates `Interfaces`), the generated parser populates that field. This is the ideal outcome.
- **Raw/scratch fallback:** If the output doesn't map to any existing field, the generated parser sets only `RawText`. The pipeline routes this to `scratch_entries` (existing path in `pipeline.go`). The data is not lost; it is searchable via `nethelper search command`.
- **New data category (out of scope):** Adding a net-new `ParseResult` field and corresponding DB table (e.g., a new struct for traffic policy statistics) requires human engineering work and is explicitly out of scope for the Rule Studio feature. Rule Studio generates parsers within the existing data model only.

This policy prevents silent data loss and sets clear expectations for both the LLM prompt and the Code Generator templates.

---

### 5.5 Code Generator

**Trigger:** Engineer clicks Approve in Rule Studio.

**Pre-flight checks:**
1. `gh` CLI is in `$PATH` — if not, print actionable error: `"gh CLI not found. Install from https://cli.github.com and run 'gh auth login' before approving rules."`
2. `gh auth status` returns success — if not, print: `"gh CLI not authenticated. Run 'gh auth login'."`
3. Working directory is a clean git repo with a remote configured

**Steps:**

1. Load `pending_rules` record + all `rule_test_cases` for the rule
2. Determine target file path (repo-relative): `internal/parser/<vendor>/<cmd_name>.go`
3. Generate parser file from template:
   - Table type → calls schema-driven generic engine (`parser.ParseTable(schema, raw)`)
   - Complex type → embeds the Go code draft as the function body
   - File header includes auto-generated notice, command name, approver, date
   - Function signature: `func Parse<Vendor><CmdName>(raw string) (model.ParseResult, error)`
4. Generate test file: one `TestParse<Vendor><CmdName>` per `rule_test_cases` row
5. **Patch `<vendor>_generated.go`** (see §5.6 — never touch the handwritten `<vendor>.go`)
6. `git checkout -b rule/<vendor>-<cmd_name>-<rule_id>`
7. `git add` the two new files + the patched `_generated.go`
8. `git commit -m "feat(parser): add <vendor> parser for <command>"`
9. `gh pr create` with auto-filled description (command, samples, occurrence count, approver)
10. Update `pending_rules`: set `pr_url` (repo-relative `go_file_path`), `approved_by` (from `os/user.Current().Username`), `approved_at`

**`nethelper rule regen <rule-id>` — post-merge safety:**
- If `pending_rules.merged_at` is set, print a warning: `"WARNING: this rule was merged at <date>. Regenerating will overwrite any post-merge changes. Run with --force to proceed."`
- With `--force`, show a diff between the current file on disk and what would be generated, then proceed

**Generated file header comment:**
```go
// Code generated by nethelper rule-studio. DO NOT EDIT.
// Command:   display traffic-policy statistics interface
// Vendor:    huawei
// Rule ID:   42
// Approved:  2026-03-25 by zhangsan
// Regenerate: nethelper rule regen 42
```

---

### 5.6 ClassifyCommand Dispatch Strategy (`_generated.go`)

**Decision (locked):** The Code Generator never patches the handwritten `<vendor>.go` file. Auto-patching Go source with regex or AST is fragile and risks corrupting the existing parser.

**Instead:** Each vendor gets a generated companion file `<vendor>_generated.go` in the same package. This file contains:

```go
// Code generated by nethelper rule-studio. DO NOT EDIT.
package huawei

import "github.com/xavierli/nethelper/internal/model"

// classifyGenerated returns the CommandType for commands added via Rule Studio.
// Called as a fallback from ClassifyCommand() when the main switch returns CmdUnknown.
func classifyGenerated(cmd string) model.CommandType {
    switch {
    case strings.HasPrefix(cmd, "display traffic-policy"):
        return model.CmdUnknown // routed to ParseGeneratedOutput
    // ... additional generated cases
    }
    return model.CmdUnknown
}

// parseGenerated dispatches to Rule Studio-generated parsers.
func parseGenerated(cmdType model.CommandType, raw string) (model.ParseResult, error) {
    // ... generated dispatch
}
```

The handwritten `ClassifyCommand()` is modified **once, manually** (not by the generator) to add a fallback call:

```go
func (p *Parser) ClassifyCommand(cmd string) model.CommandType {
    // ... existing switch ...
    // Fallback to generated rules
    if ct := classifyGenerated(lower); ct != model.CmdUnknown {
        return ct
    }
    return model.CmdUnknown
}
```

This one-time manual change per vendor is done during P3 setup. Subsequent rule additions only touch `<vendor>_generated.go`.

---

### 5.7 Schema-Driven Table Engine

A new package `internal/parser/engine` providing:

```go
type ColumnDef struct {
    Name     string
    Index    int      // 0-based column position, -1 = derive from header
    Type     string   // "string" | "int" | "ip" | "duration" | "bytes"
    Optional bool
}

type TableSchema struct {
    HeaderPattern string      // regex to detect header line
    SkipLines     int         // lines to skip after header
    Columns       []ColumnDef
}

func ParseTable(schema TableSchema, raw string) (model.ParseResult, error)
```

YAML is the editing format in Rule Studio. The Code Generator **transpiles YAML to Go struct literals** embedded in the generated file. The YAML is not loaded at runtime — the schema is compiled into the binary.

---

## 6. Data Flow Summary

```
ingest unknown output
  → unknown_outputs (status=new)
  → [offline] discovery engine clusters + LLM analyses
  → pending_rules (status=draft)
  → engineer opens Rule Studio
  → edits schema/code, tests in sandbox (live for table rules; preview for Go code)
  → saves test cases
  → approves (pre-flight checks pass)
  → code generator runs
  → PR opened on GitHub
  → developer reviews + merges
  → CI runs: go test ./internal/parser/...
  → next build includes new parser
```

---

## 7. Integration Points with Existing Code

| Existing symbol | Change required |
|---|---|
| `pipeline.processBlocks()` | Add `UnknownOutputCollector.Collect(block)` call for `CmdUnknown` blocks |
| `<vendor>.go ClassifyCommand()` | **One-time manual** addition of fallback call to `classifyGenerated()` (per vendor, done at P3 setup) |
| `<vendor>.go ParseOutput()` | **One-time manual** addition of fallback call to `parseGenerated()` |
| `<vendor>_generated.go` | **New file per vendor**, created and maintained by Code Generator |
| `store.DB` | Add migrations for 3 new tables + 1 trigger |
| `internal/cli/root.go` | Register `newRuleCmd()` with `studio`, `discover`, `regen`, `history`, `list` subcommands |
| `internal/parser/engine/` | New package: `ParseTable()` + schema types |
| `internal/studio/static/` | New embed.FS with `htmx.min.js` and HTML templates |
| `gh` CLI | **External dependency** — must be installed and authenticated; pre-flight check required |

---

## 8. CLI Commands

```bash
# Start Rule Studio web UI
nethelper rule studio [--port 7070]

# Run discovery engine once (offline)
nethelper rule discover [--vendor huawei] [--limit 20]

# Regenerate Go files for an already-approved rule (after schema edit)
nethelper rule regen <rule-id> [--force]

# Show history for a command
nethelper rule history huawei "display traffic-policy"

# List pending rules (terminal, no UI)
nethelper rule list [--status draft|testing|approved|rejected]
```

---

## 9. Testing Strategy

- **Unit tests** for `UnknownOutputCollector`, Discovery Engine clustering logic, `CodeGenerator` template output, `ParseTable()` engine
- **Integration tests** for the full flow: inject unknown block → run discover → verify `pending_rules` row created
- **Generated `_test.go` files** from Rule Studio form the regression suite for each new parser; samples saved in the sandbox become the test inputs
- **`go test ./internal/parser/...`** in CI covers all generated parsers automatically
- **Table engine tests** in `internal/parser/engine/` are independent of vendor parsers

---

## 10. Phased Delivery

| Phase | Scope | Outcome | Dependencies |
|---|---|---|---|
| **P0** | `unknown_outputs` table + Collector | Pipeline silently captures unknown outputs | None |
| **P1** | Discovery Engine + `pending_rules` | LLM drafts appear in DB | P0 |
| **P2** | Rule Studio UI (list + editor + sandbox) | Engineers can review and test (table: live; Go code: preview) | P1 |
| **P3** | Schema-driven table engine (`internal/parser/engine`) | Table-type rules can be executed in sandbox + code-generated | P2 |
| **P4** | Code Generator + PR automation | Full approve→PR flow | P2, P3 |

Note: P3 (table engine) is a dependency of P4 (code generator for table-type rules). P4 can generate Go code draft parsers without P3, but table-type code generation requires P3 complete.

---

## 11. Out-of-Scope: New Vendor Discovery

When `Split()` fails to match any prompt, the current pipeline calls `detectRawConfig()` as a fallback. If that also fails, the content is silently dropped. Capturing and discovering new vendor formats (unknown prompt structures) is a substantially different problem than discovering new commands for known vendors.

**Decision:** New vendor discovery is out of scope for this feature. The existing `BlocksSkipped` counter covers this silent drop. A future `vendor-discovery` feature should address it separately, potentially starting with a `unknown_vendor_outputs` collector analogous to §5.1.

---

## 12. Resolved Design Decisions

| Decision | Resolution |
|---|---|
| Sandbox execution for Go code drafts | Table rules: live execution via `ParseTable()`. Go code drafts: read-only preview only. |
| `model.ParseResult` for novel data | Structured mapping if possible; raw/scratch fallback otherwise; new data categories are out of scope. |
| ClassifyCommand patching | `_generated.go` per vendor + one-time manual fallback hook in handwritten files. |
| Background discovery scope | Goroutine lives inside `rule studio` server only. Not during `watch ingest`. |
| HTMX delivery | Vendored as embedded static file via `embed.FS`. No CDN. |
| `gh` CLI dependency | Declared external dep; pre-flight check before any PR operation. |
| `approved_by` identity | `os/user.Current().Username`; overridable via `--approver` flag. |
| `command_norm` | Vendor abbreviation expansion + lowercase + whitespace collapse. |
| `go_file_path` storage | Repo-relative path (e.g. `internal/parser/huawei/traffic_policy.go`). |
| New vendor discovery | Out of scope; addressed by future feature. |
