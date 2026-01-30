package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

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

	maxDepth := 15 // Bidirectional search goes deep fast, 15 allows path len 30
	depth := 0

	// API Bridge Portals: Pre-compute URL -> Handler map for O(1) jump
	portals := make(map[string]string)
	resPortals, _ := store.Query(ctx, fmt.Sprintf(`triples(?url, "%s", ?handler)`, meb.PredHandledBy))
	for _, r := range resPortals {
		url, _ := r["?url"].(string)
		handler, _ := r["?handler"].(string)
		portals[url] = handler
	}

	for len(qStart) > 0 && len(qEnd) > 0 {
		if depth > maxDepth {
			break
		}
		depth++

		start := time.Now()
		var intersection string
		// Always expand the smaller frontier to balance search
		if len(qStart) <= len(qEnd) {
			intersection = expandFrontier(ctx, s, store, &qStart, visitedStart, visitedEnd, DirectionForward, portals, depth)
		} else {
			intersection = expandFrontier(ctx, s, store, &qEnd, visitedEnd, visitedStart, DirectionBackward, portals, depth)
		}
		fmt.Printf("[Pathfinder] Level %d expanded in %v. Frontier sizes: L=%d, R=%d\n", depth, time.Since(start), len(qStart), len(qEnd))

		if intersection != "" {
			return s.constructBidirectionalPath(ctx, store, intersection, visitedStart, visitedEnd)
		}
	}

	// File-Level Fallback: If symbol path fails, try parent files
	startFile := strings.Split(cleanStart, ":")[0]
	endFile := strings.Split(cleanEnd, ":")[0]

	// ONLY fallback if we were actually looking up symbols (containing :)
	// and if the file-level search is different from the current search
	if (strings.Contains(cleanStart, ":") || strings.Contains(cleanEnd, ":")) &&
		(startFile != cleanStart || endFile != cleanEnd) {
		fmt.Printf("[Pathfinder] Symbol path not found, trying file-level fallback: %s <-> %s\n", startFile, endFile)
		fileGraph, err := s.FindShortestPath(ctx, projectID, startFile, endFile)
		if err == nil && len(fileGraph.Nodes) > 0 {
			return fileGraph, nil
		}
	}

	fmt.Printf("[Pathfinder] Path not found.\n")
	return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
}

func expandFrontier(ctx context.Context, s *GraphService, store *meb.MEBStore, queue *[]string, visitedMy map[string]string, visitedOther map[string]string, dir Direction, portals map[string]string, depth int) string {
	currentLevelSize := len(*queue)
	nextQueue := make([]string, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var intersection string

	expandNode := func(current string) {
		defer wg.Done()

		neighbors, err := s.getNeighbors(ctx, store, current, dir, depth)
		if err != nil {
			return
		}

		mu.Lock()
		defer mu.Unlock()

		// Portals: If current node is calling an API or is an API route
		if dir == DirectionForward {
			if handlerID, isRoute := portals[current]; isRoute {
				if _, seen := visitedMy[handlerID]; !seen {
					visitedMy[handlerID] = current
					nextQueue = append(nextQueue, handlerID)
					if _, hit := visitedOther[handlerID]; hit {
						intersection = handlerID
					}
				}
			}
		} else {
			for route, handler := range portals {
				if handler == current {
					if _, seen := visitedMy[route]; !seen {
						visitedMy[route] = current
						nextQueue = append(nextQueue, route)
						if _, hit := visitedOther[route]; hit {
							intersection = route
						}
					}
				}
			}
		}

		for _, n := range neighbors {
			if _, seen := visitedMy[n]; !seen {
				visitedMy[n] = current
				nextQueue = append(nextQueue, n)
				if _, hit := visitedOther[n]; hit {
					intersection = n
				}
			}
		}
	}

	wg.Add(currentLevelSize)
	for i := 0; i < currentLevelSize; i++ {
		current := (*queue)[i]
		go expandNode(current)
	}
	wg.Wait()

	*queue = nextQueue
	return intersection
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

func (s *GraphService) getNeighbors(ctx context.Context, store *meb.MEBStore, nodeID string, dir Direction, depth int) ([]string, error) {
	quoteID := fmt.Sprintf("\"%s\"", nodeID)

	// Consolidated Query: Get ALL outbound/inbound triples in one go
	var q string
	if dir == DirectionForward {
		q = fmt.Sprintf(`triples(%s, ?p, ?o)`, quoteID)
	} else {
		q = fmt.Sprintf(`triples(?s, ?p, %s)`, quoteID)
	}

	results, err := store.Query(ctx, q)
	if err != nil {
		return nil, err
	}

	neighbors := make([]string, 0)
	unique := make(map[string]bool)
	nodePriority := make(map[string]int)

	// Predicate priority ordering
	priorityOrder := map[string]int{
		meb.PredCalls:     1,
		meb.PredCallsAPI:  1,
		meb.PredHandledBy: 1,
		meb.PredImports:   2,
		meb.PredDefines:   3,
		meb.PredInPackage: 3,
	}

	for _, res := range results {
		pred, _ := res["?p"].(string)
		var obj string
		if dir == DirectionForward {
			obj, _ = res["?o"].(string)
		} else {
			obj, _ = res["?s"].(string)
		}

		priority := priorityOrder[pred]
		if priority == 0 {
			priority = 5
		}

		if depth < 5 && priority > 3 {
			continue // Only skip truly noisy ones at very low depth
		}

		if obj != nodeID {
			if !unique[obj] {
				unique[obj] = true
				neighbors = append(neighbors, obj)
				nodePriority[obj] = priority
			} else if priority < nodePriority[obj] {
				nodePriority[obj] = priority // Keep best priority
			}
		}
	}

	// 2. Extra Junction
	var qJunction string
	if dir == DirectionForward {
		qJunction = fmt.Sprintf(`triples(?s, "defines", %s)`, quoteID)
	} else {
		qJunction = fmt.Sprintf(`triples(%s, "defines", ?o)`, quoteID)
	}
	resJ, _ := store.Query(ctx, qJunction)
	for _, r := range resJ {
		var obj string
		if dir == DirectionForward {
			obj, _ = r["?s"].(string)
		} else {
			obj, _ = r["?o"].(string)
		}
		if obj != nodeID {
			if !unique[obj] {
				unique[obj] = true
				neighbors = append(neighbors, obj)
				nodePriority[obj] = 3 // defines priority
			}
		}
	}

	// Sort neighbors by priority (lower number is better)
	type weightedNeighbor struct {
		id       string
		priority int
	}
	weighted := make([]weightedNeighbor, 0, len(neighbors))
	for _, n := range neighbors {
		weighted = append(weighted, weightedNeighbor{n, nodePriority[n]})
	}

	sort.Slice(weighted, func(i, j int) bool {
		return weighted[i].priority < weighted[j].priority
	})

	final := make([]string, 0, len(weighted))
	for i := 0; i < len(weighted); i++ {
		final = append(final, weighted[i].id)
	}

	// Branching Factor Cap (limit to 100 best)
	if len(final) > 100 {
		return final[:100], nil
	}

	return final, nil
}

// resolveCandidates resolves logical IDs to Canonical IDs.
// Optimization: Skips expensive fuzzy search for full path IDs.
func (s *GraphService) resolveCandidates(ctx context.Context, store *meb.MEBStore, obj string) []string {
	return []string{obj}
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
