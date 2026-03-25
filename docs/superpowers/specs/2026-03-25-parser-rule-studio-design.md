# Parser Rule Studio — Design Spec

**Date:** 2026-03-25
**Status:** Approved (brainstorm)
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
│  patch ClassifyCommand() + ParseOutput() dispatch       │
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
- Deduplicates by `(vendor, command_normalised)` + content hash; increments `occurrence_count` on repeat

**New DB table: `unknown_outputs`**

```sql
CREATE TABLE unknown_outputs (
    id              INTEGER PRIMARY KEY,
    device_id       TEXT NOT NULL,
    vendor          TEXT NOT NULL,
    command_raw     TEXT NOT NULL,
    command_norm    TEXT NOT NULL,   -- lowercased, whitespace-collapsed
    raw_output      TEXT NOT NULL,
    content_hash    TEXT NOT NULL,   -- SHA256 of raw_output
    first_seen      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'new'
                    CHECK(status IN ('new','clustered','promoted','ignored'))
);
CREATE INDEX idx_unknown_vendor_cmd ON unknown_outputs(vendor, command_norm, status);
```

---

### 5.2 Discovery Engine

**Trigger:** Offline. Two modes:
- `nethelper rule discover` — manual, on-demand
- Background goroutine on a configurable interval (default: disabled, opt-in via config)

**Clustering:**
1. Group `unknown_outputs` where `status='new'` by `(vendor, command_norm)`
2. Within each group, sub-cluster by structural signature: presence of header line, column count, indentation depth, separator pattern
3. Select up to 5 representative samples per cluster (most recent + most distinct)
4. Mark selected records `status='clustered'`

**LLM Prompt Strategy:**
- System: "You are a network CLI output parser generator. Analyse the provided command output samples and produce a structured parsing rule."
- User: vendor name, command string, 3–5 raw output samples
- LLM must return structured JSON with:
  - `output_type`: `"table"` | `"mixed"` | `"hierarchical"` | `"raw"`
  - `schema_yaml`: declarative column/field definitions (for table type)
  - `go_code_draft`: Go function body (for mixed/hierarchical/raw)
  - `field_map`: inferred field names and types
  - `confidence`: 0–1 float

**New DB table: `pending_rules`**

```sql
CREATE TABLE pending_rules (
    id               INTEGER PRIMARY KEY,
    vendor           TEXT NOT NULL,
    command_pattern  TEXT NOT NULL,
    output_type      TEXT NOT NULL,
    schema_yaml      TEXT,
    go_code_draft    TEXT,
    sample_inputs    TEXT NOT NULL,   -- JSON array of raw strings
    expected_outputs TEXT,            -- JSON array of ParseResult previews
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
);
```

---

### 5.3 Rule Studio (Web UI)

**Start command:** `nethelper rule studio [--port 7070]`
Launches an embedded HTTP server; opens `http://localhost:7070` in the default browser.

**Tech stack:**
- Backend: Go (`net/http`), reuses existing SQLite DB and parser packages
- Frontend: server-rendered HTML + HTMX for interactivity; no JS framework
- No external dependencies beyond what's already in `go.mod`

**Three views:**

#### View 1 — Draft List (`/`)
- Table of `pending_rules` where `status IN ('draft','testing')`
- Columns: vendor, command pattern, output type, confidence, occurrence count, first seen
- Sorted by occurrence count DESC (highest priority first)
- Actions: Open editor | Ignore

#### View 2 — Rule Editor (`/rule/:id`)
- Left pane: editable YAML schema or Go code draft (CodeMirror or `<textarea>`)
- Right pane: LLM analysis summary + raw sample outputs (tab through multiple samples)
- Save button: persists edits to `pending_rules`, sets `status='testing'`

#### View 3 — Test Sandbox (`/rule/:id/sandbox`)
- Input area: paste any real device output
- Run button: executes current rule against pasted input (via `/api/rule/:id/test` endpoint)
- Result panel: parsed fields displayed as a table with colour highlights
- "Save as test case" button: stores input + expected output into `rule_test_cases` table
- "Approve" button: triggers Code Generator if ≥1 test case saved

**New DB table: `rule_test_cases`**

