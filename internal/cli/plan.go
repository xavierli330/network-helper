package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/plan"
)

func newPlanCmd() *cobra.Command {
	p := &cobra.Command{
		Use:   "plan",
		Short: "Generate change plans",
	}
	p.AddCommand(newPlanIsolateCmd())
	return p
}

func newPlanIsolateCmd() *cobra.Command {
	var format string
	var check bool
	var output string

	cmd := &cobra.Command{
		Use:   "isolate <device-id>",
		Short: "Generate a device isolation change plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceID := strings.ToLower(args[0])

			// 1. Resolve device
			device, err := db.GetDevice(deviceID)
			if err != nil {
				return fmt.Errorf("device %q not found: %w", deviceID, err)
			}

			// 2. Discover links
			links, err := plan.DiscoverLinks(db, deviceID)
			if err != nil {
				return fmt.Errorf("discover links: %w", err)
			}
			if len(links) == 0 {
				fmt.Fprintf(os.Stderr, "warning: no links discovered for %s — plan may be incomplete\n", deviceID)
			}

			// After discovering links, check if coverage is suspicious
			ifaces, _ := db.GetInterfaces(deviceID)
			physicalCount := 0
			for _, iface := range ifaces {
				if iface.Status == "up" && iface.Description != "" {
					physicalCount++
				}
			}
			if len(links) > 0 && physicalCount > 0 && len(links) < physicalCount/2 {
				fmt.Fprintf(os.Stderr, "⚠️  警告: 仅发现 %d 条互联关系，但设备有 %d 个有描述的活跃接口。\n", len(links), physicalCount)
				fmt.Fprintf(os.Stderr, "    建议: 导入更多对端设备日志，或采集 'display lldp neighbor brief' 补充拓扑数据。\n\n")
			}

			// 3. Impact analysis
			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}
			affected := graph.ImpactAnalysis(g, deviceID, graph.NodeTypeDevice)
			impactHostnames := make([]string, 0, len(affected))
			for _, id := range affected {
				hostname := id
				if n, ok := g.GetNode(id); ok && n.Props["hostname"] != "" {
					hostname = n.Props["hostname"]
				}
				impactHostnames = append(impactHostnames, hostname)
			}

			// 4. Build plan
			p := plan.BuildIsolationPlan(plan.PlanInput{
				TargetDevice:   deviceID,
				TargetHostname: device.Hostname,
				TargetVendor:   device.Vendor,
				Links:          links,
				IsSPOF:         len(affected) > 0,
				ImpactDevices:  impactHostnames,
			})

			// 5. Pre-check (optional)
			if check {
				result, err := runPreCheck(deviceID, links)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: pre-check failed: %v\n", err)
				} else {
					// Add results to Phase 1 notes
					notes := formatPreCheckResult(result)
					if len(p.Phases) > 1 {
						p.Phases[1].Notes = append(notes, p.Phases[1].Notes...)
					}

					// Persist baseline to scratch_entries
					checkJSON, _ := json.MarshalIndent(result, "", "  ")
					_, scratchErr := db.InsertScratch(model.ScratchEntry{
						DeviceID: deviceID,
						Category: "raw",
						Query:    "plan isolate --check baseline",
						Content:  string(checkJSON),
					})
					if scratchErr != nil {
						fmt.Fprintf(os.Stderr, "warning: could not persist pre-check baseline: %v\n", scratchErr)
					}
				}
			}

			// 6. Render
			var rendered string
			switch strings.ToLower(format) {
			case "markdown", "md":
				rendered = plan.RenderMarkdown(p)
			default:
				rendered = plan.RenderText(p)
			}

			// 7. Output
			if output != "" {
				if err := os.WriteFile(output, []byte(rendered), 0o644); err != nil {
					return fmt.Errorf("write output file: %w", err)
				}
				fmt.Fprintf(os.Stderr, "plan written to %s\n", output)
			} else {
				fmt.Print(rendered)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text or markdown")
	cmd.Flags().BoolVar(&check, "check", false, "run pre-check and embed baseline into Phase 1 notes")
	cmd.Flags().StringVar(&output, "output", "", "write plan to file instead of stdout")

	return cmd
}

// runPreCheck gathers current protocol/interface state and returns a PreCheckResult.
func runPreCheck(deviceID string, links []plan.Link) (plan.PreCheckResult, error) {
	result := plan.PreCheckResult{
		DeviceID: deviceID,
	}

	// Get latest snapshot
	snapID, err := db.LatestSnapshotID(deviceID)
	if err != nil {
		return result, fmt.Errorf("latest snapshot: %w", err)
	}

	// Get neighbors
	neighbors, err := db.GetNeighbors(deviceID, snapID)
	if err != nil {
		return result, fmt.Errorf("get neighbors: %w", err)
	}

	// Check OSPF
	ospfAllFull := true
	for _, n := range neighbors {
		if strings.EqualFold(n.Protocol, "ospf") {
			result.OSPFPeerCount++
			state := strings.ToLower(n.State)
			if state != "full" && state != "2-way" && state != "2way" {
				ospfAllFull = false
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("OSPF neighbor %s is in state %s (not Full/2-Way)", n.RemoteID, n.State))
			}
		}
	}
	result.OSPFAllFull = ospfAllFull || result.OSPFPeerCount == 0

	// Check BGP
	bgpAllEstab := true
	for _, n := range neighbors {
		if strings.EqualFold(n.Protocol, "bgp") {
			result.BGPPeerCount++
			state := strings.ToLower(n.State)
			if state != "established" {
				bgpAllEstab = false
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("BGP peer %s is in state %s (not Established)", n.RemoteID, n.State))
			}
		}
	}
	result.BGPAllEstab = bgpAllEstab || result.BGPPeerCount == 0

	// Get interfaces
	ifaces, err := db.GetInterfaces(deviceID)
	if err != nil {
		return result, fmt.Errorf("get interfaces: %w", err)
	}
	result.InterfaceTotal = len(ifaces)
	for _, iface := range ifaces {
		if strings.EqualFold(iface.Status, "up") {
			result.InterfaceUp++
		}
	}

	// Determine safety
	result.Safe = result.OSPFAllFull && result.BGPAllEstab && len(result.Warnings) == 0

	return result, nil
}

// formatPreCheckResult formats a PreCheckResult as human-readable notes.
func formatPreCheckResult(r plan.PreCheckResult) []string {
	var notes []string

	if r.Safe {
		notes = append(notes, "✅ 预检查通过: 所有协议邻居状态正常，可以继续变更")
	} else {
		notes = append(notes, "❌ 预检查失败: 存在异常，请先处理现有故障再继续变更")
	}

	notes = append(notes,
		fmt.Sprintf("OSPF 邻居数: %d — 全部 Full/2-Way: %v", r.OSPFPeerCount, r.OSPFAllFull),
		fmt.Sprintf("BGP 对等体数: %d — 全部 Established: %v", r.BGPPeerCount, r.BGPAllEstab),
		fmt.Sprintf("接口: %d/%d Up", r.InterfaceUp, r.InterfaceTotal),
	)

	for _, w := range r.Warnings {
		notes = append(notes, "⚠️  "+w)
	}

	return notes
}
