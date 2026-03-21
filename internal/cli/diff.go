package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/diff"
	"github.com/xavierli/nethelper/internal/model"
)

func newDiffCmd() *cobra.Command {
	d := &cobra.Command{
		Use:   "diff",
		Short: "Compare configurations or data between time points",
	}
	d.AddCommand(newDiffConfigCmd())
	d.AddCommand(newDiffRouteCmd())
	return d
}

func newDiffConfigCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Compare configuration snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}

			snapshots, err := db.GetConfigSnapshots(deviceID)
			if err != nil {
				return err
			}
			if len(snapshots) < 2 {
				fmt.Println("Need at least 2 config snapshots to diff.")
				return nil
			}

			newer := snapshots[0]
			older := snapshots[1]

			result := diff.Unified(older.ConfigText, newer.ConfigText,
				fmt.Sprintf("%s [%s]", deviceID, older.CapturedAt.Format("2006-01-02 15:04")),
				fmt.Sprintf("%s [%s]", deviceID, newer.CapturedAt.Format("2006-01-02 15:04")))

			if result == "" {
				fmt.Println("No configuration changes between the two snapshots.")
				return nil
			}
			fmt.Println(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newDiffRouteCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Compare routing tables between snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}

			snapIDs, err := db.GetRIBSnapshotIDs(deviceID, 2)
			if err != nil || len(snapIDs) < 2 {
				fmt.Println("Need at least 2 route snapshots to diff.")
				return nil
			}

			newer, _ := db.GetRIBEntries(deviceID, snapIDs[0])
			older, _ := db.GetRIBEntries(deviceID, snapIDs[1])

			oldText := routeEntriesToText(older)
			newText := routeEntriesToText(newer)

			result := diff.Unified(oldText, newText,
				fmt.Sprintf("snapshot-%d", snapIDs[1]),
				fmt.Sprintf("snapshot-%d", snapIDs[0]))

			if result == "" {
				fmt.Println("No routing table changes between the two snapshots.")
				return nil
			}
			fmt.Println(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func routeEntriesToText(entries []model.RIBEntry) string {
	var lines []string
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("%-20s %-8s %-4d %-6d %-16s %s",
			fmt.Sprintf("%s/%d", e.Prefix, e.MaskLen), e.Protocol, e.Preference, e.Metric, e.NextHop, e.OutgoingInterface))
	}
	return strings.Join(lines, "\n")
}
