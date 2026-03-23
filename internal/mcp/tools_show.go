// internal/mcp/tools_show.go
package mcp

import (
	"context"
	"encoding/json"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/store"
)

func registerShowTools(s *mcpserver.MCPServer, db *store.DB) {
	// 1. show_devices — list all network devices
	s.AddTool(mcpmcp.NewTool("show_devices",
		mcpmcp.WithDescription("List all network devices stored in the database"),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		devices, err := db.ListDevices()
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(devices, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 2. show_device — get a single device by ID
	s.AddTool(mcpmcp.NewTool("show_device",
		mcpmcp.WithDescription("Get details for a specific network device"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID to look up")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		id := req.GetString("device_id", "")
		if id == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		device, err := db.GetDevice(id)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(device, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 3. show_interfaces — list interfaces for a device
	s.AddTool(mcpmcp.NewTool("show_interfaces",
		mcpmcp.WithDescription("List all interfaces for a specific network device"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID whose interfaces to retrieve")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		id := req.GetString("device_id", "")
		if id == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		ifaces, err := db.GetInterfaces(id)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(ifaces, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 4. show_neighbors — list neighbors for a device
	s.AddTool(mcpmcp.NewTool("show_neighbors",
		mcpmcp.WithDescription("List the latest neighbors (LLDP/CDP) for a specific network device"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID whose neighbors to retrieve")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		id := req.GetString("device_id", "")
		if id == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		neighbors, err := db.GetLatestNeighbors(id)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(neighbors, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 5. show_bgp_peers — list BGP peers for a device
	s.AddTool(mcpmcp.NewTool("show_bgp_peers",
		mcpmcp.WithDescription("List the latest BGP peers for a specific network device"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID whose BGP peers to retrieve")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		id := req.GetString("device_id", "")
		if id == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		peers, err := db.GetLatestBGPPeers(id)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(peers, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 6. show_topology — summary of all devices and their neighbor counts
	s.AddTool(mcpmcp.NewTool("show_topology",
		mcpmcp.WithDescription("Show a topology summary: all devices with their neighbor counts"),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		devices, err := db.ListDevices()
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}

		type deviceSummary struct {
			ID            string `json:"id"`
			Hostname      string `json:"hostname"`
			Vendor        string `json:"vendor"`
			NeighborCount int    `json:"neighbor_count"`
		}

		summary := make([]deviceSummary, 0, len(devices))
		for _, d := range devices {
			neighbors, _ := db.GetLatestNeighbors(d.ID)
			summary = append(summary, deviceSummary{
				ID:            d.ID,
				Hostname:      d.Hostname,
				Vendor:        d.Vendor,
				NeighborCount: len(neighbors),
			})
		}

		data, _ := json.MarshalIndent(summary, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 7. show_routes — last 10 route scratch entries for a device
	s.AddTool(mcpmcp.NewTool("show_routes",
		mcpmcp.WithDescription("Show the latest routing table entries (up to 10) for a specific network device"),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID whose routes to retrieve")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		id := req.GetString("device_id", "")
		if id == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		routes, err := db.ListScratch(id, "route", 10)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(routes, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})
}
