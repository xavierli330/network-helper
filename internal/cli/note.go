package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/model"
)

func newNoteCmd() *cobra.Command {
	note := &cobra.Command{
		Use:   "note",
		Short: "Troubleshooting notes",
	}
	note.AddCommand(newNoteAddCmd())
	note.AddCommand(newNoteListCmd())
	note.AddCommand(newNoteSearchCmd())
	return note
}

func newNoteAddCmd() *cobra.Command {
	var symptom, findings, resolution, tags, deviceID, commands string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a troubleshooting note",
		RunE: func(cmd *cobra.Command, args []string) error {
			if symptom == "" {
				return fmt.Errorf("--symptom is required")
			}
			log := model.TroubleshootLog{
				DeviceID:     deviceID,
				Symptom:      symptom,
				CommandsUsed: commands,
				Findings:     findings,
				Resolution:   resolution,
				Tags:         tags,
			}
			id, err := db.InsertTroubleshootLog(log)
			if err != nil {
				return fmt.Errorf("insert: %w", err)
			}
			fmt.Printf("Note #%d created.\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&symptom, "symptom", "", "problem symptom (required)")
	cmd.Flags().StringVar(&findings, "finding", "", "what you found")
	cmd.Flags().StringVar(&resolution, "resolution", "", "how it was resolved")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tags (e.g., ospf,mtu)")
	cmd.Flags().StringVar(&deviceID, "device", "", "related device ID")
	cmd.Flags().StringVar(&commands, "commands", "", "commands used during troubleshooting")
	return cmd
}

func newNoteListCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List troubleshooting notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			logs, err := db.ListTroubleshootLogs(limit, 0)
			if err != nil {
				return err
			}
			if len(logs) == 0 {
				fmt.Println("No notes found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tDATE\tSYMPTOM\tTAGS\n")
			for _, l := range logs {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", l.ID, l.CreatedAt.Format("2006-01-02"), l.Symptom, l.Tags)
			}
			return w.Flush()
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max results")
	return cmd
}

func newNoteSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search troubleshooting notes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := db.SearchTroubleshootLogs(args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("No matching notes found.")
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
