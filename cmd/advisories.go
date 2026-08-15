package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/duynguyendang/gca/pkg/compliance"
	"github.com/spf13/cobra"
)

// advisoriesCmd manages the offline vulnerability snapshot (F4).
var advisoriesCmd = &cobra.Command{
	Use:   "advisories",
	Short: "Manage the offline vulnerability advisory snapshot",
	Long: `Manage the offline OSV-style advisory snapshot used by the F4 compliance
scanner. GCA never queries a network at runtime; the snapshot is refreshed
ahead of time with this devtool and committed alongside the project.

  gca advisories update --out data/advisories/osv-snapshot.json
  gca advisories show  --path data/advisories/osv-snapshot.json`,
}

// advisoriesUpdateCmd fetches a fresh snapshot (interactive mode: edit before commit).
var advisoriesUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Fetch a fresh advisory snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := cmd.Flags().GetString("out")
		if err != nil {
			return fmt.Errorf("failed to get out flag: %w", err)
		}
		if out == "" {
			out = compliance.DefaultSnapshotPath
		}
		url, _ := cmd.Flags().GetString("url")

		if url == "" {
			fmt.Println("No --url provided; writing an empty snapshot. Set GCA_ADVISORIES_URL or --url to fetch real data.")
			empty := &compliance.Snapshot{Packages: map[string][]compliance.Advisory{}}
			if err := compliance.SaveSnapshot(out, empty); err != nil {
				return fmt.Errorf("failed to write snapshot: %w", err)
			}
			fmt.Printf("Empty snapshot written to %s\n", out)
			return nil
		}

		data, err := fetchWithTimeout(url, 60*time.Second)
		if err != nil {
			return fmt.Errorf("failed to fetch %s: %w", url, err)
		}
		if err := os.WriteFile(out, data, 0644); err != nil {
			return fmt.Errorf("failed to write snapshot: %w", err)
		}
		fmt.Printf("Snapshot written to %s (%d bytes)\n", out, len(data))
		return nil
	},
}

// advisoriesShowCmd prints the loaded snapshot summary.
var advisoriesShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current advisory snapshot summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := cmd.Flags().GetString("path")
		if path == "" {
			path = compliance.DefaultSnapshotPath
		}
		snap, err := compliance.LoadSnapshot(path)
		if err != nil {
			return err
		}
		out, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	},
}

func fetchWithTimeout(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func init() {
	advisoriesUpdateCmd.Flags().StringP("out", "o", "", "Output snapshot path (default data/advisories/osv-snapshot.json)")
	advisoriesUpdateCmd.Flags().String("url", os.Getenv("GCA_ADVISORIES_URL"), "URL to fetch advisory data from")
	advisoriesShowCmd.Flags().String("path", "", "Snapshot path (default data/advisories/osv-snapshot.json)")
	advisoriesCmd.AddCommand(advisoriesUpdateCmd)
	advisoriesCmd.AddCommand(advisoriesShowCmd)
	rootCmd.AddCommand(advisoriesCmd)
}
