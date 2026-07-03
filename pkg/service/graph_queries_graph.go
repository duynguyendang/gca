package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/logger"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/repl"
)

// GetProjectMap returns a high-level view of file dependencies (imports only).
func (s *GraphService) GetProjectMap(ctx context.Context, projectID string) (*export.D3Graph, error) {
	s.cacheMu.RLock()
	if graph, ok := s.projectMapCache[projectID]; ok {
		s.cacheMu.RUnlock()
		return graph, nil
	}
	s.cacheMu.RUnlock()

	query := fmt.Sprintf(`triples(?s, "%s", ?o)`, config.PredicateImports)

	graph, err := s.ExportGraph(ctx, projectID, query, false, false, 0, 0)
	if err != nil {
		return nil, err
	}

	store, err := s.getStore(projectID)
	if err == nil {
		s.resolvePackageImportsToFiles(ctx, store, graph, "")
	}

	s.cacheMu.Lock()
	s.projectMapCache[projectID] = graph
	s.cacheMu.Unlock()

	return graph, nil
}

// GetSubgraph returns a subset of the graph containing the specified nodes and their connections.
func (s *GraphService) GetSubgraph(ctx context.Context, projectID string, ids []string) (*export.D3Graph, error) {
	fullGraph, err := s.GetProjectMap(ctx, projectID)
	if err != nil {
		return nil, err
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	subgraph := &export.D3Graph{
		Nodes: make([]export.D3Node, 0, len(ids)),
		Links: make([]export.D3Link, 0),
	}

	for _, n := range fullGraph.Nodes {
		if idSet[n.ID] {
			subgraph.Nodes = append(subgraph.Nodes, n)
		}
	}

	for _, l := range fullGraph.Links {
		if idSet[l.Source] || idSet[l.Target] {
			subgraph.Links = append(subgraph.Links, l)
		}
	}

	return subgraph, nil
}

// GetFileDetails returns detailed internal structure of a file.
func (s *GraphService) GetFileDetails(ctx context.Context, projectID, fileID string) (*export.D3Graph, error) {
	cleanFileID := strings.Trim(fileID, "\"")
	quotedFileID := fmt.Sprintf("\"%s\"", cleanFileID)

	q1 := fmt.Sprintf(`triples(%s, "%s", ?s)`, quotedFileID, config.PredicateDefines)
	q2 := fmt.Sprintf(`triples(?s, "%s", ?o), triples(%s, "%s", ?s), triples(%s, "%s", ?o)`,
		config.PredicateCalls, quotedFileID, config.PredicateDefines, quotedFileID, config.PredicateDefines)

	mergedGraph := &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}

	g1, err := s.ExportGraph(ctx, projectID, q1, true, true, 0, 0)
	if err == nil {
		mergedGraph.Nodes = append(mergedGraph.Nodes, g1.Nodes...)
		mergedGraph.Links = append(mergedGraph.Links, g1.Links...)
	}

	g2, err := s.ExportGraph(ctx, projectID, q2, false, true, 0, 0)
	if err == nil {
		nodeMap := make(map[string]bool)
		for _, n := range mergedGraph.Nodes {
			nodeMap[n.ID] = true
		}
		for _, n := range g2.Nodes {
			if !nodeMap[n.ID] {
				mergedGraph.Nodes = append(mergedGraph.Nodes, n)
				nodeMap[n.ID] = true
			}
		}
		mergedGraph.Links = append(mergedGraph.Links, g2.Links...)
	}

	for i := range mergedGraph.Nodes {
		mergedGraph.Nodes[i].ParentID = cleanFileID
	}

	return mergedGraph, nil
}

