package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ephemeral"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/spf13/cobra"
)

// impactCmd represents the impact report command.
var impactCmd = &cobra.Command{
	Use:   "impact",
	Short: "Generate a PR blast-radius impact report",
	Long: `Generate a CI-friendly blast-radius report for a pull request diff.

Emits a JSON report and exits non-zero when new smells or hub hits exceed the
configured thresholds, making it usable as a PR gate.

  gca impact --project proj --base main --head HEAD
  gca impact --project proj --diff-file /tmp/pr.diff --fail-if-smells 2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectID, err := cmd.Flags().GetString("project")
		if err != nil {
			return fmt.Errorf("failed to get project flag: %w", err)
		}
		if projectID == "" {
			return fmt.Errorf("project ID is required (use --project flag)")
		}

		diff, err := resolveDiff(cmd)
		if err != nil {
			return err
		}

		ctx, cancel := createBaseContext()
		defer cancel()

		storeManager := manager.NewStoreManager(dataDir, getMemoryProfile(), false)
		storeManager.SetIndexType(mebIndex)
		storeManager.SetMebProfile(mebProfile)
		defer storeManager.CloseAll()

		impactSvc := service.NewImpactReportService(ephemeral.NewEphemeralStore(0), storeManager, nil)
		report, err := impactSvc.Generate(ctx, projectID, diff)
		if err != nil {
			return fmt.Errorf("failed to generate impact report: %w", err)
		}

		failIfSmells, _ := cmd.Flags().GetInt("fail-if-smells")
		failIfHubs, _ := cmd.Flags().GetInt("fail-if-hubs")
		if failIfSmells > 0 {
			report.NewSmellThreshold = failIfSmells
			report.Blocked = report.Blocked || report.SmellsNewCount() > failIfSmells
		}
		if failIfHubs > 0 && len(report.HubFilesHit) > failIfHubs {
			report.Blocked = true
		}

		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal report: %w", err)
		}
		fmt.Println(string(out))

		if report.Blocked {
			return fmt.Errorf("impact gate failed: %d new smells, %d hub files hit",
				report.SmellsNewCount(), len(report.HubFilesHit))
		}
		return nil
	},
}

// resolveDiff obtains the diff from --diff-file, --diff, or git.
func resolveDiff(cmd *cobra.Command) (string, error) {
	if diffFile, _ := cmd.Flags().GetString("diff-file"); diffFile != "" {
		data, err := os.ReadFile(diffFile)
		if err != nil {
			return "", fmt.Errorf("failed to read diff file: %w", err)
		}
		return string(data), nil
	}
	if inline, _ := cmd.Flags().GetString("diff"); inline != "" {
		return inline, nil
	}

	base, _ := cmd.Flags().GetString("base")
	head, _ := cmd.Flags().GetString("head")
	if base == "" {
		return "", fmt.Errorf("provide one of --diff, --diff-file, or --base")
	}
	args := []string{"diff", base}
	if head != "" {
		args = append(args, head)
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func init() {
	impactCmd.Flags().StringP("project", "j", "", "Project ID to analyze (required)")
	impactCmd.Flags().String("diff", "", "Inline unified diff")
	impactCmd.Flags().String("diff-file", "", "Path to a unified diff file")
	impactCmd.Flags().String("base", "", "Git base ref for diff resolution")
	impactCmd.Flags().String("head", "", "Git head ref (default: working tree)")
	impactCmd.Flags().Int("fail-if-smells", 0, "Exit non-zero when new smells exceed N")
	impactCmd.Flags().Int("fail-if-hubs", 0, "Exit non-zero when hub files hit exceed N")
	rootCmd.AddCommand(impactCmd)
}
