# Integration Testing & LLM Optimization Design (v2)

**Date:** 2026-03-21
**Status:** Draft — pending review
**Revised by:** Claude Opus (spec review corrections)

---

## Background

The project has 29 unit tests with inline fixtures but no integration tests against real device logs. Real session logs (copied to `test/integration/testdata/`) reveal several parsing gaps. Additionally, the LLM-powered `explain` and `diagnose` commands pass excessive context to the model, and simple data queries that could be answered by direct DB lookups are needlessly routed through LLM.

This revision corrects factual errors in v1 where existing code was described as missing, and proposes more accurate fix strategies.

---

## Part 1: Integration Testing

### Approach

True TDD — for each bug, write the failing test first, fix the bug, verify green, then move to the next bug. Tests and fixes are interleaved, not written in separate phases.

### Directory Structure

```
test/integration/
├── testdata/
│   ├── huawei/    teg_20260321162156.log   (CE12816 + NE40E, dis cur | no-more)
│   ├── h3c/       teg_20260321163710.log   (H12516XAF, Comware 7, dis cur)
│   ├── cisco/     teg_20260321162808.log   (ASR9912 IOS-XR, show running-config)
│   └── juniper/   "teg (1)_20260321162932.log"  (MX960, show configuration + | display set)
├── ingest_test.go     # pipeline layer: parse → store
├── cli_test.go        # CLI layer: watch ingest → show/trace/check/diff command chain
└── helpers_test.go    # shared helpers: temp DB setup, run command, table assertions
```

All files carry `//go:build integration` build tag.

```bash
go test -tags=integration -v ./test/integration/
```

**Note:** The Juniper filename contains a space (`teg (1)_...`). Go `filepath` handles this correctly, but any shell-out in tests must quote the path.

### Assertion Layers

**Structural** (count/type checks, no hardcoded values):
- After ingest: `snapshots` count ≥ 1, device count ≥ 1
- `captured_at` on snapshot is the extracted log timestamp (not ingestion time)
- CommandBlock count matches expected number of distinct prompt lines in the log

**Semantic** (network engineer reads logs and hardcodes expected values):

| Log | Expected device | Key assertions |
|-----|----------------|----------------|
| huawei/teg_20260321162156.log | GZ-HXY-G160304-B02-HW12816-CUF-13 | vendor=huawei, config starts with `!Software Version V200R020C10SPC600` |
| huawei/teg_20260321162156.log | CD-GX-0402-J20-NE40E-BR-01 | second device in same log, separate snapshot |
| h3c/teg_20260321163710.log | GZ-HXY-0203-C05-H12516XAF-QCDR-01 | vendor=h3c (not huawei!), config contains `version 7.1.070` |
| cisco/teg_20260321162808.log | GZ-YS-0101-G05-ASR9912-QCSTIX-01 | vendor=cisco, prompt matched via IOS-XR pattern |
| juniper/teg (1)_20260321162932.log | SZ-BH-0701-J04-MX960-QCTIX-02a | vendor=juniper, two config snapshots (hierarchical + set format) |

**Negative assertions:**
- No device created for `CQ-TH-M3103-V06-MX960-BR-601a` (Juniper log has a failed SSH to this host — password error, no valid prompts)
- No device created for shell prompts (`[xavierli@...]`)
- Cisco `show running-config ?` (help query) should not create a config snapshot — only the actual `show running-config` on line 208 should

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

### Bug 1: Timestamp Extraction (Not Stripping — Stripping Already Works)

**What v1 got wrong:** v1 said timestamps aren't stripped. They are — `splitter.go` already has `timestampRe` and `stripTimestamp()` that strip the `2026-03-21-16-22-26: ` prefix before prompt detection (line 57) and from output lines (line 77).

**What's actually missing:** The timestamp *value* is discarded. `CommandBlock` has no `CapturedAt` field, so the pipeline can't pass the real capture time to `CreateSnapshot`.

