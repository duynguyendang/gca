package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/duynguyendang/gca/pkg/mcp"
	"github.com/spf13/cobra"
)

var _ context.Context // Explicitly reference context package type

// mcpCmd represents the mcp command
var mcpCmd = &cobra.Command{
	Use:   "mcp [data-folder]",
	Short: "Start the MCP (Model Context Protocol) server",
	Long: `Start the MCP server for AI coding assistant integration.
Exposes the knowledge graph through the Model Context Protocol for tools like
Claude Desktop and other MCP clients.

Resources exposed:
  - gca://graph/summary: Graph statistics
  - gca://files/{path}: Source code content
  - gca://schema/conventions: Architectural schema docs

Tools exposed:
  - search_nodes: Search for symbols/files
  - get_outgoing_edges: Get dependencies
  - get_incoming_edges: Get consumers
  - get_clusters: Detect logical communities (Leiden)
  - trace_impact_path: Trace weighted paths between nodes

Arguments:
  data-folder  Path to the data directory (default: ./data)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dataPath := dataDir
		if len(args) > 0 {
			dataPath = args[0]
		}

		// Create context with signal handling
		ctx, cancel := createBaseContext()
		defer cancel()

		// Create store in read-only mode
		s, err := createStore(true, dataPath)
		if err != nil {
			return fmt.Errorf("failed to create MEB store: %w", err)
		}
		defer s.Close()

		// Start MCP server
		if err := mcp.Run(ctx, s); err != nil {
			log.Fatalf("MCP server failed: %v", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
