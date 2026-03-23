// internal/mcp/tools_search.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/store"
)

func registerSearchTools(s *mcpserver.MCPServer, db *store.DB) {
	// 1. search_config — full-text search over config snapshots
	s.AddTool(mcpmcp.NewTool("search_config",
		mcpmcp.WithDescription("Full-text search across all device configuration snapshots using FTS5"),
		mcpmcp.WithString("query", mcpmcp.Required(), mcpmcp.Description("FTS5 search query (e.g. 'ospf', 'bgp neighbor', 'interface GE')")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return mcpmcp.NewToolResultError("query is required"), nil
		}

		db.SyncConfigFTS()
		results, err := db.SearchConfig(query)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}

		type configResult struct {
			DeviceID   string `json:"device_id"`
			SourceFile string `json:"source_file"`
			CapturedAt string `json:"captured_at"`
			Snippet    string `json:"snippet"`
		}

		out := make([]configResult, 0, len(results))
		for _, cs := range results {
			snippet := cs.ConfigText
			if len(snippet) > 500 {
				snippet = snippet[:500] + "..."
			}
			out = append(out, configResult{
				DeviceID:   cs.DeviceID,
				SourceFile: cs.SourceFile,
				CapturedAt: cs.CapturedAt.Format("2006-01-02 15:04"),
				Snippet:    snippet,
			})
		}

		data, _ := json.MarshalIndent(out, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 2. search_log — full-text search over troubleshooting logs
	s.AddTool(mcpmcp.NewTool("search_log",
		mcpmcp.WithDescription("Full-text search across all troubleshooting log notes using FTS5"),
		mcpmcp.WithString("query", mcpmcp.Required(), mcpmcp.Description("FTS5 search query (e.g. 'OSPF neighbor down', 'BGP flap')")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		query := req.GetString("query", "")
		if query == "" {
			return mcpmcp.NewToolResultError("query is required"), nil
		}

		results, err := db.SearchTroubleshootLogs(query)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}

		data, _ := json.MarshalIndent(results, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 3. diff_config — show unified diff between the two most recent config snapshots
	s.AddTool(mcpmcp.NewTool("diff_config",
		mcpmcp.WithDescription("Show a unified diff between the two most recent configuration snapshots for a device"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID whose config history to diff")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		deviceID := req.GetString("device_id", "")
		if deviceID == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}

		snapshots, err := db.GetConfigSnapshots(deviceID)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}
		if len(snapshots) == 0 {
			return mcpmcp.NewToolResultText("no config snapshots found for device: " + deviceID), nil
		}
		if len(snapshots) < 2 {
			return mcpmcp.NewToolResultText("only one config snapshot exists; no diff available"), nil
		}

		// snapshots are ordered DESC by captured_at, so [0] is newest, [1] is previous
		newest := snapshots[0]
		previous := snapshots[1]

		diff := unifiedDiff(
			previous.ConfigText,
			newest.ConfigText,
			fmt.Sprintf("previous (%s)", previous.CapturedAt.Format("2006-01-02 15:04")),
			fmt.Sprintf("newest (%s)", newest.CapturedAt.Format("2006-01-02 15:04")),
		)

		return mcpmcp.NewToolResultText(diff), nil
	})
}

// unifiedDiff produces a simple line-level unified diff between two texts.
func unifiedDiff(oldText, newText, oldLabel, newLabel string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n", oldLabel))
	sb.WriteString(fmt.Sprintf("+++ %s\n", newLabel))

	// Build a set of old lines for quick lookup
	oldSet := make(map[string]bool, len(oldLines))
	for _, l := range oldLines {
		oldSet[l] = true
	}
	newSet := make(map[string]bool, len(newLines))
	for _, l := range newLines {
		newSet[l] = true
	}

	// Lines removed (in old but not in new)
	for _, l := range oldLines {
		if !newSet[l] {
			sb.WriteString("- " + l + "\n")
		}
	}

	// Lines added (in new but not in old)
	for _, l := range newLines {
		if !oldSet[l] {
			sb.WriteString("+ " + l + "\n")
		}
	}

	result := sb.String()
	if result == fmt.Sprintf("--- %s\n+++ %s\n", oldLabel, newLabel) {
		return "No differences found between the two snapshots."
	}
	return result
}
