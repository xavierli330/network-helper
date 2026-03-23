// internal/mcp/tools_show.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/plan"
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

	// 5. show_bgp_peers — list BGP peers, summarized by default
	s.AddTool(mcpmcp.NewTool("show_bgp_peers",
		mcpmcp.WithDescription("List BGP peers for a device. Returns a per-group summary by default. Use peer_group param to see individual peers in a specific group."),
		mcpmcp.WithString("device_id", mcpmcp.Required(), mcpmcp.Description("The device ID")),
		mcpmcp.WithString("peer_group", mcpmcp.Description("Optional: show individual peers for this group only (e.g. 'LA1', 'QCDR')")),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		id := req.GetString("device_id", "")
		if id == "" {
			return mcpmcp.NewToolResultError("device_id is required"), nil
		}
		peers, err := db.GetLatestBGPPeers(id)
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}

		filterGroup := req.GetString("peer_group", "")

		// If a specific group requested, return full detail for that group only
		if filterGroup != "" {
			var filtered []interface{}
			for _, p := range peers {
				if strings.EqualFold(p.PeerGroup, filterGroup) {
					filtered = append(filtered, map[string]interface{}{
						"peer_ip":     p.PeerIP,
						"remote_as":   p.RemoteAS,
						"description": p.Description,
						"vrf":         p.VRF,
					})
				}
			}
			data, _ := json.MarshalIndent(filtered, "", "  ")
			return mcpmcp.NewToolResultText(string(data)), nil
		}

		// Default: return per-group summary (much smaller)
		type groupSummary struct {
			PeerGroup string `json:"peer_group"`
			PeerCount int    `json:"peer_count"`
			RemoteASes string `json:"remote_as"`
			SamplePeers []string `json:"sample_peers,omitempty"`
		}
		groups := make(map[string]*groupSummary)
		var order []string
		asMap := make(map[string]map[int]bool)
		for _, p := range peers {
			g := p.PeerGroup
			if g == "" { g = "(ungrouped)" }
			if _, ok := groups[g]; !ok {
				groups[g] = &groupSummary{PeerGroup: g}
				order = append(order, g)
				asMap[g] = make(map[int]bool)
			}
			groups[g].PeerCount++
			asMap[g][p.RemoteAS] = true
			if len(groups[g].SamplePeers) < 3 {
				desc := p.PeerIP
				if p.Description != "" { desc = p.Description + " (" + p.PeerIP + ")" }
				groups[g].SamplePeers = append(groups[g].SamplePeers, desc)
			}
		}
		var result []groupSummary
		for _, g := range order {
			gs := groups[g]
			asList := make([]int, 0, len(asMap[g]))
			for as := range asMap[g] { asList = append(asList, as) }
			if len(asList) == 1 {
				gs.RemoteASes = fmt.Sprintf("%d", asList[0])
			} else {
				gs.RemoteASes = fmt.Sprintf("%d distinct ASes", len(asList))
			}
			result = append(result, *gs)
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcpmcp.NewToolResultText(string(data)), nil
	})

	// 6. show_topology — topology inferred from config (BGP peers, interface descriptions, subnets)
	s.AddTool(mcpmcp.NewTool("show_topology",
		mcpmcp.WithDescription("Show network topology: per-device interconnections inferred from BGP config, interface descriptions, and subnet matching. Shows peer groups, LAGs, protocols, and SPOF status."),
	), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		devices, err := db.ListDevices()
		if err != nil {
			return mcpmcp.NewToolResultError(err.Error()), nil
		}

		type peerGroupSummary struct {
			Name     string `json:"name"`
			Role     string `json:"role"`
			PeerCount int   `json:"peer_count"`
		}
		type lagSummary struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		type deviceTopo struct {
			ID         string             `json:"id"`
			Hostname   string             `json:"hostname"`
			Vendor     string             `json:"vendor"`
			LocalAS    int                `json:"local_as,omitempty"`
			Protocols  []string           `json:"protocols,omitempty"`
			PeerGroups []peerGroupSummary `json:"peer_groups,omitempty"`
			LAGs       []lagSummary       `json:"lags,omitempty"`
			PhysicalLinks int             `json:"physical_links"`
			IsSPOF     bool               `json:"is_spof"`
		}

		var result []deviceTopo
		for _, d := range devices {
			topo, err := plan.BuildTopology(db, d.ID)
			if err != nil {
				result = append(result, deviceTopo{
					ID: d.ID, Hostname: d.Hostname, Vendor: d.Vendor,
				})
				continue
			}
			dt := deviceTopo{
				ID:            d.ID,
				Hostname:      d.Hostname,
				Vendor:        d.Vendor,
				LocalAS:       topo.LocalAS,
				Protocols:     topo.Protocols,
				PhysicalLinks: len(topo.PhysicalLinks),
				IsSPOF:        topo.IsSPOF,
			}
			for _, pg := range topo.PeerGroups {
				dt.PeerGroups = append(dt.PeerGroups, peerGroupSummary{
					Name: pg.Name, Role: string(pg.Role), PeerCount: len(pg.Peers),
				})
			}
			for _, lag := range topo.LAGs {
				dt.LAGs = append(dt.LAGs, lagSummary{
					Name: lag.Name, Description: lag.Description,
				})
			}
			result = append(result, dt)
		}

		data, _ := json.MarshalIndent(result, "", "  ")
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
