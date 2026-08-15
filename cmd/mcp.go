package cmd

import (
	"fmt"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/llmconfig"
	"github.com/duynguyendang/gca/pkg/mcp"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/spf13/cobra"
)

// mcpCmd represents the mcp command
var mcpCmd = &cobra.Command{
	Use:   "mcp [data-folder]",
	Short: "Start the MCP (Model Context Protocol) server",
	Long: `Start the MCP server for AI coding assistant integration.
Exposes the knowledge graph through the Model Context Protocol for tools like
Claude Desktop and other MCP clients.

Resources exposed:
  - gca://projects/{project}/summary: Graph statistics per project
  - gca://projects/{project}/smells: Detected code smells
  - gca://projects/{project}/health: Health overview
  - gca://projects/{project}/files/{path}: Source code content
  - gca://schema/conventions: Architectural schema docs

Tools exposed:
  - list_projects
  - search_nodes, get_outgoing_edges, get_incoming_edges, scan_facts
  - get_node_metadata, trace_impact_path, get_clusters, datalog_query
  - get_health_summary, list_smells
  - semantic_search, agent_execute (when an LLM key is configured)
  - okf_export, ingest_status
  - okf_ingest, ingest_incremental (only in --writable mode)

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

		// Use the same StoreManager as the REST server for multi-project support.
		mgr := manager.NewStoreManager(dataPath, getMemoryProfile(), true)
		mgr.SetVectorFullDim(llmconfig.GetEmbeddingDim(""))
		mgr.SetIndexType(mebIndex)
		mgr.SetMebProfile(mebProfile)
		defer mgr.CloseAll()

		// Optionally initialize the AI service for semantic search + agent tools.
		var aiSvc *ai.AIService
		if svc, err := ai.NewAIService(ctx, mgr); err == nil {
			aiSvc = svc
			defer svc.Close()
		} else {
			fmt.Printf("AI service not initialized (semantic search and agent tools disabled): %v\n", err)
		}

		// Start MCP server
		if err := mcp.RunStdio(ctx, mgr, aiSvc); err != nil {
			return fmt.Errorf("MCP server failed: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
