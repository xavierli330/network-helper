# Incremental Ingestion Truncation Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix watcher's incremental ingestion so it never stores truncated command output by tracking command-boundary offsets instead of raw byte offsets.

**Architecture:** Modify `Split()` to also return the byte offset of the last complete command block. Modify `IngestResult` to carry `BytesConsumed`. Modify `watch.go` to update `last_offset` based on `BytesConsumed` instead of `info.Size()`. The last incomplete block is left for the next ingestion cycle.

**Tech Stack:** Go, existing parser/watcher packages.

**Spec:** `docs/superpowers/specs/2026-03-22-incremental-ingestion-fix-design.md`

---

## File Map

| File | Action | Change |
|------|--------|--------|
| `internal/parser/splitter.go` | Modify | New `SplitResult` struct; `Split()` returns `([]CommandBlock, int)` where int = byte offset of last complete block |
| `internal/parser/splitter_test.go` | Create | Tests for truncation handling |
| `internal/parser/pipeline.go` | Modify | `IngestResult.BytesConsumed` field; pass through from Split |
| `internal/cli/watch.go` | Modify | Use `result.BytesConsumed` for `last_offset` |

---

## Task 1: Modify Split to return consumed byte offset

**Files:**
- Modify: `internal/parser/splitter.go`
- Create: `internal/parser/splitter_test.go`

- [ ] **Step 1: Write failing test for truncation detection**

```go
// internal/parser/splitter_test.go
package parser

import "testing"

// newTestRegistry creates a minimal registry with Huawei-style prompt detection.
func newTestRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(newPromptOnlyParser("huawei", `^<([A-Za-z][A-Za-z0-9._-]+)>`))
	return reg
}

func TestSplit_LastIncompleteBlockExcluded(t *testing.T) {
	// Simulate: two complete commands, then a third whose output is cut off (no next prompt)
	raw := "<DeviceA>display version\nVersion 7.1.070\n<DeviceA>display current-configuration\n#\ninterface GE0/0/1\n ip address 10.0.0.1 255.255.255.252\n"
	// Note: no trailing prompt after "display current-configuration" output — it's incomplete

	reg := newTestRegistry()
	blocks, consumed := SplitWithOffset(raw, reg)

	// Should return only 1 complete block (display version), not the incomplete config block
	if len(blocks) != 1 {
		t.Fatalf("expected 1 complete block, got %d", len(blocks))
	}
	if blocks[0].Command != "display version" {
		t.Errorf("expected 'display version', got %q", blocks[0].Command)
	}

	// consumed should point to the start of the second prompt line ("<DeviceA>display current-configuration")
	// so that the next read re-processes the incomplete block
	if consumed <= 0 {
		t.Errorf("consumed should be > 0, got %d", consumed)
	}
	if consumed >= len(raw) {
		t.Errorf("consumed (%d) should be less than len(raw) (%d) — last block is incomplete", consumed, len(raw))
	}
}

func TestSplit_AllBlocksComplete(t *testing.T) {
	// All blocks have a following prompt — all are complete
	raw := "<DeviceA>display version\nVersion 7.1.070\n<DeviceA>display clock\n10:00:00 2026-03-22\n<DeviceA>display interface brief\nGE0/0/1 up up\n"

	reg := newTestRegistry()
	blocks, consumed := SplitWithOffset(raw, reg)

	// All 3 blocks should be returned (last one ends at EOF, which is OK for non-incremental)
	// But for incremental safety: only first 2 are "complete" (bounded by next prompt)
	// The 3rd block has no following prompt — excluded
	if len(blocks) != 2 {
		t.Fatalf("expected 2 complete blocks, got %d", len(blocks))
	}
	if consumed >= len(raw) {
		t.Errorf("consumed should be < len(raw) since last block has no terminating prompt")
	}
}

func TestSplit_SingleBlockReturnsNothing(t *testing.T) {
	// Only one prompt, no next prompt — the single block is incomplete
	raw := "<DeviceA>display current-configuration\n#\nversion 7.1\n"

	reg := newTestRegistry()
	blocks, consumed := SplitWithOffset(raw, reg)

	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks (single incomplete), got %d", len(blocks))
	}
	if consumed != 0 {
		t.Errorf("consumed should be 0, got %d", consumed)
	}
}

func TestSplit_EmptyInput(t *testing.T) {
	reg := newTestRegistry()
	blocks, consumed := SplitWithOffset("", reg)
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty input, got %d", len(blocks))
	}
	if consumed != 0 {
		t.Errorf("consumed should be 0 for empty input, got %d", consumed)
	}
}

// TestSplitBackwardsCompat verifies that the existing Split() still works
// (returns all blocks including the last one, for non-incremental use).
func TestSplit_OriginalBehaviorPreserved(t *testing.T) {
	raw := "<DeviceA>display version\nVersion 7.1.070\n<DeviceA>display clock\n10:00:00\n"

	reg := newTestRegistry()
	blocks := Split(raw, reg)

	// Original Split should return all blocks including last (for manual ingest)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks from original Split, got %d", len(blocks))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/parser/ -v -run TestSplit_Last`
