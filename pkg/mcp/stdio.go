package mcp

import (
	"context"
	"log/slog"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/mark3labs/mcp-go/server"
)

// RunStdio starts an MCP server on stdio for local MCP clients (Claude Desktop,
// Cursor, etc.). It blocks until the server exits.
func RunStdio(ctx context.Context, mgr *manager.StoreManager, aiSvc *ai.AIService) error {
	ms := New(Options{Manager: mgr, AIService: aiSvc})
	slog.Info("Starting MCP server on Stdio")
	return server.ServeStdio(ms)
}
