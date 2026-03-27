package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/config"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/parser/cisco"
	"github.com/xavierli/nethelper/internal/parser/h3c"
	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/parser/juniper"
	"github.com/xavierli/nethelper/internal/store"
)

var (
	cfgFile       string
	dbPath        string
	cfg           *config.Config
	db            *store.DB
	pipeline      *parser.Pipeline
	llmRouter     *llm.Router
	registry      *parser.Registry
	fieldRegistry *parser.FieldRegistry
	version       = "dev" // 默认版本号，可通过 SetVersion 覆盖
)

// SetVersion 设置版本号，由 main 包在构建时注入
func SetVersion(v string) {
	version = v
}

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

			registry = parser.NewRegistry()
			registry.Register(huawei.New())
			registry.Register(cisco.New())
			registry.Register(h3c.New())
			registry.Register(juniper.New())
			pipeline = parser.NewPipelineWithCollector(db, registry, parser.NewCollector(db))
			fieldRegistry = parser.BuildFieldRegistry(registry)

			// Initialize LLM router from config
			llmRouter = llm.BuildFromConfig(cfg.LLM)

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
	root.AddCommand(newTraceCmd())
	root.AddCommand(newCheckCmd())
	root.AddCommand(newNoteCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newDiffCmd())
	root.AddCommand(newDiagnoseCmd())
	root.AddCommand(newExplainCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newScratchClearCmd())
	root.AddCommand(newPlanCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newAgentCmd())
	root.AddCommand(newChannelCmd())
	root.AddCommand(newHeartbeatCmd())
	root.AddCommand(newKnowledgeCmd())
	root.AddCommand(newRuleCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("nethelper %s\n", version)
		},
	}
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
