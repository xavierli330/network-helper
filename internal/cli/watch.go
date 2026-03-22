package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/watcher"
)

func newWatchCmd() *cobra.Command {
	watch := &cobra.Command{
		Use:   "watch",
		Short: "File monitoring and ingestion",
	}
	watch.AddCommand(newWatchIngestCmd())
	watch.AddCommand(newWatchStartCmd())
	watch.AddCommand(newWatchStopCmd())
	watch.AddCommand(newWatchStatusCmd())
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
			if err != nil { return fmt.Errorf("read file: %w", err) }

			result, err := pipeline.Ingest(filePath, string(data))
			if err != nil { return fmt.Errorf("ingest: %w", err) }

			ing := model.LogIngestion{FilePath: filePath, LastOffset: int64(len(data)), ProcessedAt: time.Now()}
			if err := db.UpsertIngestion(ing); err != nil { return fmt.Errorf("record ingestion: %w", err) }

			fmt.Printf("Ingested %s (%d bytes)\n", filePath, len(data))
			fmt.Printf("  Devices: %d, Blocks parsed: %d, Failed: %d, Skipped: %d\n",
				result.DevicesFound, result.BlocksParsed, result.BlocksFailed, result.BlocksSkipped)
			return nil
		},
	}
}

func pidFilePath() string {
	return filepath.Join(cfg.DataDir, "watcher.pid")
}

func newWatchStartCmd() *cobra.Command {
	var dirs []string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start file watcher in foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			watchDirs := dirs
			if len(watchDirs) == 0 { watchDirs = cfg.WatchDirs }
			if len(watchDirs) == 0 { return fmt.Errorf("no watch directories specified (use --dir or set watch_dirs in config)") }

			pidFile := pidFilePath()
			if watcher.IsRunning(pidFile) { return fmt.Errorf("watcher already running (PID file: %s)", pidFile) }

			// Mutex to serialize file processing (spec: 并发安全)
			var ingestMu sync.Mutex

			w, err := watcher.New(watcher.Config{
				Dirs:     watchDirs,
				Debounce: 500 * time.Millisecond,
				OnFileChange: func(path string) {
					ingestMu.Lock()
					defer ingestMu.Unlock()

					// Incremental read: check last_offset from store
					var offset int64
					ing, err := db.GetIngestion(path)
					if err == nil { offset = ing.LastOffset }

					f, err := os.Open(path)
					if err != nil {
						fmt.Fprintf(os.Stderr, "open %s: %v\n", path, err)
						return
					}
					defer f.Close()

					info, err := f.Stat()
					if err != nil {
						fmt.Fprintf(os.Stderr, "stat %s: %v\n", path, err)
						return
					}

					// If file is smaller than offset, it was rotated — read from start
					if info.Size() < offset { offset = 0 }
					if info.Size() == offset { return } // no new content

					// Read only new content from offset
					newData := make([]byte, info.Size()-offset)
					if _, err := f.ReadAt(newData, offset); err != nil {
						fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
						return
					}

					result, err := pipeline.IngestIncremental(path, string(newData))
					if err != nil {
						fmt.Fprintf(os.Stderr, "ingest %s: %v\n", path, err)
						return
					}

					newIng := model.LogIngestion{FilePath: path, LastOffset: offset + int64(result.BytesConsumed), ProcessedAt: time.Now()}
					db.UpsertIngestion(newIng)
					fmt.Printf("[%s] Ingested %s (new=%d bytes, devices=%d, parsed=%d)\n",
						time.Now().Format("15:04:05"), filepath.Base(path),
						len(newData), result.DevicesFound, result.BlocksParsed)
				},
			})
			if err != nil { return fmt.Errorf("create watcher: %w", err) }

			if err := watcher.WritePID(pidFile); err != nil {
				return fmt.Errorf("write PID: %w", err)
			}

			fmt.Printf("Watching directories: %v\n", watchDirs)
			fmt.Println("Press Ctrl+C to stop.")

			// Handle Ctrl+C / SIGTERM for graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigCh
				fmt.Println("\nStopping watcher...")
				w.Stop()
			}()

			w.Start() // blocks until w.Stop() is called
			watcher.RemovePID(pidFile)
			fmt.Println("Watcher stopped.")
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&dirs, "dir", nil, "directories to watch (can specify multiple)")
	return cmd
}

func newWatchStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the watcher daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidFile := pidFilePath()
			if !watcher.IsRunning(pidFile) {
				fmt.Println("Watcher is not running.")
				return nil
			}
			pid, err := watcher.ReadPID(pidFile)
			if err != nil { return err }

			p, err := os.FindProcess(pid)
			if err != nil { return fmt.Errorf("find process: %w", err) }

			if err := p.Signal(os.Interrupt); err != nil {
				return fmt.Errorf("send signal: %w", err)
			}
			watcher.RemovePID(pidFile)
			fmt.Printf("Stopped watcher (PID %d)\n", pid)
			return nil
		},
	}
}

func newWatchStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show watcher status",
		Run: func(cmd *cobra.Command, args []string) {
			pidFile := pidFilePath()
			if watcher.IsRunning(pidFile) {
				pid, _ := watcher.ReadPID(pidFile)
				fmt.Printf("Watcher is running (PID %d)\n", pid)
			} else {
				fmt.Println("Watcher is not running.")
			}
		},
	}
}
