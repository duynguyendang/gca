package mcp

import (
	"net/http"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/mark3labs/mcp-go/server"
)

// NewHTTPServer builds a Streamable HTTP MCP server that can be mounted on an
// existing HTTP router via its ServeHTTP method. It enables remote MCP clients.
func NewHTTPServer(mgr *manager.StoreManager, aiSvc *ai.AIService) http.Handler {
	ms := New(Options{Manager: mgr, AIService: aiSvc})
	return server.NewStreamableHTTPServer(ms)
}
