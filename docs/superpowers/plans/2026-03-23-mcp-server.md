# nethelper MCP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose all nethelper capabilities as MCP tools so Claude Code and other agents can call them directly.

**Architecture:** New `internal/mcp/` package using `github.com/mark3labs/mcp-go` SDK. Each tool handler calls existing Go functions (store/graph/plan) directly. CLI entry point `nethelper mcp serve` starts a stdio MCP server. Tools grouped by domain into separate files.

**Tech Stack:** Go, `github.com/mark3labs/mcp-go` (stdio transport), existing store/graph/plan packages.

**Spec:** `docs/superpowers/specs/2026-03-23-mcp-server-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/mcp/server.go` | Create | MCP server init + tool registration |
| `internal/mcp/tools_show.go` | Create | show_devices, show_device, show_interfaces, show_routes, show_neighbors, show_bgp_peers, show_topology |
| `internal/mcp/tools_analysis.go` | Create | trace_path, trace_impact, check_spof, check_loop |
| `internal/mcp/tools_plan.go` | Create | plan_isolate, plan_upgrade, plan_cutover |
| `internal/mcp/tools_search.go` | Create | search_config, search_log, diff_config |
| `internal/mcp/tools_write.go` | Create | note_add, watch_ingest, diagnose |
| `internal/cli/mcp.go` | Create | CLI entry: `nethelper mcp serve` |
| `internal/cli/root.go` | Modify | Register mcp command |
| `go.mod` | Modify | Add mcp-go dependency |

---

## Task 1: Add mcp-go dependency + server skeleton

**Files:**
- Modify: `go.mod`
- Create: `internal/mcp/server.go`
- Create: `internal/cli/mcp.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Add dependency**

Run: `go get github.com/mark3labs/mcp-go@latest`

- [ ] **Step 2: Create server.go — MCP server factory**

```go
// internal/mcp/server.go
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

// NewServer creates an MCP server with all nethelper tools registered.
func NewServer(db *store.DB, pipeline *parser.Pipeline, llmRouter *llm.Router) *server.MCPServer {
	s := server.NewMCPServer(
		"nethelper",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	registerShowTools(s, db)
	registerAnalysisTools(s, db)
	registerPlanTools(s, db)
	registerSearchTools(s, db)
	registerWriteTools(s, db, pipeline, llmRouter)

	return s
}
```

- [ ] **Step 3: Create CLI entry**

```go
// internal/cli/mcp.go
package cli

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	netmcp "github.com/xavierli/nethelper/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	mcp := &cobra.Command{
		Use:   "mcp",
		Short: "MCP (Model Context Protocol) server",
	}
	mcp.AddCommand(newMCPServeCmd())
	return mcp
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server (stdio transport)",
		Long:  "Start a Model Context Protocol server that exposes nethelper tools over stdio. Configure in Claude Code: {\"mcpServers\": {\"nethelper\": {\"command\": \"nethelper\", \"args\": [\"mcp\", \"serve\"]}}}",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := netmcp.NewServer(db, pipeline, llmRouter)
			return server.ServeStdio(s)
		},
	}
}
```

- [ ] **Step 4: Register in root.go**

Add `root.AddCommand(newMCPCmd())` in `NewRootCmd()`.

- [ ] **Step 5: Build**

Run: `go build ./cmd/nethelper && ./nethelper mcp serve --help`
Expected: Help text showing MCP serve command.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(mcp): add MCP server skeleton with mcp-go SDK"
```

---

## Task 2: Show tools (7 tools)

**Files:**
- Create: `internal/mcp/tools_show.go`

These are the most-used tools — all read-only queries returning JSON.

- [ ] **Step 1: Implement all show tools**

