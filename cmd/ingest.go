package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/registry"
	"github.com/spf13/cobra"
)

var _ context.Context // Explicitly reference context package type

var incremental bool
var noEmbed bool
var reEmbed bool

// ingestCmd represents the ingest command
var ingestCmd = &cobra.Command{
	Use:   "ingest <source-folder> [data-folder]",
	Short: "Ingest source code into the knowledge graph",
	Long: `Parse and ingest source code into the semantic knowledge graph.
Supports Go, Python, TypeScript, and JavaScript via tree-sitter.

Arguments:
  source-folder  Path to the source code directory to ingest
  data-folder    Path to store the ingested data (default: ./data)`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourcePath := args[0]
		dataPath := dataDir
		if len(args) > 1 {
			dataPath = args[1]
		}

		// Update global for use in createStore
		sourceDir = sourcePath
		dataDir = dataPath

		// Check env var for skip embeddings
		if os.Getenv("SKIP_EMBEDDINGS") == "true" {
			noEmbed = true
		}

		// Build ingest options
		opts := &ingest.IngestOptions{
			SkipEmbeddings: noEmbed,
			ReEmbed:        reEmbed,
		}

		// Create context with signal handling
		ctx, cancel := createBaseContext()
		defer cancel()

		// Run ingestion
		projectName := getProjectName(sourcePath)

		// Create store in write mode
		s, err := createStore(false, dataPath, projectName)
		if err != nil {
			return fmt.Errorf("failed to create MEB store: %w", err)
		}
		// Note: do NOT defer s.Close() here - we close it explicitly after ingestion

		// Run ingestion
		errChan := make(chan error, 1)

		go func() {
			state := ingest.NewIngestState()
			if incremental {
				errChan <- ingest.RunIncrementalWithOptions(s, projectName, sourcePath, state, opts)
			} else {
				errChan <- ingest.RunWithOptions(s, projectName, sourcePath, state, opts)
			}
		}()

		select {
		case <-ctx.Done():
			fmt.Println("Ingestion interrupted, closing store...")
			s.Close()
			return ctx.Err()
		case err := <-errChan:
			if err != nil {
				log.Printf("Ingestion failed: %v", err)
				s.Close()
				return err
			}

			// Recalculate stats
			if _, err := s.RecalculateStats(); err != nil {
				log.Printf("Stats recalc error: %v", err)
			}

			// Allow background goroutines to settle
			time.Sleep(1 * time.Second)
			fmt.Println("Ingestion completed successfully")

			// Close the ingest store before running template analysis
			s.Close()

			// Run template-based rule execution (smell detection, etc.)
			fmt.Println("Running template-based analysis...")
			storeManager := manager.NewStoreManager(dataPath, manager.MemoryProfileDefault, false)
			defer storeManager.CloseAll()

			templateStore := registry.NewTemplateStore(storeManager)
			if err := templateStore.LoadPolicyFiles(ctx, "policies"); err != nil {
				log.Printf("Warning: failed to load policy files: %v", err)
			}

			analyzer := ingest.NewAnalyzer(storeManager, templateStore)
			if err := analyzer.RunStaticAnalysis(ctx, projectName); err != nil {
				log.Printf("Warning: static analysis failed: %v", err)
			} else {
				fmt.Println("Template-based analysis completed")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(ingestCmd)
	ingestCmd.Flags().BoolVarP(&incremental, "incremental", "i", false, "Enable incremental ingestion (only process changed files)")
	ingestCmd.Flags().BoolVarP(&noEmbed, "no-embed", "e", false, "Skip embedding generation during ingestion")
	ingestCmd.Flags().BoolVar(&reEmbed, "re-embed", false, "Regenerate embeddings for all symbols from source code")
}
