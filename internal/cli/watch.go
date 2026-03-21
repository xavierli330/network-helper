package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/model"
)

func newWatchCmd() *cobra.Command {
	watch := &cobra.Command{
		Use:   "watch",
		Short: "File monitoring and ingestion",
	}
	watch.AddCommand(newWatchIngestCmd())
	watch.AddCommand(newWatchStartStub())
	watch.AddCommand(newWatchStopStub())
	watch.AddCommand(newWatchStatusStub())
	return watch
}

func newWatchIngestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ingest <file>",
		Short: "Manually import a log file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}
			ing := model.LogIngestion{
				FilePath:    filePath,
				LastOffset:  int64(len(data)),
				ProcessedAt: time.Now(),
			}
			if err := db.UpsertIngestion(ing); err != nil {
				return fmt.Errorf("record ingestion: %w", err)
			}
			fmt.Printf("Ingested %s (%d bytes)\n", filePath, len(data))
			fmt.Println("Note: parsing not yet implemented (Plan 2). Raw file recorded.")
			return nil
		},
	}
}

func newWatchStartStub() *cobra.Command {
	return &cobra.Command{
		Use: "start", Short: "Start file watcher daemon (not yet implemented)",
		Run: func(cmd *cobra.Command, args []string) { fmt.Println("Watch daemon not yet implemented (Plan 3).") },
	}
}

func newWatchStopStub() *cobra.Command {
	return &cobra.Command{
		Use: "stop", Short: "Stop file watcher daemon (not yet implemented)",
		Run: func(cmd *cobra.Command, args []string) { fmt.Println("Watch daemon not yet implemented (Plan 3).") },
	}
}

func newWatchStatusStub() *cobra.Command {
	return &cobra.Command{
		Use: "status", Short: "Show watcher status (not yet implemented)",
		Run: func(cmd *cobra.Command, args []string) { fmt.Println("Watch daemon not yet implemented (Plan 3).") },
	}
}
