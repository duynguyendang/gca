package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug_ingest_has_name <source_dir>")
		os.Exit(1)
	}

	sourceDir := os.Args[1]

	// Create a test store
	cfg := &store.Config{
		InMemory:       true,
		BlockCacheSize: 1 << 20,
		IndexCacheSize: 1 << 20,
	}

	store, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Run ingestion
	projectName := "test-project"
	err = ingest.Run(store, projectName, sourceDir)
	if err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}

	fmt.Printf("\n=== Ingestion Complete ===\n")
	fmt.Printf("Total facts in store: %d\n", store.Count())

	// Query for has_name facts
	fmt.Println("\n=== has_name facts (first 20) ===")
	ctx := context.Background()
	hasNameCount := 0
	for fact, err := range store.Scan("", "has_name", "") {
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}
		if hasNameCount < 20 {
			fmt.Printf("  %s %s %v\n", fact.Subject, fact.Predicate, fact.Object)
		}
		hasNameCount++
	}
	fmt.Printf("Total has_name facts: %d\n", hasNameCount)

	// Query for defines facts
	fmt.Println("\n=== defines facts (first 10) ===")
	definesCount := 0
	for fact, err := range store.Scan("", "defines", "") {
		if err != nil {
			continue
		}
		if definesCount < 10 {
			fmt.Printf("  %s %s %v\n", fact.Subject, fact.Predicate, fact.Object)
		}
		definesCount++
	}
	fmt.Printf("Total defines facts: %d\n", definesCount)

	// Query for calls facts
	fmt.Println("\n=== calls facts (first 10) ===")
	callsCount := 0
	for fact, err := range store.Scan("", "calls", "") {
		if err != nil {
			continue
		}
		if callsCount < 10 {
			fmt.Printf("  %s %s %v\n", fact.Subject, fact.Predicate, fact.Object)
		}
		callsCount++
	}
	fmt.Printf("Total calls facts: %d\n", callsCount)

	// Test FindSubjectsByObject
	fmt.Println("\n=== Testing FindSubjectsByObject ===")
	testNames := []string{"Run", "ExtractSymbols", "NewServer", "handleQuery"}
	for _, name := range testNames {
		count := 0
		for subject := range store.FindSubjectsByObject(ctx, "has_name", name) {
			if count == 0 {
				fmt.Printf("  Looking for '%s': found %s\n", name, subject)
			}
			count++
		}
		if count > 0 {
			fmt.Printf("    Total matches for '%s': %d\n", name, count)
		}
	}

	// Check a specific file
	fmt.Println("\n=== Testing with actual Go files ===")
	filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".go" && !filepath.IsAbs(path) {
			relPath := filepath.Join(projectName, path)
			// Check if this file defines symbols
			definesCount := 0
			for fact, err := range store.Scan(relPath, "defines", "") {
				if err != nil {
					continue
				}
				definesCount++
				if definesCount <= 3 {
					symID, _ := fact.Object.(string)
					// Check if this symbol has a has_name fact
					for hnFact, hnErr := range store.Scan(symID, "has_name", "") {
						if hnErr != nil {
							continue
						}
						fmt.Printf("  %s -> has_name: %v\n", symID, hnFact.Object)
						break // just show first
					}
				}
			}
			if definesCount > 0 {
				fmt.Printf("File %s defines %d symbols\n", relPath, definesCount)
			}
		}
		return nil
	})
}
