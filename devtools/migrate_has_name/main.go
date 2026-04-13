package main

import (
	"fmt"
	"log"
	"os"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: migrate_add_has_name <data_dir>")
		os.Exit(1)
	}

	dataDir := os.Args[1]

	// Open existing store
	cfg := store.DefaultConfig(dataDir)

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}
	defer s.Close()

	fmt.Printf("Opened store at: %s\n", dataDir)
	fmt.Printf("Total facts before migration: %d\n", s.Count())

	// Find all defines facts and create has_name facts from them
	// defines: file.go:FuncName defines file.go:FuncName
	// We need to extract the short name from the symbol ID
	
	hasNameFacts := make([]meb.Fact, 0)
	
	fmt.Println("\nScanning for symbols to add has_name facts...")
	
	// Scan all type facts to find symbols
	typeCount := 0
	for fact, err := range s.Scan("", config.PredicateType, "") {
		if err != nil {
			continue
		}
		
		symbolID := fact.Subject
		symType, _ := fact.Object.(string)
		
		// Extract short name from symbol ID
		// Symbol IDs are like "file.go:FuncName" or "file.go:Type.Method"
		shortName := extractShortName(symbolID)
		if shortName == "" {
			continue
		}
		
		hasNameFacts = append(hasNameFacts, meb.Fact{
			Subject:   symbolID,
			Predicate: config.PredicateHasName,
			Object:    shortName,
		})
		
		typeCount++
		if typeCount <= 5 {
			fmt.Printf("  %s (type=%s) -> has_name: %s\n", symbolID, symType, shortName)
		}
	}
	
	fmt.Printf("\nFound %d symbols to migrate\n", typeCount)
	fmt.Printf("Creating %d has_name facts...\n", len(hasNameFacts))
	
	if len(hasNameFacts) == 0 {
		fmt.Println("No facts to add, exiting")
		return
	}
	
	// Add facts in batches
	batchSize := 1000
	for i := 0; i < len(hasNameFacts); i += batchSize {
		end := i + batchSize
		if end > len(hasNameFacts) {
			end = len(hasNameFacts)
		}
		
		batch := hasNameFacts[i:end]
		if err := s.AddFactBatch(batch); err != nil {
			log.Printf("Error adding batch %d-%d: %v", i, end, err)
			continue
		}
		fmt.Printf("  Added batch %d-%d (%d facts)\n", i, end, len(batch))
	}
	
	fmt.Printf("\nMigration complete!\n")
	fmt.Printf("Total facts after migration: %d\n", s.Count())
	
	// Verify
	fmt.Println("\nVerifying has_name facts...")
	verifyCount := 0
	for _, err := range s.Scan("", config.PredicateHasName, "") {
		if err != nil {
			continue
		}
		verifyCount++
	}
	fmt.Printf("has_name facts count: %d\n", verifyCount)
}

// extractShortName extracts the short name from a symbol ID
// e.g., "file.go:FuncName" -> "FuncName"
// e.g., "file.go:Type.Method" -> "Method"
func extractShortName(symbolID string) string {
	// Find the last ':' or '.'
	lastColon := -1
	lastDot := -1
	
	for i := len(symbolID) - 1; i >= 0; i-- {
		if symbolID[i] == ':' {
			lastColon = i
			break
		}
		if symbolID[i] == '.' && lastDot == -1 {
			lastDot = i
		}
	}
	
	if lastColon != -1 {
		return symbolID[lastColon+1:]
	}
	
	if lastDot != -1 {
		// For methods like "file.go:Type.Method", get "Method"
		return symbolID[lastDot+1:]
	}
	
	return symbolID
}
