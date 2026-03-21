# Integration Testing & LLM Optimization Design

**Date:** 2026-03-21
**Status:** Approved
**Author:** Claude Code (network engineer + test engineer role)

---

## Background

The project has 29 unit tests with inline fixtures but no integration tests against real device logs. Real session logs from `/Users/xavierli/Work/session_log/netdevice` reveal several parsing gaps. Additionally, the LLM-powered `explain` and `diagnose` commands pass excessive context to the model, and simple data queries that could be answered by direct DB lookups are needlessly routed through LLM.

---

## Part 1: Integration Testing

### Approach

Test-driven: write tests first (expected to fail), fix bugs revealed by real logs, then verify all assertions pass. This gives maximum confidence that every fix is verified.

### Directory Structure

```
test/integration/
├── testdata/
│   ├── huawei/    teg_20260321162156.log   (CE12816 + NE40E, dis cur)
│   ├── h3c/       teg_20260321163710.log   (H12516XAF, Comware 7, dis cur)
│   ├── cisco/     teg_20260321162808.log   (ASR9912 IOS-XR, show running-config)
│   └── juniper/   teg (1)_20260321162932.log  (MX960, show configuration + display set)
├── ingest_test.go     # pipeline layer: timestamp strip → parse → store
├── cli_test.go        # CLI layer: watch ingest → show/trace/check/diff command chain
└── helpers_test.go    # shared helpers: temp DB setup, run command, table assertions
```

All files carry `//go:build integration` build tag.

```bash
go test -tags=integration -v ./test/integration/
```

### Test Flow (TDD Order)

1. Write failing tests
2. Fix bugs (see below)
3. Run structural assertions → green
4. Add semantic assertions (network engineer hardcodes expected values from logs)
5. Run CLI command chain assertions → green

### Assertion Layers

**Structural** (count/type checks, no hardcoded values):
- After ingest: `snapshots` count ≥ 1, device count ≥ 1, interface count ≥ 1
- `captured_at` on snapshot is non-zero and ≤ ingestion time
- CommandBlock count matches expected number of distinct commands in the log

**Semantic** (network engineer reads logs and hardcodes):

| Log | Expected device | Expected fields |
|-----|----------------|-----------------|
| huawei/teg_20260321162156.log | GZ-HXY-G160304-B02-HW12816-CUF-13 | vendor=huawei, OS=VRP |
| huawei/teg_20260321162156.log | CD-GX-0402-J20-NE40E-BR-01 | second device in same log |
| h3c/teg_20260321163710.log | GZ-HXY-0203-C05-H12516XAF-QCDR-01 | vendor=h3c, version contains "7.1" |
| cisco/teg_20260321162808.log | GZ-YS-0101-G05-ASR9912-QCSTIX-01 | vendor=cisco, OS=IOS-XR |
| juniper/teg (1)_20260321162932.log | SZ-BH-0701-J04-MX960-QCTIX-02a | vendor=juniper, both config formats stored |

**CLI command chain** (E2E):
```
watch ingest <log>
→ show device                    (assert ≥1 row)
→ show interface --device <name> (assert ≥1 row)
→ check loop --device <name>     (assert exits 0)
→ export report --device <name>  (assert non-empty Markdown output)
```

---

## Part 2: Bugs to Fix

These are revealed by real log analysis and must be fixed for integration tests to pass.

### Bug 1: Timestamp Prefix Not Stripped (All Vendors)

Every line in real logs is prefixed: `2026-03-21-13-11-07: <hostname>command`

**Fix location:** `internal/parser/splitter.go` line preprocessing.

**Fix:** Before passing each line to `DetectPrompt`, strip the prefix with:
```
^\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}:
```

**Capture time:** Extract the timestamp from this prefix and store it as `CapturedAt time.Time` on the `CommandBlock` struct. When the pipeline creates a snapshot, use the earliest `CapturedAt` in the block as `snapshots.captured_at`. This enables troubleshooting timeline reconstruction (e.g., "OSPF peer command captured at 16:23:18, route table at 16:23:22").

**Schema change:** Add `captured_at DATETIME` column to `snapshots` table (nullable for backwards compatibility). Existing tests unaffected.

### Bug 2: Cisco IOS-XR Prompt Not Recognized

Real prompt: `RP/0/RP0/CPU0:GZ-YS-0101-G05-ASR9912-QCSTIX-01#`

Current Cisco parser only matches `hostname#` (IOS/IOS-XE format).

**Fix:** Update `internal/parser/cisco/cisco.go` `DetectPrompt` to also match:
```
^RP/\d+/RP\d+/CPU\d+:([^#]+)#\s*$
```
Return the hostname portion after `:`.

**Classify:** `show running-config` → `CmdConfig` (already handled). No new command types needed.

### Bug 3: H3C vs Huawei Disambiguation

Both use `<hostname>` prompt. The current `parser.Registry` tries parsers in registration order—if Huawei is registered first, it claims all `<hostname>` prompts.

