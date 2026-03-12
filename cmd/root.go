package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	cfgFile     string
	dataDir     string
	sourceDir   string
	lowMem      bool
	geminiKey   string
	geminiModel string
	port        string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gca",
	Short: "GCA - Neuro-Symbolic Code Analysis Platform",
	Long: `GCA (Gem Code Analysis) is a next-generation code analysis tool that ingests
source code into a semantic knowledge graph, enabling powerful queries through
Datalog, natural language, and semantic search.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load .env file if exists
		_ = godotenv.Load()

		// Set defaults from environment if not provided via flags
		if geminiKey == "" {
			geminiKey = os.Getenv("GEMINI_API_KEY")
		}
		if geminiModel == "" {
			geminiModel = os.Getenv("GEMINI_MODEL")
		}
		if port == "" {
			port = os.Getenv("PORT")
			if port == "" {
				port = "8080"
			}
		}
		if lowMemStr := os.Getenv("LOW_MEM"); lowMemStr != "" && !cmd.Flags().Changed("low-mem") {
			lowMem = strings.ToLower(lowMemStr) == "true"
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gca.yaml)")
	rootCmd.PersistentFlags().StringVarP(&dataDir, "data", "d", "./data", "data directory for the store")
	rootCmd.PersistentFlags().StringVarP(&sourceDir, "source", "s", "", "path to source code (for source view)")
	rootCmd.PersistentFlags().BoolVarP(&lowMem, "low-mem", "l", false, "enable low memory mode")
	rootCmd.PersistentFlags().StringVar(&geminiKey, "api-key", "", "Gemini API key (or set GEMINI_API_KEY env var)")
	rootCmd.PersistentFlags().StringVar(&geminiModel, "model", "", "Gemini model to use (or set GEMINI_MODEL env var)")
	rootCmd.PersistentFlags().StringVarP(&port, "port", "p", "8080", "port for the server (or set PORT env var)")
}

// getMemoryProfile returns the appropriate memory profile based on flags
func getMemoryProfile() manager.MemoryProfile {
	if lowMem {
		return manager.MemoryProfileLow
	}
	return manager.MemoryProfileDefault
}

// createBaseContext creates a context with signal handling
func createBaseContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	// Handle signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nReceived signal, shutting down gracefully...")
		cancel()
	}()

	return ctx, cancel
}

// createStore creates a new MEB store with appropriate configuration
func createStore(readOnly bool, dataPath string) (*meb.MEBStore, error) {
	cfg := store.DefaultConfig(dataPath)
	cfg.SyncWrites = true

	if lowMem {
		cfg.Profile = "Safe-Serving"
	}

	cfg.BlockCacheSize = 128 << 20 // 128 MB
	cfg.IndexCacheSize = 128 << 20 // 128 MB

	if readOnly {
		cfg.ReadOnly = true
		fmt.Printf("Running in READ-ONLY mode. Data directory: %s\n", dataPath)
	} else {
		fmt.Printf("Running in INGESTION mode.\nSource: %s\nData: %s\n", sourceDir, dataDir)
	}

	return meb.NewMEBStore(cfg)
}

// getProjectName extracts the project name from the data directory
func getProjectName(dataPath string) string {
	return filepath.Base(dataPath)
}
