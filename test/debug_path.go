package main

import (
	"fmt"
	"os"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

func main() {
	dataDir := "./data/gca"
	fmt.Printf("Opening %s directly (ReadOnly)...\n", dataDir)

	cfg := store.DefaultConfig(dataDir)
	cfg.ReadOnly = true
	cfg.BypassLockGuard = true

	s, err := meb.Open(dataDir, cfg)
	if err != nil {
		fmt.Printf("Failed to open: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	// Check for the nodes mentioned in the log
	nodes := []string{
		"gca/gca-fe/services/geminiService.ts:translateNLToDatalog",
		"gca/gca-be/pkg/repl/planner.go:ExecutionSession.NextStep",
		"datalog.Parse",
	}

	for _, node := range nodes {
		id, exists := s.LookupID(node)
		if exists {
			fmt.Printf("[FOUND] Node '%s' exists (ID: %d)\n", node, id)
			// Check outgoing calls
			query := fmt.Sprintf(`triples("%s", "calls", ?o)`, node)
			res, _ := s.Query(nil, query)
			fmt.Printf("  -> Calls %d nodes\n", len(res))
			for _, r := range res {
				fmt.Printf("     -> %s\n", r["?o"])
			}
		} else {
			fmt.Printf("[MISSING] Node '%s' NOT found\n", node)
		}
	}

	// Check datalog.Parse specifically by prefix
	prefix := "github.com/duynguyendang/gca/pkg/datalog:Parse"
	_, exists := s.LookupID(prefix)
	if exists {
		fmt.Printf("[FOUND] Full path '%s' exists\n", prefix)
	} else {
		fmt.Printf("[MISSING] Full path '%s' not found\n", prefix)
	}
}
