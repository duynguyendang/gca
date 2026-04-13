package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: query_existing_db <data_dir>")
		os.Exit(1)
	}

	dataDir := os.Args[1]

	// Open existing store
	cfg := store.DefaultConfig(dataDir)

	store, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	fmt.Printf("Opened store at: %s\n", dataDir)
	fmt.Printf("Total facts: %d\n", store.Count())
	fmt.Printf("Topic ID: %d\n", store.TopicID())

	ctx := context.Background()

	// Count by predicate
	fmt.Println("\n=== Fact counts by predicate ===")
	predicates := []string{"defines", "calls", "has_name", "name", "in_package", "type"}
	for _, pred := range predicates {
		count := 0
		for _, err := range store.Scan("", pred, "") {
			if err != nil {
				continue
			}
			count++
		}
		fmt.Printf("  %s: %d\n", pred, count)
	}

	// Sample has_name facts
	fmt.Println("\n=== Sample has_name facts (first 20) ===")
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

	// Test FindSubjectsByObject with common names
	fmt.Println("\n=== Testing FindSubjectsByObject ===")
	testNames := []string{"Run", "ExtractSymbols", "NewServer", "handleQuery", "Server"}
	for _, name := range testNames {
		count := 0
		var firstSubject string
		for subject := range store.FindSubjectsByObject(ctx, "has_name", name) {
			if count == 0 {
				firstSubject = subject
			}
			count++
		}
		if count > 0 {
			fmt.Printf("  '%s' -> found %d match(es), first: %s\n", name, count, firstSubject)
		} else {
			fmt.Printf("  '%s' -> NOT FOUND\n", name)
		}
	}

	// Check if resolver would work
	fmt.Println("\n=== Testing resolver pattern ===")
	resolver := ingest.NewSymbolResolver(store)
	
	// Build import map
	resolver.BuildImportMap(store)

	// Try to resolve some callees
	testCallees := []struct{
		callerFile string
		calleeName string
	}{
		{"gca-v2/pkg/server/server.go", "NewServer"},
		{"gca-v2/pkg/server/server.go", "Run"},
		{"gca-v2/pkg/server/handlers.go", "handleQuery"},
	}

	for _, tc := range testCallees {
		resolved := resolver.ResolveCallee(tc.callerFile, tc.calleeName)
		fmt.Printf("  ResolveCallee(%q, %q) = %q\n", tc.callerFile, tc.calleeName, resolved)
	}
}