Expected: FAIL (SplitWithOffset undefined)

- [ ] **Step 3: Implement SplitWithOffset**

In `internal/parser/splitter.go`, add a new function `SplitWithOffset` that returns `([]CommandBlock, int)`. The int is the byte offset in `raw` where the last complete block ends (= the byte position of the last prompt that has a following prompt).

```go
// SplitWithOffset splits raw CLI output into command blocks and returns
// the byte offset of the last fully-bounded block. Blocks whose output
// may be incomplete (the last prompt with no following prompt) are excluded.
// This is used by incremental ingestion to avoid storing truncated output.
//
// Returns (completeBlocks, consumedBytes).
// consumedBytes is the byte offset in raw up to (but not including) the
// last prompt line that has no terminating prompt after it.
// If there are 0 or 1 prompts, consumedBytes is 0 (nothing is safe to consume).
func SplitWithOffset(raw string, registry *Registry) ([]CommandBlock, int) {
	if strings.TrimSpace(raw) == "" {
		return nil, 0
	}

	lines := strings.Split(raw, "\n")
	parsers := registry.Parsers()
	var matches []promptMatch

	// Track byte offsets for each line start
	lineByteOffsets := make([]int, len(lines)+1)
	offset := 0
	for i, line := range lines {
		lineByteOffsets[i] = offset
		offset += len(line) + 1 // +1 for the \n
	}
	lineByteOffsets[len(lines)] = len(raw)

	// Phase 1: Detect all prompts (same as Split)
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r \t")
		if trimmed == "" {
			continue
		}
		ts, _ := extractTimestamp(trimmed)
		stripped := stripTimestamp(trimmed)
		for _, p := range parsers {
			hostname, ok := p.DetectPrompt(stripped)
			if !ok {
				continue
			}
			cmd := extractCommand(stripped, p)
			if cmd == "" {
				continue
			}
			matches = append(matches, promptMatch{
				lineIndex:  i,
				hostname:   hostname,
				vendor:     p.Vendor(),
				command:    cmd,
				capturedAt: ts,
			})
			break
		}
	}

	if len(matches) <= 1 {
		// 0 prompts: nothing to parse
		// 1 prompt: single block with no terminating prompt — incomplete
		return nil, 0
	}

	// Phase 2: Build only complete blocks (all except the last match)
	var blocks []CommandBlock
	for i := 0; i < len(matches)-1; i++ {
		m := matches[i]
		outputStart := m.lineIndex + 1
		outputEnd := matches[i+1].lineIndex

		var outputLines []string
		if outputStart < outputEnd {
			for _, ol := range lines[outputStart:outputEnd] {
				stripped := stripTimestamp(strings.TrimRight(ol, "\r"))
				outputLines = append(outputLines, stripped)
			}
		}
		output := strings.TrimRight(strings.Join(outputLines, "\n"), "\n\r \t")
		blocks = append(blocks, CommandBlock{
			Hostname:   m.hostname,
			Vendor:     m.vendor,
			Command:    m.command,
			Output:     output,
			CapturedAt: m.capturedAt,
		})
	}

	// consumedBytes = byte offset of the last prompt (which is incomplete)
	lastPromptLineIdx := matches[len(matches)-1].lineIndex
	consumed := lineByteOffsets[lastPromptLineIdx]

	return blocks, consumed
}
```

The original `Split()` function stays unchanged for backwards compatibility (used by `watch ingest` manual import).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/parser/ -v -run TestSplit`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/parser/splitter.go internal/parser/splitter_test.go
git commit -m "feat(parser): add SplitWithOffset for incremental-safe command splitting"
```

---

## Task 2: Add BytesConsumed to IngestResult and wire through Pipeline

**Files:**
- Modify: `internal/parser/pipeline.go`

- [ ] **Step 1: Add BytesConsumed field to IngestResult**

In `internal/parser/pipeline.go`, add to `IngestResult`:

```go
type IngestResult struct {
	DevicesFound  int
	BlocksParsed  int
	BlocksFailed  int
	BlocksSkipped int
	BytesConsumed int // byte offset of content actually processed (for incremental safety)
}
```

- [ ] **Step 2: Add IngestIncremental method**

Add a new method that uses `SplitWithOffset` instead of `Split`:

```go
// IngestIncremental is like Ingest but uses SplitWithOffset to exclude
// the last potentially-incomplete command block. BytesConsumed in the
// result indicates how many bytes of content were fully processed.
// The caller should advance its file offset by BytesConsumed, not by
// len(content), to avoid skipping incomplete output.
func (p *Pipeline) IngestIncremental(sourceFile, content string) (IngestResult, error) {
	var result IngestResult

	blocks, consumed := SplitWithOffset(content, p.registry)
	result.BytesConsumed = consumed
	if len(blocks) == 0 {
		return result, nil
	}

	// The rest is identical to Ingest — classify, reclassify, group, store.
	// Extract to a shared helper to avoid duplication.
	return p.processBlocks(sourceFile, blocks, result)
}
```

