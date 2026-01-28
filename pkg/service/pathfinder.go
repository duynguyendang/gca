package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
)

// FindShortestPath implements BFS to find the shortest path between two symbols.
func (s *GraphService) FindShortestPath(ctx context.Context, projectID, startID, endID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	cleanStart := strings.Trim(startID, "\"")
	cleanEnd := strings.Trim(endID, "\"")

	fmt.Printf("[Pathfinder] BFS %s -> %s\n", cleanStart, cleanEnd)

	// Standard BFS
	queue := [][]string{{cleanStart}}
	visited := make(map[string]bool)
	visited[cleanStart] = true

	maxDepth := 50 // Increased depth
	var foundPath []string

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		current := path[len(path)-1]

		if len(visited) > 2000 {
			fmt.Println("[Pathfinder] BFS limit reached (2000 nodes)")
			break
		}

		fmt.Printf("Visiting %s (depth %d)\n", current, len(path))

		if current == cleanEnd {
			foundPath = path
			break
		}

		if len(path) >= maxDepth {
			continue
		}

		neighbors, err := s.getNeighbors(ctx, store, current)
		if err != nil {
			continue
		}
		fmt.Printf("Neighbors of %s: %d found\n", current, len(neighbors))

		for _, n := range neighbors {
			if !visited[n] {
				visited[n] = true
				newPath := make([]string, len(path))
				copy(newPath, path)
				newPath = append(newPath, n)
				queue = append(queue, newPath)
			}
		}
	}

	if len(foundPath) == 0 {
		fmt.Printf("[Pathfinder] Path not found.\n")
		return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
	}

	return s.buildGraphFromPath(ctx, store, foundPath)
}

func (s *GraphService) getNeighbors(ctx context.Context, store *meb.MEBStore, nodeID string) ([]string, error) {
	quoteID := fmt.Sprintf("\"%s\"", nodeID)
	// Query OUTBOUND calls/imports
	q := fmt.Sprintf(`triples(%s, ?p, ?o)`, quoteID)
	results, err := store.Query(ctx, q)
	if err != nil {
		return nil, err
	}

	neighbors := make([]string, 0)
	for _, res := range results {
		pred, _ := res["?p"].(string)
		obj, _ := res["?o"].(string)

		// Heuristic: Follow 'calls' strongly, 'imports' weakly, and 'defines' to drill down
		if pred == "calls" || pred == "imports" || pred == "defines" {
			// Debug Log
			fmt.Printf("Neighbor candidate: %s (pred: %s)\n", obj, pred)
			neighbors = append(neighbors, obj)

			// Attempt to resolve package.Symbol to canonical ID
			// If the ID doesn't look like a file path (no /), it's likely a semantic ID
			if !strings.Contains(obj, "/") {
				// Search for the symbol to find its canonical file-based ID
				matches, err := store.SearchSymbols(obj, 20, "")

				// Check if we found any canonical ID (containing /)
				foundCanonical := false
				if err == nil {
					for _, m := range matches {
						if strings.Contains(m, "/") {
							foundCanonical = true
							break
						}
					}
				}

				if (!foundCanonical) && strings.Contains(obj, ".") {
					// Fallback: Try searching for just the symbol name suffix
					parts := strings.Split(obj, ".")
					symbolOnly := parts[len(parts)-1]

					// Optimization: Skip common generic method names to avoid graph explosion
					switch symbolOnly {
					case "Add", "Get", "Set", "String", "Hash", "Equals", "Len", "Cap", "Close", "Errorf", "Fatal", "Fatalf", "Log", "Print", "Printf":
						continue
					}

					matchesFallback, errFallback := store.SearchSymbols(symbolOnly, 50, "")
					fmt.Printf("Resolved fallback %s -> %s (matches: %d, err: %v)\n", obj, symbolOnly, len(matchesFallback), errFallback)

					if errFallback == nil {
						for _, m := range matchesFallback {
							neighbors = append(neighbors, m)
						}
					}
				}

				// Add original matches too (if any)
				if err == nil {
					for _, m := range matches {
						neighbors = append(neighbors, m)
					}
				}
			}
		}
	}
	// Query INCOMING defines (Parent lookup)
	qIn := fmt.Sprintf(`triples(?s, "defines", %s)`, quoteID)
	resultsIn, errIn := store.Query(ctx, qIn)
	if errIn == nil {
		for _, res := range resultsIn {
			parent, _ := res["?s"].(string)
			fmt.Printf("Neighbor candidate (Parent): %s\n", parent)
			neighbors = append(neighbors, parent)
		}
	}

	// Optimization: Cap neighbors to prevent explosion
	if len(neighbors) > 50 {
		fmt.Printf("[Pathfinder] Capping neighbors for %s (%d -> 50)\n", nodeID, len(neighbors))
		neighbors = neighbors[:50]
	}

	return neighbors, nil
}

func (s *GraphService) buildGraphFromPath(ctx context.Context, store *meb.MEBStore, path []string) (*export.D3Graph, error) {
	graph := &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}

	// Nodes
	ids := make([]meb.DocumentID, len(path))
	for i, id := range path {
		ids[i] = meb.DocumentID(id)
	}
	hydrated, _ := store.HydrateShallow(ctx, ids, true)
	hMap := make(map[string]meb.HydratedSymbol)
	for _, h := range hydrated {
		hMap[string(h.ID)] = h
	}

	for _, id := range path {
		h, ok := hMap[id]
		name, kind := id, "unknown"
		if ok {
			// HydratedSymbol apparently doesn't have Name in this version?
			// Let's rely on ID parsing or check struct definition again.
			// Actually, let's just parse ID for safety to fix compilation.
			parts := strings.Split(string(h.ID), "/")
			name = parts[len(parts)-1]
			kind = h.Kind
		} else {
			parts := strings.Split(id, "/")
			name = parts[len(parts)-1]
		}
		graph.Nodes = append(graph.Nodes, export.D3Node{ID: id, Name: name, Kind: kind})
	}

	// Links
	for i := 0; i < len(path)-1; i++ {
		src, dst := path[i], path[i+1]
		graph.Links = append(graph.Links, export.D3Link{
			Source: src, Target: dst, Relation: "calls",
		})
	}
	return graph, nil
}
