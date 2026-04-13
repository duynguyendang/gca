package main

import (
	"context"
	"fmt"
	"log"
	
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	cfg := store.DefaultConfig("./data/gca-v2-fresh")
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	
	ctx := context.Background()
	
	// Test with names we KNOW exist from the output
	testNames := []string{"Execute", "main", "init", "getMemoryProfile", "createStore"}
	
	fmt.Println("=== Testing FindSubjectsByObject with known names ===")
	for _, name := range testNames {
		count := 0
		var subjects []string
		for subject := range s.FindSubjectsByObject(ctx, "has_name", name) {
			subjects = append(subjects, subject)
			count++
		}
		if count > 0 {
			fmt.Printf("  '%s' -> found %d: %v\n", name, count, subjects)
		} else {
			fmt.Printf("  '%s' -> NOT FOUND\n", name)
		}
	}
	
	// Test the resolver
	fmt.Println("\n=== Testing SymbolResolver ===")
	resolver := ingest.NewSymbolResolver(s)
	resolver.BuildImportMap(s)
	
	// Try resolving with names from the actual code
	testCases := []struct{
		callerFile string
		calleeName string
	}{
		{"gca-v2-fresh/cmd/root.go", "Execute"},
		{"gca-v2-fresh/cmd/root.go", "createStore"},
		{"gca-v2-fresh/pkg/server/server.go", "NewServer"},
	}
	
	for _, tc := range testCases {
		resolved := resolver.ResolveCallee(tc.callerFile, tc.calleeName)
		fmt.Printf("  ResolveCallee(%q, %q) = %q\n", tc.callerFile, tc.calleeName, resolved)
	}
	
	// Check what has_name facts exist for specific symbols
	fmt.Println("\n=== Checking specific symbols ===")
	symbols := []string{
		"gca-v2-fresh/cmd/root.go:Execute",
		"gca-v2-fresh/cmd/root.go:createStore",
		"gca-v2-fresh/pkg/server/server.go:NewServer",
	}
	
	for _, sym := range symbols {
		fmt.Printf("\nSymbol: %s\n", sym)
		// Check has_name
		for fact, err := range s.Scan(sym, "has_name", "") {
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			fmt.Printf("  has_name: %v\n", fact.Object)
		}
		// Check defines
		for fact, err := range s.Scan("", "defines", sym) {
			if err != nil {
				continue
			}
			fmt.Printf("  defined by: %s\n", fact.Subject)
		}
	}
}