**Fix:** H3C parser's `DetectPrompt` already works (it matches `<hostname>`). The disambiguation happens in `ClassifyCommand`/`ParseOutput` by inspecting the config header:
- Huawei config header: `!Software Version V200R...`
- H3C Comware 7 header: `version 7.x.xxx, Release XXXX` + `mdc Admin id`

Update `internal/parser/h3c/h3c.go` to detect vendor via config content heuristics in `ParseOutput`, or add a `DetectVendor(firstLines string) bool` method to the `VendorParser` interface (preferred—makes disambiguation explicit and testable).

### Bug 4: Juniper Dual Config Format

Two distinct command outputs in the same session:
- `show configuration` → hierarchical JunOS format (`{ }` + `;`)
- `show configuration | display set` → flat set-command format (`set x y z`)

**Fix:** Add `CmdConfigSet` to the `CommandType` enum. Update `internal/parser/juniper/juniper.go`:
- `ClassifyCommand("show configuration | display set")` → `CmdConfigSet`
- `ParseOutput(CmdConfigSet, raw)` → store as `config_snapshots` with `format="set"`
- `ClassifyCommand("show configuration")` → `CmdConfig` (hierarchical, `format="hierarchical"`)

Add `format VARCHAR(20)` column to `config_snapshots` table (nullable, default `"hierarchical"`).

---

## Part 3: LLM Optimization

### 3.1 Intent Routing

Add `internal/llm/intent/` package with a `Classifier` that maps natural language queries to `QueryIntent` before the LLM is consulted.

```go
type QueryIntent int
const (
    IntentComplex       QueryIntent = iota // → LLM
    IntentDeviceList                       // → store.ListDevices()
    IntentInterfaceStatus                  // → store.GetInterfaces()
    IntentRouteTable                       // → store.GetRIB() or scratch
    IntentNeighborList                     // → store.GetNeighbors()
    IntentConfigSearch                     // → FTS5 fts_config
)
```

**Classification rules** (keyword + sentence pattern matching, no LLM):

| Keywords present | Sentence pattern | → Intent |
|-----------------|-----------------|---------|
| 显示/列出/查看 + 设备 | — | DeviceList |
| 显示/列出/查看 + 接口/端口 | — | InterfaceStatus |
| 显示/列出/查看 + 路由/路由表 | — | RouteTable |
| 显示/列出/查看 + 邻居/neighbor | — | NeighborList |
| 搜索/查找 + 配置 | — | ConfigSearch |
| 为什么/是否/分析/诊断 | — | Complex |
| 经过/影响/如果/假设/比较 | multi-hop relational | Complex |
| (none matched) | — | Complex (safe fallback) |

In `explain.go`:
```
Classify(query)
  → IntentComplex: gatherContext() → LLM call (existing path, but compressed)
  → IntentXxx: directQuery(intent, deviceID) → format table → return (no LLM)
```

### 3.2 Context Compression

Current `gatherContext()` sends up to 8000 chars of config + all interfaces + all neighbors regardless of the question. Replace with intent-aware context building:

| Question intent | What gets included | Hard cap |
|----------------|-------------------|---------|
| OSPF question | OSPF neighbors + interfaces with OSPF config sections | 4000 chars total |
| BGP question | BGP neighbors + routing-policy + BGP config sections | 4000 chars total |
| MPLS/LSP question | LFIB entries + LDP sessions + MPLS config sections | 4000 chars total |
| General troubleshoot | Interface summary + 3 most recent scratch entries | 3000 chars total |
| Unknown | Interface summary + top config sections by keyword match | 5000 chars total |

**Implementation:** Add `buildContext(query string, intent QueryIntent, deviceID string) string` that replaces the existing `gatherContext`. Each intent has its own context builder function. Config section extraction stays keyword-based but with per-intent keyword sets and tighter per-section caps (500 chars vs current 8000 for the whole thing).

**Expected token savings:** 40–70% reduction for OSPF/BGP/MPLS questions (currently pulls everything).

---

## Implementation Order

1. Fix Bug 1 (timestamp strip + `CapturedAt`) — unlocks all other parsing
2. Fix Bug 2 (IOS-XR prompt) — enables Cisco integration tests
3. Fix Bug 3 (H3C disambiguation) — enables H3C integration tests
4. Fix Bug 4 (Juniper dual format) — enables Juniper integration tests
5. Write `test/integration/ingest_test.go` — structural + semantic assertions
6. Write `test/integration/cli_test.go` — E2E CLI command chain
7. Add `IntentClassifier` (`internal/llm/intent/`)
8. Replace `gatherContext` with `buildContext` (intent-aware)
9. Wire intent routing into `explain.go`

---

## Non-Goals

- No mock LLM in integration tests (LLM commands not tested in integration suite)
- No golden-file snapshot comparison (structural + semantic assertions are sufficient)
- No new vendor parsers beyond fixing existing four
- No embedding / vector search changes
