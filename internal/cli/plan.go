package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
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

			// 1. Build topology (includes device lookup, link discovery, SPOF, impact analysis)
			topo, err := plan.BuildTopology(db, deviceID)
			if err != nil {
				return fmt.Errorf("build topology for %q: %w", deviceID, err)
			}

			// 2. Generate v2 plan
			p := plan.GenerateIsolationPlanV2(topo)

			// 3. Pre-check (optional)
			if check {
				result, err := runPreCheck(topo)
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

			// 4. Render
			var rendered string
			switch strings.ToLower(format) {
			case "markdown", "md":
				rendered = plan.RenderMarkdown(p)
			default:
				rendered = plan.RenderText(p)
			}

			// 5. Output
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
// It uses topo.PeerGroups to know which BGP peers to verify.
func runPreCheck(topo plan.DeviceTopology) (plan.PreCheckResult, error) {
	deviceID := topo.DeviceID
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

	// Build a set of expected BGP peer IPs from topology peer groups
	expectedBGPPeers := make(map[string]bool)
	for _, pg := range topo.PeerGroups {
		for _, peer := range pg.Peers {
			expectedBGPPeers[peer.PeerIP] = true
		}
	}

	// Check BGP — verify each neighbor entry; also warn for expected peers missing from neighbors
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
			// Mark peer as seen
			delete(expectedBGPPeers, n.RemoteID)
		}
	}

	// Warn about expected peers not found in neighbor table at all
	for peerIP := range expectedBGPPeers {
		bgpAllEstab = false
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("BGP peer %s (from topology) not found in neighbor table", peerIP))
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
