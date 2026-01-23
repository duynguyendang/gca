package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
	"github.com/duynguyendang/gca/pkg/repl"
	"github.com/duynguyendang/gca/pkg/server"

	"context"

	"github.com/joho/godotenv"
)

func main() {
	// Define flags
	ingestMode := flag.Bool("ingest", false, "run in ingestion mode (requires source and data folder arguments)")
	serverMode := flag.Bool("server", false, "run REST API server")
	sourceFlag := flag.String("source", "", "path to source code (for source view)")
	lowMemMode := flag.Bool("low-mem", false, "optimize for low-memory environments (e.g., Cloud Run with 1GB RAM)")

	flag.Parse() // Parse flags early

	_ = godotenv.Load()

	// Defaults
	sourceDir := ""
	dataDir := "./data"
	readOnly := true

	args := flag.Args()

	if *ingestMode {
		readOnly = false
		if len(args) != 2 {
			fmt.Println("Error: --ingest requires exactly two arguments: <source_folder> <data_folder>")
			fmt.Println("Usage: main --ingest <source_folder> <data_folder>")
			os.Exit(1)
		}
		sourceDir = args[0]
		dataDir = args[1]
		// In ingestion mode, dataDir is the specific project store
	} else {
		// Read-only mode
		// Optional: user can provide data folder as first arg
		if len(args) >= 1 {
			dataDir = args[0]
		}

		// If explicit source flag is provided, use it
		if *sourceFlag != "" {
			sourceDir = *sourceFlag
		}
	}

	// Determine memory profile
	memProfile := manager.MemoryProfileDefault
	if *lowMemMode {
		memProfile = manager.MemoryProfileLow
		fmt.Println("Running in LOW MEMORY mode")
	}

	if *serverMode {
		fmt.Printf("Starting REST API Server. Project Root: %s\n", dataDir)

		// Initialize StoreManager
		mgr := manager.NewStoreManager(dataDir, memProfile)
		defer mgr.CloseAll()

		srv := server.NewServer(mgr, sourceDir)
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		addr := ":" + port
		if err := srv.Run(addr); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
		return
	}

	// === Single Store Mode (Ingest / Repl) ===

	// 1. MEB Store Initialization
	cfg := store.DefaultConfig(dataDir)
	cfg.SyncWrites = true
	// Optimize for dev/test environment
	cfg.Profile = "Safe-Serving"   // Reduces ValueLog size to 64MB (lowers VSZ)
	cfg.BlockCacheSize = 256 << 20 // 256 MB
	cfg.IndexCacheSize = 256 << 20 // 256 MB

	if readOnly {
		cfg.ReadOnly = true
		fmt.Printf("Running in READ-ONLY mode. Data directory: %s\n", dataDir)
	} else {
		fmt.Printf("Running in INGESTION mode.\nSource: %s\nData: %s\n", sourceDir, dataDir)
	}

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to create MEB store: %v", err)
	}
	defer s.Close()

	if *ingestMode {
		if err := ingest.Run(s, sourceDir); err != nil {
			s.Close()
			log.Fatalf("Ingestion failed: %v", err)
		}

		// Force stats recalc only in write mode
		if _, err := s.RecalculateStats(); err != nil {
			log.Printf("Stats recalc error: %v", err)
		}
	} else {
		// Start Interactive Repl
		replCfg := repl.DefaultConfig()
		replCfg.GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
		replCfg.ReadOnly = readOnly

		// Allow overriding model via env
		if model := os.Getenv("GEMINI_MODEL"); model != "" {
			replCfg.Model = model
		}

		repl.Run(context.Background(), replCfg, s)
	}
}
