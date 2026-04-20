package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/registry"
	"github.com/spf13/cobra"
)

var _ context.Context // Explicitly reference context package type

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run analysis on the knowledge graph",
	Long: `Run analysis commands on the ingested code knowledge graph.
Supports source analysis (centrality, smell detection) and template queries.`,
}

// sourceAnalyzeCmd represents the source analysis subcommand
var sourceAnalyzeCmd = &cobra.Command{
	Use:   "source",
	Short: "Run static analysis on ingested source",
	Long: `Run static analysis pipeline on ingested source code.
Computes centrality, entry points, and detects architectural smells.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectID, err := cmd.Flags().GetString("project")
		if err != nil {
			return fmt.Errorf("failed to get project flag: %w", err)
		}
		if projectID == "" {
			return fmt.Errorf("project ID is required (use --project flag)")
		}

		fmt.Printf("Running static analysis for project: %s\n", projectID)
		fmt.Printf("Data directory: %s\n", dataDir)

		// Create context with signal handling
		ctx, cancel := createBaseContext()
		defer cancel()

		// Initialize store manager
		storeManager := manager.NewStoreManager(dataDir, getMemoryProfile(), false)

		// Initialize template store
		templateStore := registry.NewTemplateStore(storeManager)

		// Load templates from policy files
		if err := templateStore.LoadPolicyFiles(ctx, "policies"); err != nil {
			log.Printf("Warning: failed to load policy files: %v", err)
		}

		// Create analyzer
		analyzer := ingest.NewAnalyzer(storeManager, templateStore)

		// Run static analysis synchronously
		start := time.Now()
		if err := analyzer.RunStaticAnalysis(ctx, projectID); err != nil {
			log.Printf("Static analysis failed: %v", err)
			return fmt.Errorf("static analysis failed: %w", err)
		}
		elapsed := time.Since(start)

		fmt.Printf("Static analysis completed in %v\n", elapsed)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.AddCommand(sourceAnalyzeCmd)

	sourceAnalyzeCmd.Flags().StringP("project", "j", "", "Project ID to analyze (required)")
	sourceAnalyzeCmd.MarkFlagRequired("project")
}
