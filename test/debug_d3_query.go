package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/datalog"
	"github.com/duynguyendang/gca/pkg/export"
)

func main() {
	// 1. Setup Manager
	mgr := manager.NewStoreManager("data") // Assume run from root
	store, err := mgr.GetStore("mangle")
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// 2. Query
	query := `triples(?s, "calls", ?t), triples(?f, "defines", ?s), regex(?f, "^analysis/.*")`
	fmt.Printf("Executing Query: %s\n", query)

	results, err := store.Query(context.Background(), query)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	fmt.Printf("Rows returned: %d\n", len(results))
	if len(results) > 0 {
		fmt.Printf("First Row: %v\n", results[0])
	}

	// 3. Debug Transformer Selection
	atoms, err := datalog.Parse(query)
	if err != nil {
		log.Fatalf("Parse failed: %v", err)
	}

	var triplesAtom *datalog.Atom
	for i, atom := range atoms {
		fmt.Printf("Atom %d: Predicate=%s Args=%v\n", i, atom.Predicate, atom.Args)
		if atom.Predicate == "triples" && triplesAtom == nil {
			triplesAtom = &atom
			fmt.Printf("-> Selected for Transform: Atom %d\n", i)
		}
	}

	// 4. Run Transform manually-ish
	if len(results) > 0 {
		graph, err := export.ExportD3(context.Background(), store, query, results)
		if err != nil {
			log.Fatalf("Export failed: %v", err)
		}

		fmt.Printf("Graph Nodes: %d\n", len(graph.Nodes))
		fmt.Printf("Graph Links: %d\n", len(graph.Links))

		if len(graph.Links) > 0 {
			fmt.Printf("First Link: %+v\n", graph.Links[0])
		} else {
			fmt.Println("WARNING: Links are empty!")
			// Analyze specific row for the selected atom
			if triplesAtom != nil {
				resolve := func(arg string, row map[string]any) string {
					if strings.HasPrefix(arg, "?") || (len(arg) > 0 && arg[0] >= 'A' && arg[0] <= 'Z') {
						if val, ok := row[arg]; ok {
							return fmt.Sprintf("%v", val)
						}
						return ""
					}
					return strings.Trim(arg, "\"'")
				}

				row := results[0]
				sArg, pArg, oArg := triplesAtom.Args[0], triplesAtom.Args[1], triplesAtom.Args[2]
				sVal := resolve(sArg, row)
				pVal := resolve(pArg, row)
				oVal := resolve(oArg, row)
				fmt.Printf("Debug Resolve Row 0: S=%s P=%s O=%s\n", sVal, pVal, oVal)
			}
		}

		// Output JSON for inspection
		data, _ := json.MarshalIndent(graph, "", "  ")
		fmt.Println(string(data[:500])) // First 500 chars
	}
}
