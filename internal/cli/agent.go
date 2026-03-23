package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/agent"
	"github.com/xavierli/nethelper/internal/llm"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "AI-powered network assistant",
	}
	cmd.AddCommand(newAgentChatCmd())
	return cmd
}

func newAgentChatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Start interactive agent session",
		Long:  "Start an interactive session with the nethelper AI agent. The agent can call nethelper tools to explore network data, generate change plans, and record troubleshooting experiences.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if llmRouter == nil {
				return fmt.Errorf("LLM not configured — add llm section to ~/.nethelper/config.yaml")
			}

			// Build embedder from config (nil if not configured)
			var embedder llm.Embedder
			if cfg != nil {
				embedder = llm.BuildEmbedder(cfg.Embedding)
			}

			// Build tool registry
			reg := agent.NewRegistry()
			agent.RegisterNethelperTools(reg, db, pipeline)

			// Create agent with optional vector memory support and JSONL session logging.
			sessionLogger := agent.NewSessionLogger(cfg.DataDir)
			ag := agent.New(llmRouter, reg, embedder, db, agent.AgentOptions{
				Logger:     sessionLogger,
				UserKey:    "repl",
				ContextCfg: cfg.Context,
			})

			// Run REPL
			return agent.RunREPL(context.Background(), ag)
		},
	}
}
