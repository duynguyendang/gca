package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
	"github.com/duynguyendang/gca/pkg/repl"

	"github.com/joho/godotenv"
)

func main() {
	// Define flags
	ingestMode := flag.Bool("ingest", false, "run in ingestion mode (requires source and data folder arguments)")

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
	} else {
		// Read-only mode
		// Optional: user can provide data folder as first arg
		if len(args) >= 1 {
			dataDir = args[0]
		}
	}

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

	if !readOnly {
		if err := ingest.Run(s, sourceDir); err != nil {
			log.Fatalf("Ingestion failed: %v", err)
		}

		// Force stats recalc only in write mode
		if _, err := s.RecalculateStats(); err != nil {
			log.Printf("Stats recalc error: %v", err)
		}
	} else {
		fmt.Println("Skipping stats recalculation and manual writes in ReadOnly mode.")
	}

	// Start Interactive Repl
	repl.Run(s, readOnly)
}
