package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
)

// FindShortestPath implements BFS to find the shortest path between two symbols.
// It considers 'calls', 'imports', and 'defines' (parent-child) relationships as edges.
func (s *GraphService) FindShortestPath(ctx context.Context, projectID, startID, endID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// Queue for BFS: holds path so far [startID, node2, ..., currentID]
	queue := [][]string{{startID}}
	visited := make(map[string]bool)
	visited[startID] = true

	// Clean IDs
	cleanStart := strings.Trim(startID, "\"")
	cleanEnd := strings.Trim(endID, "\"")

	// Limit depth to avoid infinite loops or timeout
	maxDepth := 10 // Arbitrary logical limit

	var foundPath []string

	fmt.Printf("[Pathfinder] Starting BFS from %s to %s\n", cleanStart, cleanEnd)

	// BFS Loop
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		current := path[len(path)-1]

		if current == cleanEnd {
			foundPath = path
			break
		}

		if len(path) >= maxDepth {
			continue
		}

		// Get Neighbors
		// We consider:
		// 1. Outbound calls: triples(current, "calls", ?n)
		// 2. Outbound imports: triples(current, "imports", ?n)
		// 3. Definitions (Down): triples(current, "defines", ?n) -- maybe?
		// 4. Inbound calls (Reverse): triples(?n, "calls", current) -- maybe "how does A get reached?"
		// For "Interaction Discovery", usually flow follows calls.
		// Use-case: "How does A talk to B?" implies A -> ... -> B.
		// So we follow Outbound edges.

		// Optimization: We can query multiple predicates at once if we had Union,
		// but simple iteration is fine.

		// TODO: Add caching if performance is slow.

		neighbors, err := s.getNeighbors(ctx, store, current)
		if err != nil {
			// Log error but continue?
			fmt.Printf("[Pathfinder] Error getting neighbors for %s: %v\n", current, err)
			continue
		}

		for _, neighbor := range neighbors {
			if !visited[neighbor] {
				visited[neighbor] = true
				newPath := make([]string, len(path))
				copy(newPath, path)
				newPath = append(newPath, neighbor)
				queue = append(queue, newPath)
			}
		}
	}

	if len(foundPath) == 0 {
		return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
	}

	// Construct D3Graph from the found path
	return s.buildGraphFromPath(ctx, store, foundPath)
}

// getNeighbors finds all outbound connected nodes.
func (s *GraphService) getNeighbors(ctx context.Context, store *meb.MEBStore, nodeID string) ([]string, error) {
	// Query: triples("nodeID", ?p, ?o)
	// We want ?o where ?p in ["calls", "imports"]
	// Datalog doesn't support "IN" clause easily in one go basically unless we iterate predicates.

	quoteID := fmt.Sprintf("\"%s\"", nodeID)
	q := fmt.Sprintf(`triples(%s, ?p, ?o)`, quoteID)

	results, err := store.Query(ctx, q)
	if err != nil {
		return nil, err
	}

	neighbors := make([]string, 0, len(results))
	for _, res := range results {
		pred, okP := res["?p"].(string)
		obj, okO := res["?o"].(string)

		if !okP || !okO {
			continue
		}

		// Filter predicates
		if pred == "calls" || pred == "imports" {
			neighbors = append(neighbors, obj)
		}
	}
	return neighbors, nil
}

// buildGraphFromPath converts a list of IDs into a full D3Graph with nodes and links.
func (s *GraphService) buildGraphFromPath(ctx context.Context, store *meb.MEBStore, path []string) (*export.D3Graph, error) {
	graph := &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}

	// 1. Create Nodes
	// We need to hydrate them to get names, kinds, etc.
	ids := make([]meb.DocumentID, len(path))
	for i, id := range path {
		ids[i] = meb.DocumentID(id)
	}

	hydrated, err := store.HydrateShallow(ctx, ids, true)
	if err != nil {
		return nil, err
	}

	hMap := make(map[string]meb.HydratedSymbol)
	for _, h := range hydrated {
		hMap[string(h.ID)] = h
	}

	for _, id := range path {
		h, ok := hMap[id]
		name := id
		kind := "unknown"
		if ok {
			parts := strings.Split(string(h.ID), "/")
			name = parts[len(parts)-1]
			kind = h.Kind
		} else {
			// Fallback name
			parts := strings.Split(id, "/")
			name = parts[len(parts)-1]
		}

		graph.Nodes = append(graph.Nodes, export.D3Node{
			ID:   id,
			Name: name,
			Kind: kind,
		})
	}

	// 2. Create Links
	// For path A -> B -> C, we add A->B and B->C
	// We need to know the *exact* predicate that connected them.
	// Re-query or infer? Re-query is safest to get correct "calls" vs "imports".

	for i := 0; i < len(path)-1; i++ {
		src := path[i]
		dst := path[i+1]

		// Find predicate
		q := fmt.Sprintf(`triples("%s", ?p, "%s")`, src, dst)
		results, err := store.Query(ctx, q)
		rel := "connected_to" // fallback
		if err == nil && len(results) > 0 {
			if r, ok := results[0]["?p"].(string); ok {
				rel = r
			}
		}

		graph.Links = append(graph.Links, export.D3Link{
			Source:   src,
			Target:   dst,
			Relation: rel,
		})
	}

	return graph, nil
}