**Fix:**
1. Add `CapturedAt time.Time` field to `CommandBlock` struct (`internal/parser/types.go`)
2. In `Split()` (`internal/parser/splitter.go`): extract the timestamp from `timestampRe` match before stripping it, parse to `time.Time`, store on the `CommandBlock`
3. In `Pipeline.Ingest()` (`internal/parser/pipeline.go`): when creating a snapshot, pass the earliest `CapturedAt` from the device's blocks instead of relying on `DEFAULT CURRENT_TIMESTAMP`
4. Update `CreateSnapshot` in `internal/store/snapshot_store.go` to accept and INSERT the `captured_at` value (the column already exists in the schema — no migration needed)

**No schema change required** — `snapshots.captured_at` already exists in `migrations.go` line 35.

### Bug 2: Cisco IOS-XR Prompt Not Recognized

Real prompt: `RP/0/RP0/CPU0:GZ-YS-0101-G05-ASR9912-QCSTIX-01#show running-config`

Current regex: `^([A-Za-z][A-Za-z0-9._-]*)(?:\([^)]*\))?#` — only matches IOS/IOS-XE format (starts with letter).

**Fix:** Update `internal/parser/cisco/cisco.go`:
```go
// Add second regex for IOS-XR
var iosxrPromptRe = regexp.MustCompile(`^RP/\d+/RP\d+/CPU\d+:([A-Za-z][A-Za-z0-9._-]*)#`)
```
In `DetectPrompt`: try `promptRe` first, then `iosxrPromptRe`. Return `m[1]` (hostname after colon).

**Edge case in log:** Line 22 has `show running-config ?` (help query). The splitter will create a CommandBlock for this. The `?` at the end means `ClassifyCommand("show running-config ?")` should still match `CmdConfig` (prefix check). But the "output" is just the help text, not actual config. Two options:
- A) Filter: if command ends with `?`, skip the block (treat as help, not a real command)
- B) Accept: let it through, the help output won't parse as meaningful config

**Recommended:** Option A — add a check in `Pipeline.Ingest`: if `strings.HasSuffix(strings.TrimSpace(b.Command), "?")`, skip the block.

### Bug 3: H3C vs Huawei Disambiguation

Both use `<hostname>` prompt. Registration order in `root.go`: huawei → cisco → h3c → juniper. Huawei always claims `<hostname>` prompts first (line 50). H3C devices get incorrectly classified as Huawei.

**Why v1's fix won't work:**
- `DetectVendor` at prompt time is impossible — a single `<hostname>` line has no vendor-distinguishing information
- Content heuristics in `ParseOutput` is too late — vendor is already set during `Split()`

**Correct fix — post-split reclassification in `Pipeline.Ingest`:**

After `Split()` returns blocks but before `ClassifyCommand`, scan blocks claimed by Huawei that have `CmdConfig` output. Check the first 5 lines of output for H3C signatures:
- `version 7.` (H3C Comware 7 header)
- `mdc Admin id` (H3C MDC marker)

If found, reclassify the block's `Vendor` from `"huawei"` to `"h3c"`, and also reclassify all other blocks with the same hostname.

```go
// In Pipeline.Ingest, after Split() and ClassifyCommand:
func (p *Pipeline) reclassifyH3C(blocks []CommandBlock) {
    h3cHostnames := map[string]bool{}
    for _, b := range blocks {
        if b.Vendor == "huawei" && b.CmdType == model.CmdConfig {
            lines := strings.SplitN(b.Output, "\n", 6)
            for _, l := range lines {
                lower := strings.ToLower(strings.TrimSpace(l))
                if strings.HasPrefix(lower, "version 7.") || strings.HasPrefix(lower, "mdc admin id") {
                    h3cHostnames[strings.ToLower(b.Hostname)] = true
                    break
                }
            }
        }
    }
    for i := range blocks {
        if h3cHostnames[strings.ToLower(blocks[i].Hostname)] {
            blocks[i].Vendor = "h3c"
        }
    }
}
```

No interface changes. No breaking changes to other parsers.

### Bug 4: H3C `dis cur` Abbreviation Not Expanded

**Not identified in v1.** The H3C log shows:
```
<GZ-HXY-0203-C05-H12516XAF-QCDR-01>dis cur
<GZ-HXY-0203-C05-H12516XAF-QCDR-01>dis current-configuration
```

The splitter will create two CommandBlocks — one for `dis cur` and one for `dis current-configuration`. The H3C `ClassifyCommand` only matches `display current-configuration` (line 44 of h3c.go), and unlike Huawei's parser (line 46 of huawei.go), it does **not** expand `dis ` → `display `.

**Fix:** Add abbreviation expansion to H3C `ClassifyCommand`, identical to Huawei's:
```go
if strings.HasPrefix(lower, "dis ") && !strings.HasPrefix(lower, "display ") {
    lower = "display " + lower[4:]
}
```

**Additional issue:** The two-line echo (`dis cur` on line 26, `dis current-configuration` on line 27) means the splitter sees TWO prompt lines. The first block (`dis cur`) will have an empty output (the next line is another prompt). The second block (`dis current-configuration`) gets the actual config output. After abbreviation expansion, both would classify as `CmdConfig`, but only the second has real output. The empty block is harmless (zero-length output → no config stored).

### Bug 5: Juniper Dual Config Format

Two distinct command outputs in the same Juniper session:
- Line 87: `show configuration` → hierarchical JunOS format (`{ }` + `;`)
- Line 2766: `show configuration | display set` → flat set-command format (`set x y z`)

Current `ClassifyCommand` (juniper.go line 33): `strings.HasPrefix(lower, "show configuration")` matches **both** — the `| display set` variant also starts with `show configuration`. Both get `CmdConfig`. The pipe-filtered version is indistinguishable.

**Fix:**
1. Add `CmdConfigSet CommandType = "config_set"` to `internal/model/parse_result.go`
2. In `juniper.go` `ClassifyCommand`, check the pipe-filtered variant **before** the generic match:
   ```go
   case strings.Contains(lower, "| display set"): return model.CmdConfigSet
   case strings.HasPrefix(lower, "show configuration"): return model.CmdConfig
   ```
   Order matters — `CmdConfigSet` case must precede `CmdConfig`.
3. Add `format VARCHAR(20) DEFAULT 'hierarchical'` column to `config_snapshots` table (`internal/store/migrations.go`)
4. Add `Format` field to `model.ConfigSnapshot` struct
5. In `Pipeline.storeResult`, when storing `CmdConfigSet`, set `Format: "set"`

### Bug 6: Cisco `show running-config |` Pipe Expansion

The Cisco log shows (line 201):
```
RP/0/RP0/CPU0:GZ-YS-0101-G05-ASR9912-QCSTIX-01#show running-config | ?
```
This is another help query (pipe filter help). Same treatment as Bug 2 edge case — skip blocks where command ends with `?`.

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

| Keywords present | → Intent |
|-----------------|---------|
| (显示\|列出\|查看\|show\|list\|display) + (设备\|device) | DeviceList |
| (显示\|列出\|查看\|show\|list\|display) + (接口\|端口\|interface\|port) | InterfaceStatus |
| (显示\|列出\|查看\|show\|list\|display) + (路由\|路由表\|route) | RouteTable |
| (显示\|列出\|查看\|show\|list\|display) + (邻居\|neighbor\|peer) | NeighborList |
| (搜索\|查找\|search\|find\|grep) + (配置\|config) | ConfigSearch |
| (为什么\|是否\|分析\|诊断\|why\|analyze\|diagnose) | Complex |
| (经过\|影响\|如果\|假设\|比较\|impact\|what if\|compare) | Complex |
| (none matched) | Complex (safe fallback) |

**Bilingual support:** Include both Chinese and English keywords — real users mix languages (e.g., `"show me the BGP neighbors for gz-hxy"`).

In `explain.go`:
```
intent := Classify(query)
switch intent {
case IntentComplex:
    buildContext(query, intent, deviceID) → LLM call (existing path, but compressed)
default:
    directQuery(intent, deviceID) → format table → print (no LLM)
}
```

### 3.2 Context Compression

**What v1 got wrong:** v1 described this as greenfield. In fact, `explain.go` already has:
- `extractRelevantConfig()` (lines 207–294) — keyword-to-config-section mapping with `#` delimiter parsing
- Per-config truncation at 8000 chars (line 290)
- Scratch pad limiting (5 most recent, 2000 chars each, lines 157–168)

