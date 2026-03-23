// internal/mcp/tools_plan.go
package mcp

import (
	"context"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/plan"
	"github.com/xavierli/nethelper/internal/store"
)

func registerPlanTools(s *mcpserver.MCPServer, db *store.DB) {
	// 1. plan_isolate — generate device isolation plan
	s.AddTool(mcpmcp.NewTool("plan_isolate",
		mcpmcp.WithDescription("Generate a 6-phase device isolation change plan in Markdown format"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID to isolate")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		deviceID := req.GetString("device_id", "")
		if deviceID == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}

		topo, err := plan.BuildTopology(db, deviceID)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to build topology: " + err.Error()), nil
		}

		p := plan.GenerateIsolationPlanV2(topo)
		return mcpmcp.NewToolResultText(plan.RenderMarkdown(p)), nil
	})

	// 2. plan_upgrade — generate device upgrade plan
	s.AddTool(mcpmcp.NewTool("plan_upgrade",
		mcpmcp.WithDescription("Generate an 8-phase device software upgrade plan in Markdown format"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID to upgrade")),
		mcpmcp.WithString("version", mcpmcp.Required(), mcpmcp.Description("Target software version (e.g. V200R021C10SPC600)")),
		mcpmcp.WithString("file", mcpmcp.Required(), mcpmcp.Description("Firmware file name on the device (e.g. firmware.cc)")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		deviceID := req.GetString("device_id", "")
		version := req.GetString("version", "")
		file := req.GetString("file", "")

		if deviceID == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		if version == "" {
			return mcpmcp.NewToolResultError("version is required"), nil
		}
		if file == "" {
			return mcpmcp.NewToolResultError("file is required"), nil
		}

		topo, err := plan.BuildTopology(db, deviceID)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to build topology: " + err.Error()), nil
		}

		params := plan.UpgradeParams{
			TargetVersion: version,
			FirmwareFile:  file,
		}
		p := plan.GenerateUpgradePlan(topo, params)
		return mcpmcp.NewToolResultText(plan.RenderMarkdown(p)), nil
	})

	// 3. plan_cutover — generate link cutover plan
	s.AddTool(mcpmcp.NewTool("plan_cutover",
		mcpmcp.WithDescription("Generate a 7-phase link migration (cutover) plan in Markdown format"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID on which the cutover will occur")),
		mcpmcp.WithString("old_interface", mcpmcp.Required(), mcpmcp.Description("The interface to migrate traffic away from (e.g. Eth-Trunk1)")),
		mcpmcp.WithString("new_interface", mcpmcp.Required(), mcpmcp.Description("The interface to migrate traffic onto (e.g. Eth-Trunk200)")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		deviceID := req.GetString("device_id", "")
		oldIface := req.GetString("old_interface", "")
		newIface := req.GetString("new_interface", "")

		if deviceID == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		if oldIface == "" {
			return mcpmcp.NewToolResultError("old_interface is required"), nil
		}
		if newIface == "" {
			return mcpmcp.NewToolResultError("new_interface is required"), nil
		}

		topo, err := plan.BuildTopology(db, deviceID)
		if err != nil {
			return mcpmcp.NewToolResultError("failed to build topology: " + err.Error()), nil
		}

		params := plan.CutoverParams{
			OldInterface: oldIface,
			NewInterface: newIface,
		}
		p := plan.GenerateCutoverPlan(topo, params)
		return mcpmcp.NewToolResultText(plan.RenderMarkdown(p)), nil
	})
}
