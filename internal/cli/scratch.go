package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newShowScratchCmd() *cobra.Command {
	var deviceID, category string
	var limit int
	var showID int
	cmd := &cobra.Command{
		Use:   "scratch",
		Short: "Show scratch pad entries (temporary command outputs)",
		Long: `The scratch pad stores large command outputs (routing tables, forwarding tables,
label tables) and specific object queries. Data is kept in FIFO order with
a 200-entry limit. Use this to review recent command outputs during troubleshooting.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Show a specific entry by ID
			if showID > 0 {
				entry, err := db.GetScratch(showID)
				if err != nil {
					return fmt.Errorf("scratch entry #%d not found", showID)
				}
				fmt.Printf("--- Scratch #%d [%s] %s @ %s ---\n",
					entry.ID, entry.Category, entry.DeviceID,
					entry.CreatedAt.Format("2006-01-02 15:04:05"))
				fmt.Printf("Command: %s\n\n", entry.Query)
				fmt.Println(entry.Content)
				return nil
			}

			// List entries
			entries, err := db.ListScratch(deviceID, category, limit)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("Scratch pad is empty.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tDEVICE\tCATEGORY\tCOMMAND\tSIZE\tTIME\n")
			for _, e := range entries {
				cmd := e.Query
				if len(cmd) > 50 {
					cmd = cmd[:50] + "..."
				}
				size := formatSize(len(e.Content))
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
					e.ID, e.DeviceID, e.Category, cmd, size,
					e.CreatedAt.Format("15:04:05"))
			}
			w.Flush()
			fmt.Printf("\nUse 'nethelper show scratch --id <N>' to view full content.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "filter by device ID")
	cmd.Flags().StringVar(&category, "category", "", "filter by category (route/fib/label/raw)")
	cmd.Flags().IntVar(&limit, "limit", 20, "max entries to show")
	cmd.Flags().IntVar(&showID, "id", 0, "show full content of a specific entry")
	return cmd
}

func newScratchClearCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear scratch pad",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := db.ClearScratch(deviceID); err != nil {
				return err
			}
			if deviceID != "" {
				fmt.Printf("Cleared scratch pad for device %s.\n", deviceID)
			} else {
				fmt.Println("Cleared all scratch pad entries.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "only clear entries for this device")
	return cmd
}

func formatSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	kb := float64(bytes) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.1fK", kb)
	}
	mb := kb / 1024
	return fmt.Sprintf("%.1fM", mb)
}

// newShowScratchCmd and newScratchClearCmd are registered in show.go and root.go
