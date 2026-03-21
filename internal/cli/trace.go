// internal/cli/trace.go
package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
)

func newTraceCmd() *cobra.Command {
	trace := &cobra.Command{
		Use:   "trace",
		Short: "Path analysis and impact assessment",
	}
	trace.AddCommand(newTracePathCmd())
	trace.AddCommand(newTraceImpactCmd())
	return trace
}

func newTracePathCmd() *cobra.Command {
	var from, to string
	var all bool
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Trace path between two devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("--from and --to are required")
			}

			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}

			from = strings.ToLower(from)
			to = strings.ToLower(to)

			if all {
				paths := graph.AllPaths(g, from, to, 10)
				if len(paths) == 0 {
					fmt.Printf("No path found from %s to %s\n", from, to)
					return nil
				}
				fmt.Printf("Found %d path(s) from %s to %s:\n\n", len(paths), from, to)
				for i, path := range paths {
					fmt.Printf("  Path %d (%d hops): %s\n", i+1, len(path)-1, strings.Join(path, " → "))
				}
				return nil
			}

			path, err := graph.ShortestPath(g, from, to)
			if err != nil {
				return err
			}

			fmt.Printf("Shortest path from %s to %s (%d hops):\n\n", from, to, len(path)-1)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "HOP\tNODE\tTYPE\tDETAILS\n")
			for i, nodeID := range path {
				n, ok := g.GetNode(nodeID)
				if !ok {
					continue
				}
				details := ""
				if n.Props["hostname"] != "" {
					details = n.Props["hostname"]
				}
				if n.Props["ip"] != "" {
					details = n.Props["ip"]
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i, nodeID, n.Type, details)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "source device ID")
	cmd.Flags().StringVar(&to, "to", "", "destination device ID")
	cmd.Flags().BoolVar(&all, "all", false, "show all paths (not just shortest)")
	return cmd
}

func newTraceImpactCmd() *cobra.Command {
	var node string
	cmd := &cobra.Command{
		Use:   "impact",
		Short: "Analyze impact of a node/link failure",
		RunE: func(cmd *cobra.Command, args []string) error {
			if node == "" {
				return fmt.Errorf("--node is required")
			}

			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}

			node = strings.ToLower(node)
			affected := graph.ImpactAnalysis(g, node, graph.NodeTypeDevice)

			if len(affected) == 0 {
				fmt.Printf("Removing %s has no impact — all remaining devices stay connected.\n", node)
				return nil
			}

			fmt.Printf("Removing %s would isolate %d device(s):\n\n", node, len(affected))
			for _, id := range affected {
				n, ok := g.GetNode(id)
				hostname := id
				if ok && n.Props["hostname"] != "" {
					hostname = n.Props["hostname"]
				}
				fmt.Printf("  - %s (%s)\n", id, hostname)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&node, "node", "", "node to simulate removing")
	return cmd
}
