package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/spf13/cobra"
)

var testgenCmd = &cobra.Command{
	Use:   "testgen <source-folder> [data-folder]",
	Short: "List API handlers in a project",
	Long: `Discover and list all API handlers in the project by scanning for
symbols with the 'api_handler' role.

Arguments:
  source-folder  Path to the source code directory (used to derive project name)
  data-folder    Path to the data directory (default: ./data)`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourcePath := args[0]
		dataPath := dataDir
		if len(args) > 1 {
			dataPath = args[1]
		}

		ctx, cancel := createBaseContext()
		defer cancel()

		projectName := getProjectName(sourcePath)
		if projectName == "" {
			projectName = filepath.Base(sourcePath)
		}

		storeManager := manager.NewStoreManager(dataPath, manager.MemoryProfileDefault, true)
		projStore, err := storeManager.GetStore(projectName)
		if err != nil {
			return fmt.Errorf("project not found: %w", err)
		}

		var handlers []string
		for fact := range projStore.ScanContext(ctx, "", "has_role", "api_handler") {
			handlers = append(handlers, fact.Subject)
		}

		if len(handlers) == 0 {
			fmt.Println("No API handlers found")
			return nil
		}

		fmt.Printf("Project: %s\n", projectName)
		fmt.Printf("Found %d API handlers:\n\n", len(handlers))

		for i, h := range handlers {
			fmt.Printf("%d. %s\n", i+1, h)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(testgenCmd)
}