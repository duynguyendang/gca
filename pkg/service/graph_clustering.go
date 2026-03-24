package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/meb/clustering"
)

// DetectCommunityHierarchy runs the Leiden algorithm on the graph and returns a hierarchical structure.
func (s *GraphService) DetectCommunityHierarchy(ctx context.Context, projectID string) (*clustering.CommunityHierarchy, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}
	return store.DetectCommunities("default")
}

// GetHybridClusters performs k-means clustering on vector search results while preserving community structure.
func (s *GraphService) GetHybridClusters(ctx context.Context, projectID string, queryEmbedding []float32, limit int, numClusters int) (*clustering.HybridClusteringResult, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}
	return store.ClusterWithHybrid("default", queryEmbedding, limit, numClusters)
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
	log.Printf("[ClusterGraphData] Starting with %d nodes and %d links", len(fullGraph.Nodes), len(fullGraph.Links))

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
	log.Println("[ClusterGraphData] Running Leiden algorithm...")
	result := clusteringSvc.DetectCommunitiesLeiden(nodes, links)
	log.Printf("[ClusterGraphData] Leiden returned %d clusters", len(result.Clusters))

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
