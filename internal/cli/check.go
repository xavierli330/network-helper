// internal/cli/check.go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
)

func newCheckCmd() *cobra.Command {
	check := &cobra.Command{
		Use:   "check",
		Short: "Health checks and consistency verification",
	}
	check.AddCommand(newCheckLoopCmd())
	check.AddCommand(newCheckSPOFCmd())
	return check
}

func newCheckLoopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "loop",
		Short: "Detect routing loops",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}

			loops := graph.DetectLoops(g)
			if len(loops) == 0 {
				fmt.Println("No loops detected.")
				return nil
			}

			fmt.Printf("Found %d potential loop(s):\n\n", len(loops))
			for i, loop := range loops {
				fmt.Printf("  Loop %d: %v\n", i+1, loop)
			}
			return nil
		},
	}
}

func newCheckSPOFCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "spof",
		Short: "Find single points of failure",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}

			spofs := graph.FindSPOF(g, graph.NodeTypeDevice)
			if len(spofs) == 0 {
				fmt.Println("No single points of failure found.")
				return nil
			}

			fmt.Printf("Found %d single point(s) of failure:\n\n", len(spofs))
			for _, id := range spofs {
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
}
