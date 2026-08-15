package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/spf13/cobra"
)

// reportCmd represents the report command.
var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a project architecture report",
	Long: `Generate a regenerable markdown architecture report for an ingested project.
Assembles health, entry points, hubs, smells, clusters, call flows, and OKF
sections from the Analytical Store and source graph.

  gca report --project proj --out ARCHITECTURE.md
  gca report --project proj --out report.md --sections overview,smells
  gca report --project proj --out report.md --include-ai`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectID, err := cmd.Flags().GetString("project")
		if err != nil {
			return fmt.Errorf("failed to get project flag: %w", err)
		}
		if projectID == "" {
			return fmt.Errorf("project ID is required (use --project flag)")
		}
		outFile, err := cmd.Flags().GetString("out")
		if err != nil {
			return fmt.Errorf("failed to get out flag: %w", err)
		}
		if outFile == "" {
			return fmt.Errorf("output file is required (use --out flag)")
		}
		sectionsArg, _ := cmd.Flags().GetString("sections")
		includeAI, _ := cmd.Flags().GetBool("include-ai")

		var sections []string
		if sectionsArg != "" {
			for _, s := range strings.Split(sectionsArg, ",") {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					sections = append(sections, trimmed)
				}
			}
		}

		ctx, cancel := createBaseContext()
		defer cancel()

		storeManager := manager.NewStoreManager(dataDir, getMemoryProfile(), false)
		storeManager.SetIndexType(mebIndex)
		storeManager.SetMebProfile(mebProfile)
		defer storeManager.CloseAll()

		var narrator service.NarrativeService
		if includeAI {
			aiSvc, err := ai.NewAIService(ctx, storeManager)
			if err != nil {
				fmt.Printf("Warning: AI service unavailable, skipping narrative: %v\n", err)
			} else {
				narrator = aiSvc
			}
		}

		reportSvc := service.NewReportService(storeManager, narrator)
		md, err := reportSvc.GenerateMarkdown(ctx, service.ReportOptions{
			ProjectID: projectID,
			Sections:  sections,
			IncludeAI: includeAI && narrator != nil,
		})
		if err != nil {
			return fmt.Errorf("failed to generate report: %w", err)
		}

		if err := os.WriteFile(outFile, []byte(md), 0644); err != nil {
			return fmt.Errorf("failed to write report: %w", err)
		}
		fmt.Printf("Architecture report written to %s\n", outFile)
		return nil
	},
}

func init() {
	reportCmd.Flags().StringP("project", "j", "", "Project ID to report on (required)")
	reportCmd.Flags().StringP("out", "o", "", "Output markdown file (required)")
	reportCmd.Flags().String("sections", "", "Comma-separated sections: overview,entry_points,hubs,smells,clusters,call_flows,okf (default all)")
	reportCmd.Flags().Bool("include-ai", false, "Append an AI narrative summary")
	rootCmd.AddCommand(reportCmd)
}
