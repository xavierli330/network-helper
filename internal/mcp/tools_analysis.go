// internal/mcp/tools_analysis.go
package mcp

import (
	"context"
	"encoding/json"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/graph"
	"github.com/xavierli/nethelper/internal/store"
)

func registerAnalysisTools(s *mcpserver.MCPServer, db *store.DB) {
	// 1. trace_path — shortest path between two devices
	s.AddTool(mcpmcp.NewTool("trace_path",
		mcpmcp.WithDescription("Find the shortest path between two network devices in the topology graph"),
		mcpmcp.WithString("from", mcpmcp.Required(), mcpmcp.Description("Source device ID")),
		mcpmcp.WithString("to", mcpmcp.Required(), mcpmcp.Description("Destination device ID")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		if from == "" {
			return mcpmcp.NewToolResultError("from is required"), nil
		}
		if to == "" {
			return mcpmcp.NewToolResultError("to is required"), nil
		}

		g, err := graph.BuildFromDB(db)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to build graph: " + err.Error()), nil
		}

		path, err := graph.ShortestPath(g, from, to)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}

		data, _ := json.MarshalIndent(path, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 2. trace_impact — impact analysis when a device is removed
	s.AddTool(mcpmcp.NewTool("trace_impact",
		mcpmcp.WithDescription("Determine which devices become unreachable if a given device is removed from the topology"),
		mcpmcp.WithString("node_id", mcpmcp.Required(), mcpmcp.Description("The device ID to remove")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		nodeID := req.GetString("node_id", "")
		if nodeID == "" {
			return mcpmcp.NewToolResultError("node_id is required"), nil
		}

		g, err := graph.BuildFromDB(db)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to build graph: " + err.Error()), nil
		}

		affected := graph.ImpactAnalysis(g, nodeID, graph.NodeTypeDevice)
		if affected == nil {
			affected = []string{}
		}

		type impactResult struct {
			RemovedNode    string   `json:"removed_node"`
			AffectedDevices []string `json:"affected_devices"`
		}
		result := impactResult{
			RemovedNode:    nodeID,
			AffectedDevices: affected,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 3. check_spof — find single points of failure
	s.AddTool(mcpmcp.NewTool("check_spof",
		mcpmcp.WithDescription("Find all single points of failure (SPOF) in the network device topology"),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		g, err := graph.BuildFromDB(db)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to build graph: " + err.Error()), nil
		}

		spofs := graph.FindSPOF(g, graph.NodeTypeDevice)
		if spofs == nil {
			spofs = []string{}
		}

		data, _ := json.MarshalIndent(spofs, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 4. check_loop — detect routing loops
	s.AddTool(mcpmcp.NewTool("check_loop",
		mcpmcp.WithDescription("Detect directed cycles (routing loops) in the network topology graph"),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		g, err := graph.BuildFromDB(db)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to build graph: " + err.Error()), nil
		}

		loops := graph.DetectLoops(g)
		if loops == nil {
			loops = [][]string{}
		}

		type loopResult struct {
			LoopCount int        `json:"loop_count"`
			Loops     [][]string `json:"loops"`
		}
		result := loopResult{
			LoopCount: len(loops),
			Loops:     loops,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})
}