**What to improve** (refinement, not replacement):

1. **Make `extractRelevantConfig` intent-aware:** Instead of one global keyword map, provide per-intent keyword sets with tighter caps:

| Question intent | Config keywords to extract | Per-section cap | Total cap |
|----------------|---------------------------|----------------|-----------|
| OSPF question | `ospf`, `interface` (with OSPF) | 500 chars | 4000 chars |
| BGP question | `bgp`, `route-policy` | 500 chars | 4000 chars |
| MPLS/LSP question | `mpls`, `tunnel`, `segment-routing` | 500 chars | 4000 chars |
| General troubleshoot | (all keywords) | 300 chars | 3000 chars |
| Unknown | (keyword-matched) | 500 chars | 5000 chars |

2. **Reduce non-config context** for focused questions:
   - OSPF question: include only OSPF neighbors (not all protocols) and interfaces mentioned in OSPF config
   - BGP question: include only BGP peers and route-policy config
   - Currently `gatherContext` includes ALL interfaces and ALL neighbors regardless

3. **Token estimation instead of character caps:** CJK characters use more tokens than ASCII. Use rough heuristic: `estimateTokens(text) ≈ len([]rune(text)) * 1.5` for CJK-heavy text, `len(text)/4` for ASCII-heavy. Cap on estimated tokens, not raw characters.

