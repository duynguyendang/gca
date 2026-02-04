package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

func main() {
	// Setup temporary store
	dir, err := os.MkdirTemp("", "gca-debug-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cfg := store.DefaultConfig(dir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	// Add sample facts mirroring ingest
	// File fact
	s.AddFact(meb.Fact{Subject: "test/file.go", Predicate: "defines_symbol", Object: "test/file.go:Func", Graph: "default"})
	// Symbol facts
	s.AddFact(meb.Fact{Subject: "test/file.go:Func", Predicate: "type", Object: "function", Graph: "default"})
	s.AddFact(meb.Fact{Subject: "test/file.go:Func", Predicate: "defines", Object: "Func", Graph: "default"})

	// Test 1: Regex on File (Single Backslash in Query String)
	// Query: triples(File, _, _), regex(File, "\.go$")
	// Note: In Go source, we must escape backslash for string literal -> "\\.go$" -> passed as "\.go$" to parser?
	// User wrote "\\.go" in REPL. If REPL reads standard input:
	// Input: \.go -> Go string "\\.go" -> Regex "\.go" (Matches dot)
	// Input: \\.go -> Go string "\\\\.go" -> Regex "\\.go" (Matches slash dot)

	// Let's simulate what parser gets.
	// If user query is `triples(File, _, _), regex(File, "\.go$")`
	fmt.Println("--- Test 1: Single Escape in Datalog ---")
	q1 := `triples(File, _, _), regex(File, "\.go$")`
	res, err := s.Query(context.Background(), q1)
	if err != nil {
		fmt.Printf("Q1 Error: %v\n", err)
	} else {
		fmt.Printf("Q1 Results: %d\n", len(res))
		for _, r := range res {
			fmt.Printf("  %v\n", r)
		}
	}

	// Test 2: Double Escape in Datalog
	// User query: `triples(File, _, _), regex(File, "\\.go$")`
	fmt.Println("--- Test 2: Double Escape in Datalog ---")
	q2 := `triples(File, _, _), regex(File, "\\.go$")`
	res, err = s.Query(context.Background(), q2)
	if err != nil {
		fmt.Printf("Q2 Error: %v\n", err)
	} else {
		fmt.Printf("Q2 Results: %d\n", len(res))
	}

	// Test 3: Predicate Check
	// User query: `triples(Path, "defines", _)`
	fmt.Println("--- Test 3: defines Predicate ---")
	q3 := `triples(Path, "defines", _)`
	res, err = s.Query(context.Background(), q3)
	if err != nil {
		fmt.Printf("Q3 Error: %v\n", err)
	} else {
		fmt.Printf("Q3 Results: %d\n", len(res))
		for _, r := range res {
			fmt.Printf("  Path=%v\n", r["Path"])
		}
	}

	// Test 4: Correct Predicate
	// Query: `triples(Path, "defines_symbol", _)`
	fmt.Println("--- Test 4: defines_symbol Predicate ---")
	q4 := `triples(Path, "defines_symbol", _)`
	res, err = s.Query(context.Background(), q4)
	if err != nil {
		fmt.Printf("Q4 Error: %v\n", err)
	} else {
		fmt.Printf("Q4 Results: %d\n", len(res))
		for _, r := range res {
			fmt.Printf("  Path=%v\n", r["Path"])
		}
	}
}
