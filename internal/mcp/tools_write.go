// internal/mcp/tools_write.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

func registerWriteTools(s *mcpserver.MCPServer, db *store.DB, pipeline *parser.Pipeline, llmRouter *llm.Router) {
	// 1. note_add — add a troubleshooting note
	s.AddTool(mcpmcp.NewTool("note_add",
		mcpmcp.WithDescription("Add a troubleshooting log note to the database"),
		mcpmcp.WithString("device_id", mcpmcp.Description("The device ID this note relates to (optional)")),
		mcpmcp.WithString("symptom", mcpmcp.Required(), mcpmcp.Description("Description of the observed symptom or problem")),
		mcpmcp.WithString("commands_used", mcpmcp.Description("Commands used during troubleshooting (optional)")),
		mcpmcp.WithString("findings", mcpmcp.Description("What was discovered (optional)")),
		mcpmcp.WithString("resolution", mcpmcp.Description("How the issue was resolved (optional)")),
		mcpmcp.WithString("tags", mcpmcp.Description("Comma-separated tags for categorisation (optional)")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		symptom := req.GetString("symptom", "")
		if symptom == "" {
			return mcpmcp.NewToolResultError("symptom is required"), nil
		}

		log := model.TroubleshootLog{
			DeviceID:     req.GetString("device_id", ""),
			Symptom:      symptom,
			CommandsUsed: req.GetString("commands_used", ""),
			Findings:     req.GetString("findings", ""),
			Resolution:   req.GetString("resolution", ""),
			Tags:         req.GetString("tags", ""),
		}

		id, err := db.InsertTroubleshootLog(log)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to insert note: " + err.Error()), nil
		}

		type noteResult struct {
			ID      int    `json:"id"`
			Message string `json:"message"`
		}
		result := noteResult{
			ID:      id,
			Message: fmt.Sprintf("Troubleshooting note #%d added successfully", id),
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 2. watch_ingest — ingest a terminal log file into the database
	s.AddTool(mcpmcp.NewTool("watch_ingest",
		mcpmcp.WithDescription("Parse and ingest a terminal session log file into the nethelper database"),
		mcpmcp.WithString("file_path", mcpmcp.Required(), mcpmcp.Description("Absolute path to the terminal log file to ingest")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		filePath := req.GetString("file_path", "")
		if filePath == "" {
			return mcpmcp.NewToolResultError("file_path is required"), nil
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to read file: " + err.Error()), nil
		}

		result, err := pipeline.Ingest(filePath, string(data))
		if err != nil {
			return mcpmcp.NewToolResultError("ingest failed: " + err.Error()), nil
		}

		out, _ := json.MarshalIndent(result, "", "  ")
		return mcpmcp.NewToolResultText(string(out)), nil
	})

	// 3. diagnose — AI-powered diagnosis using device context
	s.AddTool(mcpmcp.NewTool("diagnose",
		mcpmcp.WithDescription("AI-powered network troubleshooting: describe a problem and get diagnosis based on device data"),
		mcpmcp.WithString("description", mcpmcp.Required(), mcpmcp.Description("Description of the network problem to diagnose")),
		mcpmcp.WithString("device_id", mcpmcp.Description("Optional device ID to focus the diagnosis on")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		description := req.GetString("description", "")
		if description == "" {
			return mcpmcp.NewToolResultError("description is required"), nil
		}
		deviceID := req.GetString("device_id", "")

		if llmRouter == nil || !llmRouter.Available() {
			return mcpmcp.NewToolResultError("LLM not configured — run 'nethelper config llm' to set up an LLM provider"), nil
		}

		// Search troubleshooting history for relevant past cases
		logs, _ := db.SearchTroubleshootLogs(description)

		// Build device context from the database
		deviceCtx := gatherMCPContext(db, description, deviceID)

		// Build history context
		var historyCtx string
		if len(logs) > 0 {
			var parts []string
			for _, l := range logs {
				parts = append(parts, fmt.Sprintf("- Symptom: %s / Findings: %s / Resolution: %s",
					l.Symptom, l.Findings, l.Resolution))
			}
			historyCtx = "## Related Troubleshooting History\n" + strings.Join(parts, "\n")
		}

		systemPrompt := `You are a senior network engineer assistant. You have access to a network management database.
The user describes a network problem. Below is the relevant device data and troubleshooting history from the database.

Your job:
1. Analyze the problem using the provided device data (config, neighbors, interfaces, routes)
2. Identify possible root causes (ranked by likelihood), referencing specific data from the context
3. Suggest troubleshooting commands to run next
4. If similar past issues exist, reference them
5. If the data is insufficient, say what additional information is needed

Be precise. Reference specific interface names, IP addresses, AS numbers from the context.
Reply in the same language as the user's input.`

		userMsg := fmt.Sprintf("Problem description: %s", description)
		if deviceCtx != "" {
			userMsg += "\n\n" + deviceCtx
		}
		if historyCtx != "" {
			userMsg += "\n\n" + historyCtx
		}

		resp, err := llmRouter.Chat(ctx, llm.CapAnalyze, llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userMsg},
			},
			MaxTokens: 4096,
		})
		if err != nil {
			return mcpmcp.NewToolResultError("LLM error: " + err.Error()), nil
		}

		return mcpmcp.NewToolResultText(resp.Content), nil
	})
}

// gatherMCPContext collects relevant device data from the database and formats it
// as a Markdown context block for the LLM.
func gatherMCPContext(db *store.DB, query, deviceID string) string {
	var sb strings.Builder

	// If a specific device is provided, pull its details
	if deviceID != "" {
		device, err := db.GetDevice(deviceID)
		if err == nil {
			sb.WriteString(fmt.Sprintf("## Device: %s (%s) — %s\n\n", device.Hostname, device.Vendor, device.ID))

			ifaces, err := db.GetInterfaces(deviceID)
			if err == nil && len(ifaces) > 0 {
				sb.WriteString("### Interfaces\n")
				for _, iface := range ifaces {
					sb.WriteString(fmt.Sprintf("- %s: %s/%s status=%s\n",
						iface.Name, iface.IPAddress, iface.Mask, iface.Status))
				}
				sb.WriteString("\n")
			}

			neighbors, err := db.GetLatestNeighbors(deviceID)
			if err == nil && len(neighbors) > 0 {
				sb.WriteString("### Neighbors\n")
				for _, n := range neighbors {
					sb.WriteString(fmt.Sprintf("- %s via %s (state=%s protocol=%s)\n",
						n.RemoteID, n.LocalInterface, n.State, n.Protocol))
				}
				sb.WriteString("\n")
			}

			peers, err := db.GetLatestBGPPeers(deviceID)
			if err == nil && len(peers) > 0 {
				sb.WriteString("### BGP Peers\n")
				for _, p := range peers {
					sb.WriteString(fmt.Sprintf("- %s AS%d (group=%s)\n", p.PeerIP, p.RemoteAS, p.PeerGroup))
				}
				sb.WriteString("\n")
			}

			snapshots, err := db.GetConfigSnapshots(deviceID)
			if err == nil && len(snapshots) > 0 {
				cfg := snapshots[0].ConfigText
				if len(cfg) > 2000 {
					cfg = cfg[:2000] + "\n... (truncated)"
				}
				sb.WriteString("### Latest Config\n```\n")
				sb.WriteString(cfg)
				sb.WriteString("\n```\n\n")
			}

			// Scratch entries (routes, etc.)
			routes, err := db.ListScratch(deviceID, "route", 5)
			if err == nil && len(routes) > 0 {
				sb.WriteString("### Recent Route Entries\n")
				for _, r := range routes {
					sb.WriteString(fmt.Sprintf("- [%s] %s\n", r.Query, r.Content))
				}
				sb.WriteString("\n")
			}
		}
	} else {
		// No device specified — list all devices briefly
		devices, err := db.ListDevices()
		if err == nil && len(devices) > 0 {
			sb.WriteString("## All Devices\n")
			for _, d := range devices {
				sb.WriteString(fmt.Sprintf("- %s (%s) vendor=%s\n", d.ID, d.Hostname, d.Vendor))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
