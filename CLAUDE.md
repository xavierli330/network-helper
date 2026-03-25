# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`nethelper` is a Go CLI tool for network engineers. It parses terminal session logs from Huawei VRP, Cisco IOS, H3C Comware, and Juniper JUNOS devices, stores structured data in a local SQLite database, and provides topology analysis, config diffing, and LLM-powered diagnostics.

## Common Commands

```bash
# Build
go build -o nethelper ./cmd/nethelper

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/parser/...
go test ./internal/graph/...

# Vet
go vet ./...

# Run the binary
./nethelper version
./nethelper watch ingest <log-file>
./nethelper show device
```

## Architecture

### Entry Point & Bootstrap

`cmd/nethelper/main.go` → `internal/cli/root.go`

The root Cobra command's `PersistentPreRunE` wires the app on every invocation:
1. `config.LoadFrom(cfgFile)` — load `~/.nethelper/config.yaml`
2. `store.Open(cfg.DBPath)` — open SQLite with WAL mode + FTS5
3. Register vendor parsers into `parser.Registry` (order matters: first prompt match wins)
4. `parser.NewPipeline(db, registry)` — create ingest pipeline
5. `llm.BuildFromConfig(cfg.LLM)` — build LLM router

Four globals (`cfg`, `db`, `pipeline`, `llmRouter`) are shared across all CLI commands.

### Core Data Pipeline

```
Terminal log file
  → parser.Split()           // detect prompts, split into CommandBlocks
  → VendorParser.ClassifyCommand()   // determine CommandType
  → VendorParser.ParseOutput()       // extract structured model.ParseResult
  → Pipeline.storeResult()           // write to SQLite
```

Large table outputs (RIB, FIB, LFIB) go to the **scratch pad** (`scratch_entries` table, 200-entry FIFO) instead of structured tables.

### Key Interfaces

**`VendorParser`** (`internal/parser/types.go`) — implemented by `huawei`, `cisco`, `h3c`, `juniper` packages:
```go
type VendorParser interface {
    Vendor() string
    DetectPrompt(line string) (hostname string, ok bool)
    ClassifyCommand(cmd string) model.CommandType
    ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
}
```

**`llm.Provider`** (`internal/llm/`) — two implementations:
- `OpenAIProvider` — `POST /chat/completions` (OpenAI, Ollama, DeepSeek, Qwen, etc.)
- `AnthropicProvider` — `POST /v1/messages` with `x-api-key` (Claude, Kimi Coding)

Selection in `router.go` is by name/URL heuristic: names or URLs containing "anthropic" or "kimi" → Anthropic protocol; everything else → OpenAI.

### Data Model Patterns

- **`model.ParseResult`** is a union struct — all parsed data fields in one struct, pipeline fans out based on which slices are non-empty
- **`model.CommandType`** is a string enum (`CmdRIB`, `CmdFIB`, `CmdLFIB`, `CmdInterface`, `CmdNeighbor`, `CmdTunnel`, `CmdSRMapping`, `CmdConfig`, `CmdUnknown`)
- **Snapshot versioning** — every ingestion creates a `snapshots` row; all data tables reference `snapshot_id` for historical queries and diffs
- **Incremental ingestion** — file offset tracked in `log_ingestions` table; only new bytes are read; rotation detected by size comparison

### Graph Engine (`internal/graph/`)

Built on demand from SQLite via `graph.BuildFromDB(db)`. Typed nodes (Device, Interface, Subnet, VRF, LSP, SRPolicy) with typed edges. Algorithms: BFS shortest path, DFS all paths, loop detection (DFS coloring), SPOF detection (articulation point), impact analysis.

### LLM Context (`internal/cli/`)

`diagnose` and `explain` commands share a `gatherContext(query, deviceID)` helper that pulls device details, interfaces, neighbors, config snapshots, and scratch entries from SQLite and formats them as Markdown blocks for the LLM system prompt.

## SQLite Notes

Uses `github.com/ncruces/go-sqlite3` (CGo-free, WASM via wazero — no system SQLite required). Three FTS5 virtual tables: `fts_config`, `fts_troubleshoot`, `fts_commands` backed by their base tables.

## Adding a New Vendor Parser

1. Create `internal/parser/<vendor>/` package implementing `VendorParser`
2. Register in `internal/cli/root.go` `PersistentPreRunE`
3. Add test cases in `internal/parser/pipeline_test.go`

## Adding a New LLM Provider Protocol

Implement `llm.Provider` interface in `internal/llm/`, add selection logic in `router.go`'s `createProvider`.

## Knowledge Sources (`internal/memory/`)

The knowledge system supports pluggable sources via the `KnowledgeSource` interface:

```go
type KnowledgeSource interface {
    Name() string
    Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
}
```

**Built-in sources:**
- `localKnowledgeAdapter` - Local embedding-based search (requires embedder)
- `HTTPKnowledgeSource` - External HTTP API search
- `IMAKnowledgeSource` - Tencent IMA knowledge base search

**Adding a new source:**
1. Implement `KnowledgeSource` interface in `internal/memory/source_<name>.go`
2. Add config fields in `internal/config/config.go` `KnowledgeSourceConfig`
3. Register in `internal/cli/agent.go` `buildKnowledgeSources()`
4. Add CLI command in `internal/cli/knowledge.go` if needed

**Direct knowledge search (no LLM):**
Use `nethelper knowledge search` for verifiable, traceable results without LLM processing.
