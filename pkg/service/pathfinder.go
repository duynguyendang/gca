package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
)

type Direction int

const (
	DirectionForward Direction = iota
	DirectionBackward
)

// FindShortestPath implements Bidirectional BFS to find the shortest path between two symbols.
func (s *GraphService) FindShortestPath(ctx context.Context, projectID, startID, endID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	cleanStart := strings.Trim(startID, "\"")
	cleanEnd := strings.Trim(endID, "\"")

	if cleanStart == cleanEnd {
		return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
	}

	fmt.Printf("[Pathfinder] Bidirectional BFS %s <-> %s\n", cleanStart, cleanEnd)

	// Queues
	qStart := []string{cleanStart}
	qEnd := []string{cleanEnd}

	// Visited Maps (Node -> Parent)
	// For Start: Parent is the node closer to Start
	// For End: Parent is the node closer to End
	visitedStart := make(map[string]string)
	visitedEnd := make(map[string]string)

	visitedStart[cleanStart] = ""
	visitedEnd[cleanEnd] = ""

	maxDepth := 10 // Bidirectional search goes deep fast, 10 is plenty (path len 20)
	depth := 0

	for len(qStart) > 0 && len(qEnd) > 0 {
		if depth > maxDepth {
			break
		}
		depth++

		// Always expand the smaller frontier to balance search
		if len(qStart) <= len(qEnd) {
			intersection := expandFrontier(ctx, s, store, &qStart, visitedStart, visitedEnd, DirectionForward)
			if intersection != "" {
				return s.constructBidirectionalPath(ctx, store, intersection, visitedStart, visitedEnd)
			}
		} else {
			intersection := expandFrontier(ctx, s, store, &qEnd, visitedEnd, visitedStart, DirectionBackward)
			if intersection != "" {
				return s.constructBidirectionalPath(ctx, store, intersection, visitedStart, visitedEnd)
			}
		}
	}

	fmt.Printf("[Pathfinder] Path not found.\n")
	return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
}

func expandFrontier(ctx context.Context, s *GraphService, store *meb.MEBStore, queue *[]string, visitedMy map[string]string, visitedOther map[string]string, dir Direction) string {
	currentLevelSize := len(*queue)
	for i := 0; i < currentLevelSize; i++ {
		current := (*queue)[0]
		*queue = (*queue)[1:]

		neighbors, err := s.getNeighbors(ctx, store, current, dir)
		if err != nil {
			fmt.Printf("Error getting neighbors for %s: %v\n", current, err)
			continue
		}

		for _, n := range neighbors {
			if _, seen := visitedMy[n]; !seen {
				visitedMy[n] = current
				*queue = append(*queue, n)

				// Check intersection
				if _, hit := visitedOther[n]; hit {
					fmt.Printf("[Pathfinder] Intersection met at: %s\n", n)
					return n
				}
			}
		}
	}
	return ""
}

func (s *GraphService) constructBidirectionalPath(ctx context.Context, store *meb.MEBStore, intersection string, visitedStart map[string]string, visitedEnd map[string]string) (*export.D3Graph, error) {
	path := []string{}

	// Trace back to Start
	curr := intersection
	for curr != "" {
		path = append([]string{curr}, path...) // Prepend
		curr = visitedStart[curr]
	}

	// Trace forward to End
	curr = visitedEnd[intersection]
	for curr != "" {
		path = append(path, curr)
		curr = visitedEnd[curr]
	}

	return s.buildGraphFromPath(ctx, store, path)
}

func (s *GraphService) getNeighbors(ctx context.Context, store *meb.MEBStore, nodeID string, dir Direction) ([]string, error) {
	neighbors := make([]string, 0)
	quoteID := fmt.Sprintf("\"%s\"", nodeID)

	var wg sync.WaitGroup
	var mu sync.Mutex

	add := func(list []string) {
		mu.Lock()
		defer mu.Unlock()
		neighbors = append(neighbors, list...)
	}

	// Helper helper to query logic
	runQuery := func(q string, isOutbound bool) {
		defer wg.Done()
		results, err := store.Query(ctx, q)
		if err != nil {
			return
		}

		localNeighbors := make([]string, 0)
		for _, res := range results {
			var obj string
			if isOutbound {
				obj, _ = res["?o"].(string)
			} else {
				obj, _ = res["?s"].(string)
			}

			// Resolve Candidates logic
			candidates := s.resolveCandidates(ctx, store, obj)
			localNeighbors = append(localNeighbors, candidates...)
		}
		add(localNeighbors)
	}

	wg.Add(2)

	if dir == DirectionForward {
		// Forward: Outbound (calls, imports, defines-children)
		// + Inbound defines (parents)
		go runQuery(fmt.Sprintf(`triples(%s, ?p, ?o)`, quoteID), true) // ?p check inside? Simplification: get all outbound and filter later?
		// Actually for speed, let's be specific or filter.
		// The generic query captures calls, imports, defines.

		// Inbound defines (Parent)
		go runQuery(fmt.Sprintf(`triples(?s, "defines", %s)`, quoteID), false)

	} else {
		// Backward: Inbound (calls, imports, defines-children => wait, defines-parent?)
		// Logic: If A calls B. Fwd: A->B. Bwd: B->A.
		// Query: triples(?s, "calls", B).

		// Inbound calls/imports
		go runQuery(fmt.Sprintf(`triples(?s, ?p, %s)`, quoteID), false)

		// Outbound defines (Parent -> Child relation reversed is Child -> Parent? No.)
		// If A defines B. Fwd: A -> B.
		// Bwd: B -> A.
		// So checking "who defines B" is the reverse edge.
		// triples(?s, "defines", B) -> returns A.
		// Wait, Fwd traversal usually allows going from Files to Symbols (Defines).
		// So Backward traversal must allow Symbols to Files (Defined By).
		// That is Inbound Defines.

		// What about Child -> Parent in Forward?
		// If Fwd allows Symbol -> File (Parent), then Bwd must allow File -> Symbol (Children).
		// Use triples(B, "defines", ?o).
		go runQuery(fmt.Sprintf(`triples(%s, "defines", ?o)`, quoteID), true)
	}

	wg.Wait()

	// Dedup
	unique := make(map[string]bool)
	final := make([]string, 0)
	for _, n := range neighbors {
		// Filter out predicates if we used wildcard
		// But here we rely on the fact that most links are calls/imports/defines.
		// We should technically filter logic, but for now assuming all links are relevant for pathfinding
		// is safer for connectivity.
		if n != nodeID && !unique[n] {
			unique[n] = true
			final = append(final, n)
		}
	}

	// Cap neighbor count for safety (though bidirectional makes this less critical)
	if len(final) > 1000 {
		return final[:1000], nil
	}

	return final, nil
}

// resolveCandidates resolves logical IDs to Canonical IDs.
// Optimization: Skips expensive fuzzy search for full path IDs.
func (s *GraphService) resolveCandidates(ctx context.Context, store *meb.MEBStore, obj string) []string {
	if strings.Contains(obj, "/") {
		return []string{obj}
	}

	ids := []string{obj}
	matches, err := store.SearchSymbols(obj, 20, "")
	if err == nil {
		for _, m := range matches {
			if m != obj {
				ids = append(ids, m)
			}
		}
	}
	return ids
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
			Source: src, Target: dst, Relation: "related", // Generic relation for path
		})
	}
	return graph, nil
}
