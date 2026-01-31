package main

import (
	"context"
	"fmt"
	"os"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

func main() {
	dataDir := "./data/mangle-v2"
	fmt.Printf("Opening %s directly (RW)...\n", dataDir)

	cfg := store.DefaultConfig(dataDir)
	// Open in RW mode to allow recovery/truncate
	cfg.ReadOnly = false
	cfg.BypassLockGuard = true

	s, err := meb.Open(dataDir, cfg)
	if err != nil {
		fmt.Printf("Failed to open: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	// Query
	// triples("mangle-v2/analysis/rectify.go", ?p, ?o)
	fmt.Println("Querying for mangle-v2/analysis/rectify.go...")
	results, err := s.Query(context.Background(), `triples("mangle-v2/analysis/rectify.go", ?p, ?o)`)
	if err != nil {
		fmt.Printf("Query error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d triples\n", len(results))
	for i, r := range results {
		if i > 10 {
			break
		}
		fmt.Printf("%v %v %v\n", r["?s"], r["?p"], r["?o"])
	}
}
