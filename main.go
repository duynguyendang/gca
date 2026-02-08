package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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

	flag.Parse() // Parse flags early

	_ = godotenv.Load()

	// Check environment variable for LOW_MEM
	lowMemVal := os.Getenv("LOW_MEM")
	isLowMem := strings.ToLower(lowMemVal) == "true"

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
	if isLowMem {
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

		httpSrv := &http.Server{
			Addr:    addr,
			Handler: srv.Handler(),
		}

		// Initializing the server in a goroutine so that
		// it won't block the graceful shutdown handling below
		go func() {
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %s\n", err)
			}
		}()

		// Wait for interrupt signal to gracefully shutdown the server with
		// a timeout of 5 seconds.
		quit := make(chan os.Signal, 1)
		// kill (no param) default send syscall.SIGTERM
		// kill -2 is syscall.SIGINT
		// kill -9 is syscall.SIGKILL but can't be caught, so don't need to add it
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("Shutting down server...")

		// The context is used to inform the server it has 5 seconds to finish
		// the request it is currently handling
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Fatal("Server forced to shutdown: ", err)
		}

		log.Println("Server exiting")
		// defer mgr.CloseAll() will run here
		return
	}

	// === Single Store Mode (Ingest / Repl / MCP) ===

	// 1. MEB Store Initialization
	cfg := store.DefaultConfig(dataDir)
	cfg.SyncWrites = true
	// Optimize for resource awareness
	if isLowMem {
		cfg.Profile = "Cloud-Run-LowMem"
	}
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
