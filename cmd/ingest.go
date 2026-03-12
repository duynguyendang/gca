package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/spf13/cobra"
)

var _ context.Context // Explicitly reference context package type

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

		// Create context with signal handling
		ctx, cancel := createBaseContext()
		defer cancel()

		// Create store in write mode
		s, err := createStore(false, dataPath)
		if err != nil {
			return fmt.Errorf("failed to create MEB store: %w", err)
		}
		defer s.Close()

		// Run ingestion
		projectName := getProjectName(dataPath)
		errChan := make(chan error, 1)

		go func() {
			errChan <- ingest.Run(s, projectName, sourcePath)
		}()

		select {
		case <-ctx.Done():
			fmt.Println("Ingestion interrupted, closing store...")
			return ctx.Err()
		case err := <-errChan:
			if err != nil {
				log.Printf("Ingestion failed: %v", err)
				return err
			}

			// Recalculate stats
			if _, err := s.RecalculateStats(); err != nil {
				log.Printf("Stats recalc error: %v", err)
			}

			// Allow background goroutines to settle
			time.Sleep(1 * time.Second)
			fmt.Println("Ingestion completed successfully")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(ingestCmd)
}
