package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

type GraphSnapshot struct {
	Timestamp   time.Time              `json:"timestamp"`
	ProjectID   string                 `json:"project_id"`
	NodeCount   int                    `json:"node_count"`
	EdgeCount   int                    `json:"edge_count"`
	Nodes       map[string]NodeSnap   `json:"nodes"`
	Edges       map[string]EdgeSnap   `json:"edges"`
	Communities map[string]int         `json:"communities"`
}

type NodeSnap struct {
	Kind       string `json:"kind"`
	File       string `json:"file,omitempty"`
	Language   string `json:"language,omitempty"`
	ClusterID  int    `json:"cluster_id,omitempty"`
	Centrality int    `json:"centrality,omitempty"`
}

type EdgeSnap struct {
	Predicate   string  `json:"predicate"`
	SourceFile  string  `json:"source_file,omitempty"`
	TargetFile  string  `json:"target_file,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	ConfidenceTier string `json:"confidence_tier,omitempty"`
}

type GraphDiff struct {
	NewNodes           []NodeDiff     `json:"new_nodes"`
	RemovedNodes       []string       `json:"removed_nodes"`
	NewEdges           []EdgeDiff     `json:"new_edges"`
	RemovedEdges       []string       `json:"removed_edges"`
	CommunityChanges   []CommChange   `json:"community_changes"`
	Summary            DiffSummary    `json:"summary"`
}

type NodeDiff struct {
	ID    string    `json:"id"`
	Kind  string    `json:"kind"`
	File  string    `json:"file,omitempty"`
	Score float64   `json:"surprise_score,omitempty"`
}

type EdgeDiff struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Predicate string  `json:"predicate"`
	Score     float64 `json:"surprise_score,omitempty"`
}

type CommChange struct {
	Node         string `json:"node"`
	BeforeCluster int    `json:"before_cluster"`
	AfterCluster  int    `json:"after_cluster"`
}

type DiffSummary struct {
	NodesAdded     int `json:"nodes_added"`
	NodesRemoved   int `json:"nodes_removed"`
	EdgesAdded     int `json:"edges_added"`
	EdgesRemoved   int `json:"edges_removed"`
	CommunityMoves int `json:"community_moves"`
	BeforeTotal    int `json:"before_total_nodes"`
	AfterTotal     int `json:"after_total_nodes"`
}

type GraphDiffService struct{}

func NewGraphDiffService() *GraphDiffService {
	return &GraphDiffService{}
}

func (s *GraphDiffService) TakeSnapshot(ctx context.Context, store *meb.MEBStore, projectID string) (*GraphSnapshot, error) {
	snap := &GraphSnapshot{
		Timestamp:   time.Now(),
		ProjectID:   projectID,
		Nodes:       make(map[string]NodeSnap),
		Edges:       make(map[string]EdgeSnap),
		Communities: make(map[string]int),
	}

	// Capture all defined symbols
	for fact := range store.ScanContext(ctx, "", config.PredicateDefines, "") {
		if sym, ok := fact.Object.(string); ok {
			snap.Nodes[sym] = NodeSnap{Kind: "symbol"}
		}
	}

	// Capture all files (entities without colon in ID)
	for fact := range store.ScanContext(ctx, "", config.PredicateCalls, "") {
		if fact.Subject != "" && snap.Nodes[fact.Subject].Kind == "" {
			snap.Nodes[fact.Subject] = NodeSnap{Kind: "file"}
		}
		if objStr, ok := fact.Object.(string); ok && snap.Nodes[objStr].Kind == "" {
			snap.Nodes[objStr] = NodeSnap{Kind: "file"}
		}
	}

	// Capture imports
	for fact := range store.ScanContext(ctx, "", config.PredicateImports, "") {
		if fact.Subject != "" && snap.Nodes[fact.Subject].Kind == "" {
			snap.Nodes[fact.Subject] = NodeSnap{Kind: "file"}
		}
		if objStr, ok := fact.Object.(string); ok && snap.Nodes[objStr].Kind == "" {
			snap.Nodes[objStr] = NodeSnap{Kind: "file"}
		}
	}

	// Capture cluster assignments
	for fact := range store.ScanContext(ctx, "", "belongs_to_cluster", "") {
		if clusterStr, ok := fact.Object.(string); ok {
			var clusterID int
			fmt.Sscanf(clusterStr, "cluster_%d", &clusterID)
			snap.Communities[fact.Subject] = clusterID
			if node, exists := snap.Nodes[fact.Subject]; exists {
				node.ClusterID = clusterID
				snap.Nodes[fact.Subject] = node
			}
		}
	}

	// Capture call edges
	for fact := range store.ScanContext(ctx, "", config.PredicateCalls, "") {
		objStr, ok := fact.Object.(string)
		if !ok {
			continue
		}
		edgeKey := fmt.Sprintf("%s->%s", fact.Subject, objStr)
		snap.Edges[edgeKey] = EdgeSnap{Predicate: config.PredicateCalls}
	}

	// Capture import edges
	for fact := range store.ScanContext(ctx, "", config.PredicateImports, "") {
		objStr, ok := fact.Object.(string)
		if !ok {
			continue
		}
		edgeKey := fmt.Sprintf("%s->%s", fact.Subject, objStr)
		snap.Edges[edgeKey] = EdgeSnap{Predicate: config.PredicateImports}
	}

	snap.NodeCount = len(snap.Nodes)
	snap.EdgeCount = len(snap.Edges)

	logger.Debug("Snapshot taken", "nodes", snap.NodeCount, "edges", snap.EdgeCount)
	return snap, nil
}

func (s *GraphDiffService) DiffSnapshots(before, after *GraphSnapshot) *GraphDiff {
	diff := &GraphDiff{
		NewNodes:         []NodeDiff{},
		RemovedNodes:     []string{},
		NewEdges:         []EdgeDiff{},
		RemovedEdges:     []string{},
		CommunityChanges: []CommChange{},
		Summary:         DiffSummary{},
	}

	for id, node := range after.Nodes {
		if _, exists := before.Nodes[id]; !exists {
			diff.NewNodes = append(diff.NewNodes, NodeDiff{ID: id, Kind: node.Kind, File: node.File})
		}
	}

	for id := range before.Nodes {
		if _, exists := after.Nodes[id]; !exists {
			diff.RemovedNodes = append(diff.RemovedNodes, id)
		}
	}

	for id, edge := range after.Edges {
		if _, exists := before.Edges[id]; !exists {
			parts := splitEdgeKey(id)
			diff.NewEdges = append(diff.NewEdges, EdgeDiff{
				ID:        id,
				Source:    parts[0],
				Target:    parts[1],
				Predicate: edge.Predicate,
			})
		}
	}

	for id := range before.Edges {
		if _, exists := after.Edges[id]; !exists {
			diff.RemovedEdges = append(diff.RemovedEdges, id)
		}
	}

	for nodeID, afterCluster := range after.Communities {
		if beforeCluster, exists := before.Communities[nodeID]; exists {
			if beforeCluster != afterCluster {
				diff.CommunityChanges = append(diff.CommunityChanges, CommChange{
					Node:         nodeID,
					BeforeCluster: beforeCluster,
					AfterCluster:  afterCluster,
				})
			}
		}
	}

	diff.Summary = DiffSummary{
		NodesAdded:     len(diff.NewNodes),
		NodesRemoved:   len(diff.RemovedNodes),
		EdgesAdded:     len(diff.NewEdges),
		EdgesRemoved:   len(diff.RemovedEdges),
		CommunityMoves: len(diff.CommunityChanges),
		BeforeTotal:    before.NodeCount,
		AfterTotal:     after.NodeCount,
	}

	return diff
}

func (s *GraphDiffService) SaveSnapshot(snap *GraphSnapshot, path string) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *GraphDiffService) LoadSnapshot(path string) (*GraphSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap GraphSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func splitEdgeKey(key string) []string {
	for i := 0; i < len(key); i++ {
		if key[i] == '-' && i+1 < len(key) && key[i+1] == '>' {
			return []string{key[:i], key[i+2:]}
		}
	}
	return []string{key, ""}
}