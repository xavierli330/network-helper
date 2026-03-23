// internal/mcp/server.go
package mcp

import (
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/store"
)

// NewServer creates an MCP server with all nethelper tools registered.
func NewServer(db *store.DB, pipeline *parser.Pipeline, llmRouter *llm.Router) *mcpserver.MCPServer {
	s := mcpserver.NewMCPServer(
		"nethelper",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	// Tools will be registered by Tasks 2-6
	// registerShowTools(s, db)
	// registerAnalysisTools(s, db)
	// registerPlanTools(s, db)
	// registerSearchTools(s, db)
	// registerWriteTools(s, db, pipeline, llmRouter)

	return s
}
