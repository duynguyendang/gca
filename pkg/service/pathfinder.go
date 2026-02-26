package service

import (
	"container/heap"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/meb"
)

// FindShortestPath implements Dijkstra to find the shortest weighted path.
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

	fmt.Printf("[Pathfinder] Dijkstra %s -> %s\n", cleanStart, cleanEnd)

	// Dijkstra State
	pq := &PriorityQueue{}
	heap.Init(pq)

	// Distance map: NodeID -> Best Cost
	dist := make(map[string]int)
	// Parent map: NodeID -> ParentID (for reconstruction)
	parent := make(map[string]string)

	dist[cleanStart] = 0
	heap.Push(pq, &Item{
		Value:    cleanStart,
		Priority: 0, // Min-Heap based on cost (priority)
	})

	found := false
	processed := 0

	// API Bridge Portals: Pre-compute URL -> Handler map for O(1) jump
	portals := make(map[string]string)
	resPortals, _ := store.Query(ctx, fmt.Sprintf(`triples(?url, "%s", ?handler)`, "handled_by"))
	for _, r := range resPortals {
		url, _ := r["?url"].(string)
		handler, _ := r["?handler"].(string)
		portals[url] = handler
	}

	startT := time.Now()
	neighborCache := make(map[string]map[string]string) // node -> neighbor -> predicate
	depth := make(map[string]int)
	edgePred := make(map[string]string) // curr -> predicate from parent
	depth[cleanStart] = 0

	fmt.Printf("[Pathfinder] Dijkstra START: %s -> %s\n", cleanStart, cleanEnd)

	for pq.Len() > 0 {
		item := heap.Pop(pq).(*Item)
		curr := item.Value
		cost := item.Priority

		if cost > dist[curr] {
			continue // Stale item
		}

		processed++
		if curr == cleanEnd {
			found = true
			break
		}

		if processed > 10000 { // Safety break
			break
		}

		d := depth[curr]
		if d >= 10 { // Depth limit
			continue
		}

		// Get Neighbors with Predicates (Cached)
		var neighbors map[string]string
		if cached, ok := neighborCache[curr]; ok {
			neighbors = cached
		} else {
			neighbors = s.getWeightedNeighbors(ctx, store, curr, portals)
			neighborCache[curr] = neighbors
		}

		// Sort neighbors by weight to ensure branching doesn't cut high-priority links
		type neighborWeight struct {
			n    string
			pred string
			w    int
		}
		sorted := make([]neighborWeight, 0, len(neighbors))
		for n, pred := range neighbors {
			sorted = append(sorted, neighborWeight{n, pred, s.getWeight(pred)})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].w < sorted[j].w
		})

		for i, nw := range sorted {
			if i >= 50 { // Branching control AFTER sorting
				break
			}

			n, pred, weight := nw.n, nw.pred, nw.w
			newCost := cost + weight
			if oldD, ok := dist[n]; !ok || newCost < oldD {
				dist[n] = newCost
				parent[n] = curr
				edgePred[n] = pred
				depth[n] = d + 1
				heap.Push(pq, &Item{
					Value:    n,
					Priority: newCost,
				})
			}
		}
	}

	fmt.Printf("[Pathfinder] Processed %d nodes in %v. Found: %v\n", processed, time.Since(startT), found)

	if found {
		// Reconstruct Path
		path := []string{}
		curr := cleanEnd
		links := []export.D3Link{}
		for curr != "" {
			path = append([]string{curr}, path...) // Prepend
			if curr == cleanStart {
				break
			}
			p := parent[curr]
			pred := edgePred[curr]
			if p != "" { // Only create link if parent exists (not at start)
				links = append([]export.D3Link{{Source: p, Target: curr, Relation: pred}}, links...)
			}
			curr = p
		}
		fmt.Printf("[Pathfinder] Path RECONSTRUCTED: %d nodes, %d links\n", len(path), len(links))
		return s.buildGraphFromPath(ctx, store, path, links)
	}

	// File-Level Fallback
	startFile := strings.Split(cleanStart, ":")[0]
	endFile := strings.Split(cleanEnd, ":")[0]

	if (strings.Contains(cleanStart, ":") || strings.Contains(cleanEnd, ":")) &&
		(startFile != cleanStart || endFile != cleanEnd) {
		fmt.Printf("[Pathfinder] Fallback to file-level: %s -> %s\n", startFile, endFile)
		return s.FindShortestPath(ctx, projectID, startFile, endFile)
	}

	return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
}

func (s *GraphService) getWeight(pred string) int {
	switch pred {
	case "calls", "calls_api", "handled_by", "references", "exports":
		return 1
	case "imports", "defines", "in_package":
		return 10
	}
	return 5 // Default for others (e.g. parent defines)
}

func (s *GraphService) getWeightedNeighbors(ctx context.Context, store *meb.MEBStore, nodeID string, portals map[string]string) map[string]string {
	neighbors := make(map[string]string)

	// Portals check (Logical jump)
	if handler, ok := portals[nodeID]; ok {
		neighbors[handler] = "handled_by"
	}

	// 1. Outbound edges
	for fact, err := range store.Scan(nodeID, "", "", "default") {
		if err != nil {
			continue
		}
		pred := fact.Predicate
		obj := fact.Object.(string)

		if obj == nodeID {
			continue
		}

		if oldPred, exists := neighbors[obj]; !exists || s.getWeight(pred) < s.getWeight(oldPred) {
			neighbors[obj] = pred
		}
	}

	// 2. Inbound 'defines' (Structure Nav)
	for fact, err := range store.Scan("", "defines", nodeID, "default") {
		if err != nil {
			continue
		}
		parent := fact.Subject
		pred := "parent_defines"
		if oldPred, exists := neighbors[parent]; !exists || s.getWeight(pred) < s.getWeight(oldPred) {
			neighbors[parent] = pred
		}
	}

	return neighbors
}

func (s *GraphService) buildGraphFromPath(ctx context.Context, store *meb.MEBStore, path []string, pathLinks []export.D3Link) (*export.D3Graph, error) {
	graph := &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: pathLinks,
	}

	// Nodes
	ids := make([]string, len(path))
	for i, id := range path {
		ids[i] = string(id)
	}
	hydrated, _ := s.HydrateShallow(ctx, store, ids)
	hMap := make(map[string]HydratedSymbol)
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

	return graph, nil
}

// --- Priority Queue ---

type Item struct {
	Value    string
	Priority int // Cost
	Index    int
}

type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// Min-Heap: We want lower cost (priority) to pop first
	return pq[i].Priority < pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*Item)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.Index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}
