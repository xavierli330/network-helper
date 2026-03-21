package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/config"
	"github.com/xavierli/nethelper/internal/store"
)

var (
	cfgFile string
	dbPath  string
	cfg     *config.Config
	db      *store.DB
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nethelper",
		Short: "Network troubleshooting helper with memory",
		Long:  "CLI tool for network engineers — parses device logs, builds topology, tracks changes, and learns from troubleshooting history.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.LoadFrom(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if dbPath != "" {
				cfg.DBPath = dbPath
			}
			if cmd.Name() == "version" {
				return nil
			}
			db, err = store.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if db != nil {
				db.Close()
			}
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", config.DefaultConfigPath(), "config file path")
	root.PersistentFlags().StringVar(&dbPath, "db", "", "database file path (overrides config)")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newWatchCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("nethelper v0.1.0")
		},
	}
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
