package cmd

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/repl"
	"github.com/spf13/cobra"
)

var _ context.Context // Explicitly reference context package type

// replCmd represents the repl command
var replCmd = &cobra.Command{
	Use:   "repl [data-folder]",
	Short: "Start interactive REPL for querying the knowledge graph",
	Long: `Start an interactive Read-Eval-Print Loop (REPL) for querying the knowledge graph.
Supports Datalog queries, natural language queries, and semantic search.

Commands available in REPL:
  - Datalog queries: triples(?A, "calls", ?B)
  - Natural language: Who calls the panic function?
  - Source view: show main.go:main
  - Schema: .schema
  - Exit: .exit

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

		// Configure REPL
		replCfg := repl.DefaultConfig()
		replCfg.GeminiAPIKey = geminiKey
		replCfg.ReadOnly = true

		if geminiModel != "" {
			replCfg.Model = geminiModel
		}

		// Start REPL
		repl.Run(ctx, replCfg, s)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
}
