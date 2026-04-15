package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/logger"
)

// CommunityHierarchy represents a hierarchical community structure.
type CommunityHierarchy struct {
	Levels []CommunityLevel `json:"levels"`
}

// CommunityLevel represents a single level in the hierarchy.
type CommunityLevel struct {
	ID          int      `json:"id"`
	Communities []int    `json:"communities"`
	Members     []string `json:"members,omitempty"`
}

// HybridClusteringResult contains hybrid clustering results.
type HybridClusteringResult struct {
	Clusters []HybridCluster `json:"clusters"`
}

// HybridCluster represents a single cluster in hybrid results.
type HybridCluster struct {
	ID      int      `json:"id"`
	Members []string `json:"members"`
}

// DetectCommunityHierarchy runs the Leiden algorithm on the graph and returns a hierarchical structure.
func (s *GraphService) DetectCommunityHierarchy(ctx context.Context, projectID string) (*CommunityHierarchy, error) {
	graph, err := s.ExportGraph(ctx, projectID, "", false, false)
	if err != nil {
		return nil, err
	}

	if len(graph.Nodes) == 0 {
		return &CommunityHierarchy{Levels: []CommunityLevel{}}, nil
	}

	nodes := make([]GraphNode, len(graph.Nodes))
	for i, n := range graph.Nodes {
		nodes[i] = GraphNode{ID: n.ID, Name: n.Name, Kind: n.Kind}
	}

	links := make([]GraphLink, len(graph.Links))
	for i, l := range graph.Links {
		links[i] = GraphLink{Source: l.Source, Target: l.Target}
	}

	clusteringSvc := NewClusteringService()
	result := clusteringSvc.DetectCommunitiesLeiden(nodes, links)

	hierarchy := &CommunityHierarchy{
		Levels: make([]CommunityLevel, 1),
	}

	membersByCluster := make(map[int][]string)
	for nodeID, clusterID := range result.NodeCluster {
		membersByCluster[clusterID] = append(membersByCluster[clusterID], nodeID)
	}

	var commIDs []int
	for id := range membersByCluster {
		commIDs = append(commIDs, id)
	}

	hierarchy.Levels[0] = CommunityLevel{
		ID:          0,
		Communities: commIDs,
	}

	return hierarchy, nil
}

// GetHybridClusters performs k-means clustering on vector search results while preserving community structure.
func (s *GraphService) GetHybridClusters(ctx context.Context, projectID string, queryEmbedding []float32, limit int, numClusters int) (*HybridClusteringResult, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	results, err := store.Find().
		SimilarTo(queryEmbedding).
		Limit(limit).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	if len(results) == 0 {
		return &HybridClusteringResult{}, nil
	}

	// Assign results evenly across requested clusters
	clusters := make([]HybridCluster, numClusters)
	for i := range clusters {
		clusters[i] = HybridCluster{ID: i}
	}

	for i, r := range results {
		clusterIdx := i % numClusters
		clusters[clusterIdx].Members = append(clusters[clusterIdx].Members, r.Key)
	}

	return &HybridClusteringResult{Clusters: clusters}, nil
}

// GetClusterGraph applies Leiden clustering to reduce large graphs.
func (s *GraphService) GetClusterGraph(ctx context.Context, projectID, query string) (*export.D3Graph, error) {
	fullGraph, err := s.ExportGraph(ctx, projectID, query, true, false)
	if err != nil {
		return nil, err
	}

	return s.ClusterGraphData(fullGraph)
}

// ClusterGraphData takes an existing D3Graph and applies clustering to it.
func (s *GraphService) ClusterGraphData(fullGraph *export.D3Graph) (*export.D3Graph, error) {
	logger.Debug("ClusterGraphData starting", "nodes", len(fullGraph.Nodes), "links", len(fullGraph.Links))

	if len(fullGraph.Nodes) == 0 {
		return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
	}

	if len(fullGraph.Nodes) < config.MinNodesForClustering {
		return fullGraph, nil
	}

	nodes := make([]GraphNode, len(fullGraph.Nodes))
	for i, n := range fullGraph.Nodes {
		nodes[i] = GraphNode{
			ID:   n.ID,
			Name: n.Name,
			Kind: n.Kind,
		}
	}

	links := make([]GraphLink, len(fullGraph.Links))
	for i, l := range fullGraph.Links {
		links[i] = GraphLink{
			Source: l.Source,
			Target: l.Target,
		}
	}

	clusteringSvc := NewClusteringService()
	logger.Debug("Running Leiden algorithm")
	result := clusteringSvc.DetectCommunitiesLeiden(nodes, links)
	logger.Debug("Leiden returned clusters", "clusters", len(result.Clusters))

	superNodes := make([]export.D3Node, 0, len(result.Clusters))
	superLinks := make([]export.D3Link, 0)

	for clusterID, memberIDs := range result.Clusters {
		dirCounts := make(map[string]int)
		for _, id := range memberIDs {
			lastSlash := strings.LastIndex(id, "/")
			if lastSlash != -1 {
				dir := id[:lastSlash]
				dirCounts[dir]++
			} else {
				dirCounts["/"]++
			}
		}

		bestDir := ""
		maxCount := -1
		for dir, count := range dirCounts {
			if count > maxCount {
				maxCount = count
				bestDir = dir
			}
		}

		clusterLabel := fmt.Sprintf("Cluster %d", clusterID)
		if bestDir != "" && bestDir != "/" {
			parts := strings.Split(bestDir, "/")
			if len(parts) > 2 {
				clusterLabel = strings.Join(parts[len(parts)-2:], "/")
			} else {
				clusterLabel = bestDir
			}
		} else {
			clusterLabel = "Root"
		}

		superNodes = append(superNodes, export.D3Node{
			ID:   fmt.Sprintf("cluster_%d", clusterID),
			Name: fmt.Sprintf("%s (%d)", clusterLabel, len(memberIDs)),
			Kind: config.SymbolKindCluster,
			Metadata: map[string]string{
				"cluster_id":     fmt.Sprintf("%d", clusterID),
				"member_count":   fmt.Sprintf("%d", len(memberIDs)),
				"representative": bestDir,
				"members":        strings.Join(memberIDs, ","),
			},
		})
	}

	linkWeights := make(map[string]int)

	for _, l := range fullGraph.Links {
		srcCluster := result.NodeCluster[l.Source]
		tgtCluster := result.NodeCluster[l.Target]

		if srcCluster == tgtCluster {
			continue
		}

		key := fmt.Sprintf("cluster_%d->cluster_%d", srcCluster, tgtCluster)
		linkWeights[key]++
	}

	for key, weight := range linkWeights {
		parts := strings.Split(key, "->")
		if len(parts) != 2 {
			continue
		}

		superLinks = append(superLinks, export.D3Link{
			Source:   parts[0],
			Target:   parts[1],
			Relation: config.RelationAggregated,
			Weight:   float64(weight),
		})
	}

	return &export.D3Graph{
		Nodes: superNodes,
		Links: superLinks,
	}, nil
}