```sql
CREATE TABLE rule_test_cases (
    id          INTEGER PRIMARY KEY,
    rule_id     INTEGER NOT NULL REFERENCES pending_rules(id),
    description TEXT,
    input       TEXT NOT NULL,
    expected    TEXT NOT NULL,   -- JSON of model.ParseResult
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

---

### 5.4 Code Generator

**Trigger:** Engineer clicks Approve in Rule Studio.

**Steps:**

1. Load `pending_rules` record + all `rule_test_cases` for the rule
2. Determine target file path: `internal/parser/<vendor>/<cmd_name>.go`
3. Generate parser file from template:
   - Table type → calls schema-driven generic engine (`parser.ParseTable(schema, raw)`)
   - Complex type → embeds the Go code draft as the function body
   - File header includes auto-generated notice, command name, approver, date
   - Function signature: `func Parse<Vendor><CmdName>(raw string) (model.ParseResult, error)`
4. Generate test file: one `TestParse<Vendor><CmdName>` per `rule_test_cases` row
5. Patch `<vendor>.go`:
   - Add case to `ClassifyCommand()` switch
   - Add case to `ParseOutput()` switch calling the new function
6. `git checkout -b rule/<vendor>-<cmd_name>-<rule_id>`
7. `git add` the two new files + the patched vendor file
8. `git commit -m "feat(parser): add <vendor> parser for <command>"`
9. `gh pr create` with auto-filled description (command, samples, occurrence count, approver)
10. Update `pending_rules`: set `pr_url`, `status` remains `approved`

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

### 5.5 Schema-Driven Table Engine

A new package `internal/parser/engine` providing:

```go
type ColumnDef struct {
    Name     string
    Index    int      // 0-based column position, -1 = derive from header
    Type     string   // "string" | "int" | "ip" | "duration" | "bytes"
    Optional bool
}

type TableSchema struct {
    HeaderPattern string        // regex to detect header line
    SkipLines     int           // lines to skip after header
    Columns       []ColumnDef
}

func ParseTable(schema TableSchema, raw string) (model.ParseResult, error)
```

This is what generated code calls for simple table outputs. The schema is embedded as a struct literal in the generated Go file — not loaded from YAML at runtime. YAML is only used in Rule Studio for editing; the code generator transpiles it to Go struct literals.

---

## 6. Data Flow Summary

```
ingest unknown output
  → unknown_outputs (status=new)
  → [offline] discovery engine clusters + LLM analyses
  → pending_rules (status=draft)
  → engineer opens Rule Studio
  → edits schema/code, tests in sandbox
  → saves test cases
  → approves
  → code generator runs
  → PR opened on GitHub
  → developer reviews + merges
  → CI runs go test ./internal/parser/...
  → next build includes new parser
```

---

## 7. Integration Points with Existing Code

| Existing symbol | Change required |
|---|---|
| `pipeline.processBlocks()` | Add `UnknownOutputCollector.Collect(block)` call for `CmdUnknown` blocks |
| `<vendor>.go ClassifyCommand()` | Code Generator patches in new `case` |
| `<vendor>.go ParseOutput()` | Code Generator patches in new `case` |
| `store.DB` | Add migrations for 3 new tables |
| `internal/cli/root.go` | Register `newRuleCmd()` with `studio`, `discover`, `regen`, `history` subcommands |
| `go.mod` | Add HTMX (CDN, no build dependency); no other new deps required |

---

## 8. CLI Commands

```bash
# Start Rule Studio web UI
nethelper rule studio [--port 7070]

# Run discovery engine once (offline)
nethelper rule discover [--vendor huawei] [--limit 20]

# Regenerate Go files for an already-approved rule (after schema edit)
nethelper rule regen <rule-id>

# Show history for a command
nethelper rule history huawei "display traffic-policy"

# List pending rules (terminal, no UI)
nethelper rule list [--status draft|testing|approved|rejected]
```

---

## 9. Testing Strategy

- **Unit tests** for `UnknownOutputCollector`, `Discovery Engine` clustering logic, `CodeGenerator` template output
- **Integration tests** for the full flow: inject unknown block → run discover → verify `pending_rules` row created
- **Generated test files** form the regression suite for each new parser
- **`go test ./internal/parser/...`** in CI covers all generated parsers automatically

---

## 10. Phased Delivery

| Phase | Scope | Outcome |
|---|---|---|
| **P0** | `unknown_outputs` table + Collector | Pipeline silently captures unknown outputs |
| **P1** | Discovery Engine + `pending_rules` | LLM drafts appear in DB |
| **P2** | Rule Studio UI (list + editor + sandbox) | Engineers can review and test |
| **P3** | Code Generator + PR automation | Full approve→PR flow |
| **P4** | Schema-driven table engine | Reduce LLM Go code for simple tables |

Each phase is independently useful and deployable.

---

## 11. Open Questions

1. **Discovery trigger:** Should the background discovery goroutine be on-by-default or opt-in? (Recommendation: opt-in — avoids surprise LLM API calls)
2. **New vendor detection:** When `Split()` can't match any prompt, how do we capture the raw content for vendor discovery? (May need a separate "unrecognised vendor" collector path)
3. **ClassifyCommand patching:** Auto-patching existing Go source is fragile. Alternative: generate a separate `<vendor>_generated.go` file with a `ClassifyCommandGenerated()` helper that the main `ClassifyCommand()` delegates to as a fallback. Safer, avoids touching existing files.
4. **Rule Studio auth:** Local tool, no auth needed for now. If ever exposed on a network interface, add basic token auth.
