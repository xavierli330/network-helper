// internal/cli/mcp.go
package cli

import (
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	netmcp "github.com/xavierli/nethelper/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP (Model Context Protocol) server",
	}
	cmd.AddCommand(newMCPServeCmd())
	return cmd
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server (stdio transport)",
		Long: `Start a Model Context Protocol server that exposes nethelper tools over stdio.

Configure in Claude Code settings:
  "mcpServers": {"nethelper": {"command": "nethelper", "args": ["mcp", "serve"]}}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := netmcp.NewServer(db, pipeline, llmRouter)
			return mcpserver.ServeStdio(s)
		},
	}
}
