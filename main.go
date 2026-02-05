package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/mcp"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"

	"github.com/duynguyendang/gca/pkg/repl"
	"github.com/duynguyendang/gca/pkg/server"

	"github.com/joho/godotenv"
)

func main() {
	// Define flags
	mcpMode := flag.Bool("mcp", false, "run MCP server (Standard Input/Output)")
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
		mgr := manager.NewStoreManager(dataDir, memProfile, true)
		defer mgr.CloseAll()

		apiKey := os.Getenv("GEMINI_API_KEY")

		srv := server.NewServer(mgr, sourceDir, apiKey)
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

	// === Single Store Mode (Ingest / Repl / MCP) ===

	// 1. MEB Store Initialization
	cfg := store.DefaultConfig(dataDir)
	cfg.SyncWrites = true
	// Optimize for resource awareness
	cfg.Profile = "Cloud-Run-LowMem"
	cfg.BlockCacheSize = 128 << 20 // 128 MB
	cfg.IndexCacheSize = 128 << 20 // 128 MB

	if readOnly {
		cfg.ReadOnly = true
		fmt.Printf("Running in READ-ONLY mode. Data directory: %s\n", dataDir)
	} else {
		fmt.Printf("Running in INGESTION mode.\nSource: %s\nData: %s\n", sourceDir, dataDir)
	}

	// Create context that cancels on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nReceived signal, shutting down gracefully...")
		cancel()
	}()

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to create MEB store: %v", err)
	}
	// We handle Close manually on exit or signal

	// Ensure Close is called eventually
	defer s.Close()

	if *ingestMode {
		// Ingest backend (Go) files from source directory
		projectName := filepath.Base(dataDir)
		// Run ingestion in a goroutine so we can wait closer
		errChan := make(chan error, 1)
		go func() {
			errChan <- ingest.Run(s, projectName, sourceDir)
		}()

		select {
		case <-ctx.Done():
			fmt.Println("Ingestion interrupted, closing store...")
			// Store close deferred
		case err := <-errChan:
			if err != nil {
				log.Printf("Ingestion failed: %v", err)
			}
			// Recalc stats
			if _, err := s.RecalculateStats(); err != nil {
				log.Printf("Stats recalc error: %v", err)
			}
		}
	} else if *mcpMode {
		// Start MCP Server
		if err := mcp.Run(ctx, s); err != nil {
			log.Fatalf("MCP server failed: %v", err)
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

		repl.Run(ctx, replCfg, s)
	}
}
