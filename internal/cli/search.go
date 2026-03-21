package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	search := &cobra.Command{
		Use:   "search",
		Short: "Full-text search across data",
	}
	search.AddCommand(newSearchConfigCmd())
	search.AddCommand(newSearchLogCmd())
	search.AddCommand(newSearchCommandCmd())
	return search
}

func newSearchConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config <query>",
		Short: "Search configuration content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db.SyncConfigFTS() // ensure FTS is up to date
			results, err := db.SearchConfig(args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("No matching configs found.")
				return nil
			}
			for _, cs := range results {
				fmt.Printf("--- Device: %s [%s] from %s ---\n", cs.DeviceID, cs.CapturedAt.Format("2006-01-02 15:04"), cs.SourceFile)
				// Show snippet (first 500 chars)
				text := cs.ConfigText
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				fmt.Println(text)
				fmt.Println()
			}
			return nil
		},
	}
}

func newSearchLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log <query>",
		Short: "Search troubleshooting logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := db.SearchTroubleshootLogs(args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("No matching logs found.")
				return nil
			}
			for _, l := range results {
				fmt.Printf("--- Note #%d [%s] tags:%s ---\n", l.ID, l.CreatedAt.Format("2006-01-02"), l.Tags)
				fmt.Printf("Symptom:    %s\n", l.Symptom)
				if l.Findings != "" {
					fmt.Printf("Findings:   %s\n", l.Findings)
				}
				if l.Resolution != "" {
					fmt.Printf("Resolution: %s\n", l.Resolution)
				}
				fmt.Println()
			}
			return nil
		},
	}
}

func newSearchCommandCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "command <query>",
		Short: "Search command references",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := db.SearchCommands(args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("No matching commands found.")
				return nil
			}
			for _, r := range results {
				fmt.Printf("[%s] %s\n  %s\n\n", r.Vendor, r.Command, r.Description)
			}
			return nil
		},
	}
}
