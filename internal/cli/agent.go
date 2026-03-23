package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/agent"
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

			// Build tool registry
			reg := agent.NewRegistry()
			agent.RegisterNethelperTools(reg, db, pipeline)

			// Create agent
			ag := agent.New(llmRouter, reg)

			// Run REPL
			return agent.RunREPL(context.Background(), ag)
		},
	}
}