4. **Rename `gatherContext` to `buildContext`** and add `intent QueryIntent` parameter. The existing `gatherContext` becomes the `IntentComplex` path with tighter per-section limits.

**Expected token savings:** 40–70% reduction for protocol-specific questions.

---

## Implementation Order (TDD-Interleaved)

Each step is: write test → see failure → fix → verify green.

| Step | What | Test file | Code to change |
|------|------|-----------|---------------|
| 1 | Timestamp extraction + `CapturedAt` passthrough | `ingest_test.go` (assert `captured_at` is log time, not ingestion time) | `types.go`, `splitter.go`, `pipeline.go`, `snapshot_store.go` |
| 2 | Cisco IOS-XR prompt recognition | `ingest_test.go` (assert cisco log produces device `GZ-YS-...`) | `cisco/cisco.go` |
| 3 | Skip `?` help commands | `ingest_test.go` (assert no config from `show running-config ?`) | `pipeline.go` |
| 4 | H3C disambiguation (post-split reclassification) | `ingest_test.go` (assert H3C device has `vendor=h3c`) | `pipeline.go` |
| 5 | H3C `dis` abbreviation expansion | `ingest_test.go` (assert H3C `dis cur` classifies as `CmdConfig`) | `h3c/h3c.go` |
| 6 | Juniper dual config format | `ingest_test.go` (assert 2 config snapshots with different formats) | `parse_result.go`, `juniper/juniper.go`, `migrations.go`, `pipeline.go` |
| 7 | Huawei multi-device in one log | `ingest_test.go` (assert 2 devices from huawei log) | (should work after steps 1–6; test validates) |
| 8 | CLI E2E command chain | `cli_test.go` | (should work; test validates) |
| 9 | Intent classifier | `internal/llm/intent/intent_test.go` | new `internal/llm/intent/` package |
| 10 | Context compression refactor | `explain_test.go` | `explain.go` |
| 11 | Wire intent routing | integration test for explain (mock or skip LLM) | `explain.go` |

---

## Non-Goals

- No mock LLM in integration tests (LLM commands not tested in integration suite)
- No golden-file snapshot comparison (structural + semantic assertions are sufficient)
- No new vendor parsers beyond fixing existing four
- No embedding / vector search changes
- No `VendorParser` interface changes — disambiguation is done in pipeline, not parsers

---

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Testdata files are large (54K lines total, 32K for Huawei) | Acceptable for integration tests; add `.gitattributes` note if repo grows |
| H3C reclassification requires seeing config output first | Only config-bearing sessions get reclassified; non-config H3C sessions remain misclassified as Huawei. Acceptable for now — a future `DetectVendor` interface method can solve this if needed |
| Intent classifier false positives route LLM-worthy queries to direct DB | Safe fallback to `IntentComplex`. Only trigger direct path on high-confidence keyword matches |
| `config_snapshots` has no `snapshot_id` FK | Config snapshots are tied to `device_id` only. Future work could add snapshot correlation, but not required for current goals |