// GetBackboneGraph returns a graph containing only cross-file dependencies.
func (s *GraphService) GetBackboneGraph(ctx context.Context, projectID string, aggregate bool) (*export.D3Graph, error) {
	query := fmt.Sprintf(`triples(?s, "%s", ?o)`, config.PredicateCalls)
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	results, err := gcamdb.Query(ctx, store, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	backbone := &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}
	nodeSet := make(map[string]bool)

	for _, r := range results {
		srcID, ok1 := r["?s"].(string)
		tgtID, ok2 := r["?o"].(string)
		if !ok1 || !ok2 {
			continue
		}

		srcParts := strings.SplitN(srcID, ":", 2)
		tgtParts := strings.SplitN(tgtID, ":", 2)

		if len(srcParts) < 2 || len(tgtParts) < 2 {
			continue
		}

		srcFile := srcParts[0]
		tgtFile := tgtParts[0]

		if srcFile != tgtFile {
			if aggregate {
				linkKey := srcFile + "->" + tgtFile
				if !nodeSet[linkKey] {
					backbone.Links = append(backbone.Links, export.D3Link{
						Source:   srcFile,
						Target:   tgtFile,
						Relation: config.RelationCalls,
						Weight:   1,
					})
				}

				if !nodeSet[srcFile] {
					backbone.Nodes = append(backbone.Nodes, export.D3Node{
						ID:   srcFile,
						Name: common.ExtractBaseName(srcFile),
						Kind: config.SymbolKindFile,
					})
					nodeSet[srcFile] = true
				}
				if !nodeSet[tgtFile] {
					backbone.Nodes = append(backbone.Nodes, export.D3Node{
						ID:   tgtFile,
						Name: common.ExtractBaseName(tgtFile),
						Kind: config.SymbolKindFile,
					})
					nodeSet[tgtFile] = true
				}
			} else {
				backbone.Links = append(backbone.Links, export.D3Link{
					Source:   srcID,
					Target:   tgtID,
					Relation: config.RelationCalls,
				})

				if !nodeSet[srcID] {
					backbone.Nodes = append(backbone.Nodes, export.D3Node{
						ID:       srcID,
						Name:     srcParts[1],
						Kind:     config.SymbolKindGateway,
						ParentID: srcFile,
					})
					nodeSet[srcID] = true
				}
				if !nodeSet[tgtID] {
					backbone.Nodes = append(backbone.Nodes, export.D3Node{
						ID:       tgtID,
						Name:     tgtParts[1],
						Kind:     config.SymbolKindGateway,
						ParentID: tgtFile,
					})
					nodeSet[tgtID] = true
				}
			}
		}
	}

	if aggregate {
		uniqueLinks := make([]export.D3Link, 0)
		linkSeen := make(map[string]bool)
		for _, l := range backbone.Links {
			key := fmt.Sprintf("%s->%s", l.Source, l.Target)
			if !linkSeen[key] {
				uniqueLinks = append(uniqueLinks, l)
				linkSeen[key] = true
			}
		}
		backbone.Links = uniqueLinks
	}

	if len(backbone.Nodes) > 0 {
		if err := s.enrichNodes(ctx, store, backbone, true); err != nil {
			logger.Warn("Backbone enrichment warning", "error", err)
		}
	}

	return backbone, nil
}

// GenerateSummary generates a project summary.
func (s *GraphService) GenerateSummary(projectID string) (*repl.ProjectSummary, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	summary, err := repl.GenerateProjectSummary(store)
	if err != nil {
		return nil, err
	}

	return summary, nil
}

// ResolveVirtualTriples identifies potential implicit relationships.
func (s *GraphService) ResolveVirtualTriples(ctx context.Context, projectID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	links := []export.D3Link{}
	nodes := []export.D3Node{}

	interfaces, err := gcamdb.Query(ctx, store, fmt.Sprintf(`triples(?s, "%s", "interface")`, config.PredicateHasKind))
	if err != nil {
		return nil, err
	}

	structs, err := gcamdb.Query(ctx, store, fmt.Sprintf(`triples(?s, "%s", "struct")`, config.PredicateHasKind))
	if err != nil {
		return nil, err
	}

	uniqueInterfaces := make(map[string]bool)
	for _, row := range interfaces {
		if s, ok := row["?s"].(string); ok {
			uniqueInterfaces[s] = true
		}
	}

	uniqueStructs := make(map[string]bool)
	for _, row := range structs {
		if s, ok := row["?s"].(string); ok {
			uniqueStructs[s] = true
		}
	}

	for iName := range uniqueInterfaces {
		shortName := common.ExtractSymbolName(iName)

		for sName := range uniqueStructs {
			sShort := common.ExtractSymbolName(sName)

			if strings.HasSuffix(sShort, "Impl") && strings.TrimSuffix(sShort, "Impl") == shortName {
				links = append(links, export.D3Link{
					Source:   iName,
					Target:   sName,
					Relation: config.VirtualRelationWiresTo,
					Type:     "virtual",
					Weight:   0.8,
				})
			}
			if strings.HasPrefix(sShort, "Default") && strings.TrimPrefix(sShort, "Default") == shortName {
				links = append(links, export.D3Link{
					Source:   iName,
					Target:   sName,
					Relation: config.VirtualRelationWiresTo,
					Type:     "virtual",
					Weight:   0.8,
				})
			}
		}
	}

	return &export.D3Graph{Nodes: nodes, Links: links}, nil
}