```go
// internal/mcp/tools_show.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/store"
)

func registerShowTools(s *server.MCPServer, db *store.DB) {
	// show_devices
	s.AddTool(mcp.NewTool("show_devices",
		mcp.WithDescription("List all network devices in the database with hostname, vendor, OS version, and management IP"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		devices, err := db.ListDevices()
		if err != nil { return nil, err }
		data, _ := json.MarshalIndent(devices, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// show_device
	s.AddTool(mcp.NewTool("show_device",
		mcp.WithDescription("Get detailed info for a specific device by ID"),
		mcp.WithString("device_id", mcp.Required(), mcp.Description("Device ID (lowercase hostname slug)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, _ := req.Params.Arguments["device_id"].(string)
		if id == "" { return mcp.NewToolResultError("device_id is required"), nil }
		dev, err := db.GetDevice(id)
		if err != nil { return mcp.NewToolResultError(fmt.Sprintf("device not found: %v", err)), nil }
		data, _ := json.MarshalIndent(dev, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// show_interfaces
	s.AddTool(mcp.NewTool("show_interfaces",
		mcp.WithDescription("List all interfaces for a device with status, IP, description"),
		mcp.WithString("device_id", mcp.Required(), mcp.Description("Device ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, _ := req.Params.Arguments["device_id"].(string)
		ifaces, err := db.GetInterfaces(id)
		if err != nil { return nil, err }
		data, _ := json.MarshalIndent(ifaces, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// show_neighbors
	s.AddTool(mcp.NewTool("show_neighbors",
		mcp.WithDescription("List protocol neighbors (OSPF/BGP/ISIS/LDP) for a device"),
		mcp.WithString("device_id", mcp.Required(), mcp.Description("Device ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, _ := req.Params.Arguments["device_id"].(string)
		neighbors, err := db.GetLatestNeighbors(id)
		if err != nil { return nil, err }
		data, _ := json.MarshalIndent(neighbors, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// show_bgp_peers
	s.AddTool(mcp.NewTool("show_bgp_peers",
		mcp.WithDescription("List BGP peers for a device with peer-group, AS, description"),
		mcp.WithString("device_id", mcp.Required(), mcp.Description("Device ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, _ := req.Params.Arguments["device_id"].(string)
		peers, err := db.GetLatestBGPPeers(id)
		if err != nil { return nil, err }
		data, _ := json.MarshalIndent(peers, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// show_topology
	s.AddTool(mcp.NewTool("show_topology",
		mcp.WithDescription("Get network topology overview: device count, interface count, subnet count, per-device peer counts"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Build simple topology summary
		devices, _ := db.ListDevices()
		type topoSummary struct {
			Devices    int `json:"devices"`
			DeviceList []struct {
				ID       string `json:"id"`
				Hostname string `json:"hostname"`
				Vendor   string `json:"vendor"`
			} `json:"device_list"`
		}
		summary := topoSummary{Devices: len(devices)}
		for _, d := range devices {
			summary.DeviceList = append(summary.DeviceList, struct {
				ID       string `json:"id"`
				Hostname string `json:"hostname"`
				Vendor   string `json:"vendor"`
			}{d.ID, d.Hostname, d.Vendor})
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})

	// show_routes — uses scratch pad (RIB is stored there for large tables)
	s.AddTool(mcp.NewTool("show_routes",
		mcp.WithDescription("Get routing table entries for a device (from scratch pad)"),
		mcp.WithString("device_id", mcp.Required(), mcp.Description("Device ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, _ := req.Params.Arguments["device_id"].(string)
		entries, err := db.ListScratch(id, "route", 10)
		if err != nil { return nil, err }
		if len(entries) == 0 {
			return mcp.NewToolResultText("No routing table entries found. Run 'display ip routing-table' on the device and ingest the log."), nil
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	})
}
```

**IMPORTANT NOTE for implementor:** The exact `mcp-go` API might differ slightly. Read the SDK source or docs to confirm:
- `mcp.NewTool(name, opts...)` vs `server.NewTool`
- `mcp.CallToolRequest` field structure — `req.Params.Arguments` is `map[string]interface{}`
- `mcp.NewToolResultText(text)` vs `mcp.NewToolResult(...)` — check the actual API
- Error handling: `mcp.NewToolResultError(msg)` may not exist — might need `&mcp.CallToolResult{IsError: true, Content: [...]}`

Read `github.com/mark3labs/mcp-go` source to get exact types before implementing.

- [ ] **Step 2: Build**

Run: `go build ./internal/mcp/`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(mcp): add 7 show tools (devices, interfaces, neighbors, BGP, topology, routes)"
```

---

## Task 3: Analysis tools (4 tools)

**Files:**
- Create: `internal/mcp/tools_analysis.go`

Tools: `trace_path`, `trace_impact`, `check_spof`, `check_loop`. All need `graph.BuildFromDB(db)` first.

- [ ] **Step 1: Implement**

```go
// internal/mcp/tools_analysis.go
package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/graph"
	"github.com/xavierli/nethelper/internal/store"
)