- [ ] **Step 3: Extract shared processing into processBlocks**

Refactor: move the classify → reclassify → group → store logic from `Ingest()` into a private `processBlocks(sourceFile string, blocks []CommandBlock, result IngestResult) (IngestResult, error)` method. Both `Ingest()` and `IngestIncremental()` call it.

```go
func (p *Pipeline) Ingest(sourceFile, content string) (IngestResult, error) {
	var result IngestResult
	blocks := Split(content, p.registry)
	if len(blocks) == 0 {
		return result, nil
	}
	result.BytesConsumed = len(content) // full content consumed for non-incremental
	return p.processBlocks(sourceFile, blocks, result)
}

func (p *Pipeline) IngestIncremental(sourceFile, content string) (IngestResult, error) {
	var result IngestResult
	blocks, consumed := SplitWithOffset(content, p.registry)
	result.BytesConsumed = consumed
	if len(blocks) == 0 {
		return result, nil
	}
	return p.processBlocks(sourceFile, blocks, result)
}

func (p *Pipeline) processBlocks(sourceFile string, blocks []CommandBlock, result IngestResult) (IngestResult, error) {
	// ... existing classify, reclassify, group, store logic moved here ...
}
```

- [ ] **Step 4: Build and run existing tests**

Run: `go build ./... && go test ./internal/parser/ -v -count=1`
Expected: All existing tests still pass. No new test needed — this is pure refactoring.

- [ ] **Step 5: Commit**

```bash
git add internal/parser/pipeline.go
git commit -m "feat(parser): add IngestIncremental with BytesConsumed tracking"
```

---

## Task 3: Wire watch.go to use IngestIncremental

**Files:**
- Modify: `internal/cli/watch.go`

- [ ] **Step 1: Change OnFileChange to use IngestIncremental**

In `watch.go`, line 109, change:
```go
// OLD:
result, err := pipeline.Ingest(path, string(newData))
```
to:
```go
// NEW:
result, err := pipeline.IngestIncremental(path, string(newData))
```

And line 115, change:
```go
// OLD:
newIng := model.LogIngestion{FilePath: path, LastOffset: info.Size(), ProcessedAt: time.Now()}
```
to:
```go
// NEW: advance offset by BytesConsumed, not total new data size
newOffset := offset + int64(result.BytesConsumed)
newIng := model.LogIngestion{FilePath: path, LastOffset: newOffset, ProcessedAt: time.Now()}
```

Note: `watch ingest` (manual one-shot) continues to use `pipeline.Ingest()` — it reads the whole file at once, so truncation is not a concern there.

- [ ] **Step 2: Build**

Run: `go build ./cmd/nethelper`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/cli/watch.go
git commit -m "fix(watch): use IngestIncremental to prevent config truncation"
```

---

## Task 4: Integration test — re-ingest LC log and verify complete config

- [ ] **Step 1: Delete the truncated data and re-ingest**

```bash
# Clear existing LC data
sqlite3 ~/.nethelper/nethelper.db "DELETE FROM config_snapshots WHERE device_id='cd-gx-0201-g17-h12516af-lc-01'"
sqlite3 ~/.nethelper/nethelper.db "DELETE FROM log_ingestions WHERE file_path LIKE '%teg_20260322112428%'"

# Re-ingest the full log file (manual ingest uses full-file Ingest, not incremental)
./nethelper watch ingest /Users/xavierli/Work/session_log/netdevice/teg_20260322112428.log
```

- [ ] **Step 2: Verify LC config is now complete**

```bash
sqlite3 ~/.nethelper/nethelper.db "SELECT length(config_text) FROM config_snapshots WHERE device_id='cd-gx-0201-g17-h12516af-lc-01' ORDER BY id DESC LIMIT 1"
```

Expected: Much larger than 14125 (should be ~100KB+ with BGP section).

- [ ] **Step 3: Verify BGP peers were extracted**

```bash
sqlite3 ~/.nethelper/nethelper.db "SELECT count(*), group_concat(DISTINCT peer_group) FROM bgp_peers WHERE device_id='cd-gx-0201-g17-h12516af-lc-01'"
```

Expected: Multiple peers, groups including LA1, LA2~448, MAN, QCDR, XGWL, SDN-Controller-Read, SDN-Controller-Write.

- [ ] **Step 4: Run full test suite**

Run: `go test ./... && go vet ./...`
Expected: All pass.

- [ ] **Step 5: Commit any fixes**

```bash
git commit -m "test: verify LC config completeness after ingestion fix"
```
