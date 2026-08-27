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
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/llmconfig"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	cfgFile       string
	dataDir       string
	sourceDir     string
	lowMem        bool
	port          string
	mebIndex      string
	mebProfile    string
	writable      bool
	noMCP         bool
	ingestWorkers int
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

		// Configure log level from environment
		if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
			logger.SetLevelFromString(logLevel)
		}

		// Set defaults from environment if not provided via flags
		if port == "" {
			port = os.Getenv("PORT")
			if port == "" {
				port = "8080"
			}
		}
		if lowMemStr := os.Getenv("LOW_MEM"); lowMemStr != "" && !cmd.Flags().Changed("low-mem") {
			lowMem = strings.ToLower(lowMemStr) == "true"
		}
		if idx := os.Getenv("MEB_INDEX"); idx != "" {
			mebIndex = idx
		}
		if prof := os.Getenv("MEB_PROFILE"); prof != "" {
			mebProfile = prof
		}
		if w := os.Getenv("GCA_WRITABLE"); w != "" && !cmd.Flags().Changed("writable") {
			writable = strings.ToLower(w) == "true"
		}
		config.SetIngestWorkers(ingestWorkers)

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
	rootCmd.PersistentFlags().StringVarP(&port, "port", "p", "8080", "port for the server (or set PORT env var)")
	rootCmd.PersistentFlags().IntVar(&ingestWorkers, "ingest-workers", 0, "concurrent ingest workers (default: GCA_INGEST_WORKERS env or min(4, NumCPU))")
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
func createStore(readOnly bool, dataPath string, projectName string, sourcePath string) (*meb.MEBStore, error) {
	storePath := dataPath
	if projectName != "" {
		storePath = filepath.Join(dataPath, projectName)
	}
	cfg := store.DefaultConfig(storePath)
	cfg.SyncWrites = true

	if lowMem {
		cfg.Profile = "Safe-Serving"
	}

	cfg.BlockCacheSize = 128 << 20 // 128 MB
	cfg.IndexCacheSize = 128 << 20 // 128 MB

	if readOnly {
		cfg.ReadOnly = true
		fmt.Printf("Running in READ-ONLY mode. Data directory: %s\n", storePath)
	} else {
		fmt.Printf("Running in INGESTION mode.\nSource: %s\nData: %s\n", sourcePath, storePath)
	}

	// Set vector dimension to match embedding model output.
	// Must match the dimension of the configured embedding model or
	// all Vectors().Add() calls will fail with "invalid vector dimension".
	if dim := llmconfig.GetEmbeddingDim(""); dim > 0 {
		cfg.VectorFullDim = dim
	}

	return meb.NewMEBStore(cfg)
}

// getProjectName extracts the project name from the data directory
func getProjectName(dataPath string) string {
	return filepath.Base(dataPath)
}
