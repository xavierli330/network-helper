package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
)

func newExportCmd() *cobra.Command {
	export := &cobra.Command{
		Use:   "export",
		Short: "Export data for backup or migration",
	}
	export.AddCommand(newExportDBCmd())
	export.AddCommand(newExportTopologyCmd())
	export.AddCommand(newExportReportCmd())
	return export
}

func newExportDBCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Export SQLite database (copy)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				output = fmt.Sprintf("nethelper-backup-%s.db", time.Now().Format("20060102-150405"))
			}

			src, err := os.Open(db.Path())
			if err != nil {
				return fmt.Errorf("open source: %w", err)
			}
			defer src.Close()

			dst, err := os.Create(output)
			if err != nil {
				return fmt.Errorf("create output: %w", err)
			}
			defer dst.Close()

			n, err := io.Copy(dst, src)
			if err != nil {
				return fmt.Errorf("copy: %w", err)
			}

			fmt.Printf("Exported database to %s (%d bytes)\n", output, n)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	return cmd
}

func newExportTopologyCmd() *cobra.Command {
	var format, output string
	cmd := &cobra.Command{
		Use:   "topology",
		Short: "Export network topology",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}

			var content string
			switch format {
			case "dot":
				content = exportDOT(g)
			case "json":
				content = exportJSON(g)
			default:
				content = exportDOT(g)
			}

			if output != "" {
				if err := os.WriteFile(output, []byte(content), 0644); err != nil {
					return fmt.Errorf("write file: %w", err)
				}
				fmt.Printf("Exported topology to %s (%s format)\n", output, format)
			} else {
				fmt.Print(content)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "dot", "output format (dot, json)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	return cmd
}

func exportDOT(g *graph.Graph) string {
	var sb strings.Builder
	sb.WriteString("digraph network {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box];\n\n")

	// Device nodes
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		label := n.Props["hostname"]
		if label == "" {
			label = n.ID
		}
		sb.WriteString(fmt.Sprintf("  %q [label=%q shape=box style=filled fillcolor=lightblue];\n",
			n.ID, label))
	}
	sb.WriteString("\n")

	// PEER edges only (device-to-device)
	seen := make(map[string]bool)
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		for _, e := range g.NeighborsByType(n.ID, graph.EdgePeer) {
			key := n.ID + "-" + e.To
			reverseKey := e.To + "-" + n.ID
			if seen[key] || seen[reverseKey] {
				continue
			}
			seen[key] = true
			label := e.Props["protocol"]
			sb.WriteString(fmt.Sprintf("  %q -> %q [label=%q dir=none];\n", n.ID, e.To, label))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

func exportJSON(g *graph.Graph) string {
	type jsonNode struct {
		ID    string            `json:"id"`
		Type  string            `json:"type"`
		Props map[string]string `json:"props"`
	}
	type jsonEdge struct {
		From  string            `json:"from"`
		To    string            `json:"to"`
		Type  string            `json:"type"`
		Props map[string]string `json:"props,omitempty"`
	}
	type jsonGraph struct {
		Nodes []jsonNode `json:"nodes"`
		Edges []jsonEdge `json:"edges"`
	}

	var jg jsonGraph
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		jg.Nodes = append(jg.Nodes, jsonNode{ID: n.ID, Type: string(n.Type), Props: n.Props})
	}
	for _, n := range g.NodesByType(graph.NodeTypeDevice) {
		for _, e := range g.NeighborsByType(n.ID, graph.EdgePeer) {
			jg.Edges = append(jg.Edges, jsonEdge{From: n.ID, To: e.To, Type: string(e.Type), Props: e.Props})
		}
	}

	data, _ := json.MarshalIndent(jg, "", "  ")
	return string(data) + "\n"
}

func newExportReportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate network status report (Markdown)",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}

			devices, _ := db.ListDevices()
			var sb strings.Builder

			sb.WriteString("# Network Status Report\n\n")
			sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

			// Summary
			sb.WriteString("## Summary\n\n")
			sb.WriteString(fmt.Sprintf("- **Devices:** %d\n", len(devices)))
			sb.WriteString(fmt.Sprintf("- **Interfaces:** %d\n", len(g.NodesByType(graph.NodeTypeInterface))))
			sb.WriteString(fmt.Sprintf("- **Subnets:** %d\n", len(g.NodesByType(graph.NodeTypeSubnet))))
			sb.WriteString(fmt.Sprintf("- **Graph edges:** %d\n\n", g.EdgeCount()))

			// Devices
			sb.WriteString("## Devices\n\n")
			sb.WriteString("| Hostname | Vendor | Model | Mgmt IP | Router-ID | Last Seen |\n")
			sb.WriteString("|----------|--------|-------|---------|-----------|-----------|\n")
			for _, d := range devices {
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
					d.Hostname, d.Vendor, d.Model, d.MgmtIP, d.RouterID, d.LastSeen.Format("2006-01-02 15:04")))
			}
			sb.WriteString("\n")

			// SPOFs
			spofs := graph.FindSPOF(g, graph.NodeTypeDevice)
			if len(spofs) > 0 {
				sb.WriteString("## Single Points of Failure\n\n")
				for _, s := range spofs {
					n, _ := g.GetNode(s)
					hostname := s
					if n != nil && n.Props["hostname"] != "" {
						hostname = n.Props["hostname"]
					}
					sb.WriteString(fmt.Sprintf("- %s\n", hostname))
				}
				sb.WriteString("\n")
			}

			content := sb.String()
			if output != "" {
				if err := os.WriteFile(output, []byte(content), 0644); err != nil {
					return fmt.Errorf("write file: %w", err)
				}
				fmt.Printf("Report exported to %s\n", output)
			} else {
				fmt.Print(content)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	return cmd
}
