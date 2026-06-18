package cmd

import (
	"fmt"
	"log"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/okf"
	"github.com/spf13/cobra"
)

var okfProject string
var okfOutDir string
var okfScope string

var okfCmd = &cobra.Command{
	Use:   "okf",
	Short: "OKF (Open Knowledge Format) ingest and export commands",
	Long: `Ingest OKF bundles into the knowledge graph, or export the code graph as an OKF bundle.
See docs/designs/okf-support.md for the full design.`,
}

var ingestOkfCmd = &cobra.Command{
	Use:   "ingest <bundle-dir> [--project <name>]",
	Short: "Ingest an OKF bundle into the knowledge graph",
	Long: `Parse and ingest an OKF v0.1 bundle (markdown files with YAML frontmatter)
into the Source Store as okf_concept entities with cross-references and bridge facts.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bundleDir := args[0]
		projectName := okfProject
		if projectName == "" {
			projectName = getProjectName(bundleDir)
		}

		ctx, cancel := createBaseContext()
		defer cancel()

		storeManager := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
		defer storeManager.CloseAll()

		fmt.Printf("Ingesting OKF bundle from %s into project %s...\n", bundleDir, projectName)

		report, err := okf.Ingest(ctx, storeManager, dataDir, okf.IngestOptions{
			ProjectID: projectName,
			BundleDir: bundleDir,
		})
		if err != nil {
			return fmt.Errorf("OKF ingest failed: %w", err)
		}

		fmt.Printf("OKF ingest complete:\n")
		fmt.Printf("  Concepts:    %d\n", report.Concepts)
		fmt.Printf("  Links:       %d\n", report.Links)
		fmt.Printf("  Bridges:     %d\n", report.Bridges)
		fmt.Printf("  Bridge miss: %d\n", report.BridgeMiss)
		fmt.Printf("  Conformant:  %v\n", report.Conformant)
		fmt.Printf("  Duration:    %s\n", report.Duration)
		if len(report.Errors) > 0 {
			fmt.Printf("  Errors (%d):\n", len(report.Errors))
			for _, e := range report.Errors {
				log.Printf("    - %s\n", e)
			}
		}

		return nil
	},
}

var exportOkfCmd = &cobra.Command{
	Use:   "export <project> --out <bundle-dir> [--scope file|package|cluster]",
	Short: "Export the code graph as an OKF bundle",
	Long: `Export the code graph as an OKF v0.1 bundle. One concept per file (default),
package, or cluster. Includes index.md per directory and log.md for change history.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		if okfOutDir == "" {
			return fmt.Errorf("--out is required")
		}
		scope := okf.ExportScope(okfScope)
		if scope == "" {
			scope = okf.ExportFile
		}

		ctx, cancel := createBaseContext()
		defer cancel()

		storeManager := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
		defer storeManager.CloseAll()

		fmt.Printf("Exporting project %s as OKF bundle to %s (scope: %s)...\n",
			projectName, okfOutDir, scope)

		report, err := okf.Export(ctx, storeManager, okf.ExportOptions{
			ProjectID:        projectName,
			OutDir:           okfOutDir,
			Scope:            scope,
			IncludeSmells:    true,
			IncludeCitations: true,
		})
		if err != nil {
			return fmt.Errorf("OKF export failed: %w", err)
		}

		fmt.Printf("OKF export complete:\n")
		fmt.Printf("  Concepts: %d\n", report.ConceptsWritten)
		fmt.Printf("  Files:    %d\n", report.FilesWritten)
		fmt.Printf("  Duration: %s\n", report.Duration)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(okfCmd)
	okfCmd.AddCommand(ingestOkfCmd)
	okfCmd.AddCommand(exportOkfCmd)

	// ingestOkfCmd flags
	ingestOkfCmd.Flags().StringVarP(&okfProject, "project", "p", "", "Project name (default: derived from bundle dir name)")

	// exportOkfCmd flags
	exportOkfCmd.Flags().StringVarP(&okfOutDir, "out", "o", "", "Output bundle directory (required)")
	exportOkfCmd.Flags().StringVarP(&okfScope, "scope", "s", "file", "Export scope: file, package, or cluster")
	_ = exportOkfCmd.MarkFlagRequired("out")

	_ = config.GenePoolPath // ensure config is linked
}