func registerAnalysisTools(s *server.MCPServer, db *store.DB) {
	// trace_path — ShortestPath(g, from, to) returns ([]string, error)
	s.AddTool(mcp.NewTool("trace_path",
		mcp.WithDescription("Trace shortest path between two devices"),
		mcp.WithString("from", mcp.Required(), mcp.Description("Source device ID")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Destination device ID")),
	), tracePathHandler(db))

	// trace_impact — ImpactAnalysis(g, nodeID, NodeTypeDevice) returns []string
	s.AddTool(mcp.NewTool("trace_impact",
		mcp.WithDescription("Analyze which devices become unreachable if a node is removed"),
		mcp.WithString("node_id", mcp.Required(), mcp.Description("Device ID to simulate removing")),
	), traceImpactHandler(db))

	// check_spof — FindSPOF(g, NodeTypeDevice) returns []string
	s.AddTool(mcp.NewTool("check_spof",
		mcp.WithDescription("Find single points of failure in the network topology"),
	), checkSPOFHandler(db))

	// check_loop — DetectLoops(g) returns [][]string
	s.AddTool(mcp.NewTool("check_loop",
		mcp.WithDescription("Detect routing loops in the network topology"),
	), checkLoopHandler(db))
}
```

Each handler: build graph → call analysis function → JSON marshal result.

- [ ] **Step 2: Build + commit**

---

## Task 4: Plan tools (3 tools)

**Files:**
- Create: `internal/mcp/tools_plan.go`

Tools: `plan_isolate`, `plan_upgrade`, `plan_cutover`. Each calls `BuildTopology` then the appropriate generator, renders as Markdown.

```go
// plan_isolate handler pattern:
topo, err := plan.BuildTopology(db, deviceID)
p := plan.GenerateIsolationPlanV2(topo)
result := plan.RenderMarkdown(p)
return mcp.NewToolResultText(result), nil
```

`plan_upgrade` additionally needs `version` and `file` params.
`plan_cutover` needs `old_interface` and `new_interface` params.

- [ ] **Step 1: Implement**
- [ ] **Step 2: Build + commit**

---

## Task 5: Search + diff tools (3 tools)

**Files:**
- Create: `internal/mcp/tools_search.go`

Tools: `search_config`, `search_log`, `diff_config`.

Search tools use FTS5 queries. Read `internal/cli/search.go` to see how existing search commands work (they use `db.Query` with FTS5 MATCH syntax).

`diff_config` gets the two most recent config snapshots for a device and shows the diff.

- [ ] **Step 1: Implement**
- [ ] **Step 2: Build + commit**

---

## Task 6: Write tools + diagnose (3 tools)

**Files:**
- Create: `internal/mcp/tools_write.go`

Tools: `note_add`, `watch_ingest`, `diagnose`.

`note_add` — inserts a troubleshoot log record.
`watch_ingest` — calls `pipeline.Ingest(filePath, content)` (reads file, ingests).
`diagnose` — calls the LLM router with device context (similar to existing `diagnose` CLI command). Read `internal/cli/diagnose.go` for the pattern.

- [ ] **Step 1: Implement**
- [ ] **Step 2: Build + commit**

---

## Task 7: Integration test + Claude Code config

- [ ] **Step 1: Start MCP server and verify it responds to initialize**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./nethelper mcp serve 2>/dev/null | head -1
```

Expected: JSON response with server capabilities listing all 19+ tools.

- [ ] **Step 2: Test a tool call**

```bash
# First initialize, then call show_devices
(echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'; echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"show_devices","arguments":{}}}') | ./nethelper mcp serve 2>/dev/null
```

Expected: JSON with device list.

- [ ] **Step 3: Create Claude Code config**

Create `.mcp.json` in project root (or instruct user to add to settings):

```json
{
  "mcpServers": {
    "nethelper": {
      "command": "./nethelper",
      "args": ["mcp", "serve"]
    }
  }
}
```

- [ ] **Step 4: Full test suite**

Run: `go test ./... && go vet ./...`

- [ ] **Step 5: Update README**

Add MCP section to README. Update Phase 2 progress.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(mcp): complete MCP server with 19 tools + Claude Code integration"
```
