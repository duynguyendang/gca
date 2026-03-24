package main

import (
	"context"
	"fmt"
	"log"

	"strings"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	cfg := store.DefaultConfig("./data/gca")
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()

	// 1. Define Route Mappings (API Path -> Handler ID)
	routeMap := map[string]string{
		"/v1/graph/path": "gca-be/pkg/server/handlers.go:handleGraphPath",
	}

	// 2. Scan Frontend Services for fetch calls
	q := `triples(?s, "type", "function")`
	results, err := s.Query(ctx, q)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("[Debug] Scanning %d functions for API calls...\n", len(results))
	count := 0

	for _, res := range results {
		sID, ok := res["?s"].(string)
		if !ok || !strings.Contains(sID, "gca-fe/services") {
			continue
		}

		fmt.Printf("[Debug] Found Service Func: %s\n", sID)

		// Get Content
		contentBytes, err := s.GetContentByKey(string(sID))
		if err != nil {
			fmt.Printf("[Debug] GetDocument failed for %s: %v\n", sID, err)
			continue
		}

		content := string(contentBytes)
		// Debug print partial content
		if len(content) > 50 {
			fmt.Printf("[Debug] Content prefix: %s\n", content[:50])
		}

		for route, handlerID := range routeMap {
			if strings.Contains(content, route) {
				fmt.Printf("[Debug] MATCHED %s -> %s (via %s)\n", sID, handlerID, route)
				count++
			} else {
				// Check why not matching
				if strings.Contains(sID, "fetchGraphPath") {
					fmt.Printf("[Debug] fetchGraphPath content: %s\n", content)
					fmt.Printf("[Debug] Route checked: '%s'\n", route)
				}
			}
		}
	}
}
