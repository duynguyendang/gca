package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/datalog"
	"github.com/duynguyendang/gca/pkg/export"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/repl"
)

var queryOptimizer = datalog.NewQueryOptimizer()

// ExecuteQuery executes a Datalog query and returns results.
func (s *GraphService) ExecuteQuery(ctx context.Context, projectID, query string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	results, err := gcamdb.Query(ctx, store, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	return results, nil
}

// ExecuteQueryOptimized executes a Datalog query with optimization (join reordering and predicate pushdown).
func (s *GraphService) ExecuteQueryOptimized(ctx context.Context, projectID, query string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// Parse the query
	atoms, err := datalog.Parse(query)
	if err != nil {
		return nil, err
	}

	// Create optimized execution plan
	plan := queryOptimizer.CreateExecutionPlan(atoms)

	// Reconstruct the optimized query string
	optimizedQuery := reconstructQuery(plan.Atoms)

	// Execute the optimized query
	results, err := gcamdb.Query(ctx, store, optimizedQuery)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	// Apply any pushed-down predicates as post-processing filters
	if len(plan.PushdownPreds) > 0 {
		results = applyPushdownPredicates(results, plan.PushdownPreds)
	}

	return results, nil
}

// reconstructQuery reconstructs a Datalog query string from a list of atoms.
func reconstructQuery(atoms []datalog.Atom) string {
	var queryParts []string
	for _, atom := range atoms {
		queryParts = append(queryParts, formatAtom(atom))
	}
	return strings.Join(queryParts, ", ")
}

// formatAtom formats an atom back into Datalog syntax.
func formatAtom(atom datalog.Atom) string {
	args := strings.Join(atom.Args, ", ")
	return fmt.Sprintf("%s(%s)", atom.Predicate, args)
}

// applyPushdownPredicates applies pushed-down predicates as post-processing filters.
func applyPushdownPredicates(results []map[string]any, predicates map[string]string) []map[string]any {
	if len(predicates) == 0 {
		return results
	}

	filtered := make([]map[string]any, 0, len(results))

	for _, result := range results {
		if matchesPushdownPredicates(result, predicates) {
			filtered = append(filtered, result)
		}
	}

	return filtered
}

// matchesPushdownPredicates checks if a result matches all pushed-down predicates.
func matchesPushdownPredicates(result map[string]any, predicates map[string]string) bool {
	for varName, constraint := range predicates {
		value, ok := result[varName]
		if !ok {
			return false
		}

		valueStr := fmt.Sprintf("%v", value)

		// Handle different constraint types
		if strings.HasPrefix(constraint, "neq:") {
			// Not equals constraint
			expectedValue := strings.TrimPrefix(constraint, "neq:")
			if valueStr == expectedValue {
				return false
			}
		} else {
			// Equals constraint
			if valueStr != constraint {
				return false
			}
		}
	}
	return true
}

// GetManifest returns a compressed project manifest for the AI.
func (s *GraphService) GetManifest(ctx context.Context, projectID string) (map[string]interface{}, error) {
	store, err := s.manager.GetStore(projectID)
	if err != nil {
		return nil, err
	}

	fileMap := make(map[string]string)
	symbolMap := make(map[string]string)

	for fact, err := range store.Scan("", config.PredicateDefines, "") {
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		filePath := string(fact.Subject)
		fullID, ok := fact.Object.(string)
		if !ok {
			continue
		}

		fileMap[filePath] = filePath

		shortName := fullID
		parts := strings.Split(fullID, ":")
		if len(parts) > 1 {
			shortName = parts[len(parts)-1]
		}
		if idx := strings.LastIndex(shortName, "."); idx != -1 && idx < len(shortName)-1 {
			shortName = shortName[idx+1:]
		}

		symbolMap[shortName] = fullID
	}

	return map[string]interface{}{
		"F": fileMap,
		"S": symbolMap,
	}, nil
}

// GetSource returns the content of a specific file/symbol.
func (s *GraphService) GetSource(projectID, docID string) (string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return "", err
	}

	doc, err := store.GetContentByKey(string(docID))
	if err != nil {
		if projectID != "" && !strings.HasPrefix(docID, projectID+"/") {
			prefixedDocID := projectID + "/" + docID
			doc, err = store.GetContentByKey(string(prefixedDocID))
		}

		if err != nil {
			return "", fmt.Errorf("%w: document not found", errors.ErrNotFound)
		}
	}

	return string(doc), nil
}

// GetSymbol retrieves the full hydrated symbol (content + metadata) for a given ID.
func (s *GraphService) GetSymbol(ctx context.Context, projectID, docID string) (*HydratedSymbol, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	ids := []string{string(docID)}
	hydrated, err := s.Hydrate(ctx, store, projectID, ids)
	if err != nil || len(hydrated) == 0 || hydrated[0].Content == "" {
		if projectID != "" && !strings.HasPrefix(docID, projectID+"/") {
			prefixedDocID := projectID + "/" + docID
			ids = []string{string(prefixedDocID)}
			hydrated, err = s.Hydrate(ctx, store, projectID, ids)
		}

		if err != nil || len(hydrated) == 0 || hydrated[0].Content == "" {
			return nil, fmt.Errorf("%w: symbol not found", errors.ErrNotFound)
		}
	}

	return &hydrated[0], nil
}

// GetPredicates returns known predicates.
func (s *GraphService) GetPredicates(projectID string) ([]map[string]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	var results []map[string]string
	for _, p := range store.ListPredicates() {
		results = append(results, map[string]string{
			"name": string(p.Symbol),
		})
	}
	return results, nil
}

// SearchSymbols performs symbol search.
func (s *GraphService) SearchSymbols(projectID, query, predicate string, limit int) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = config.DefaultSearchLimit
	}

	var matches []string
	count := 0
	for fact, err := range store.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		if obj, ok := fact.Object.(string); ok {
			if strings.Contains(strings.ToLower(obj), strings.ToLower(query)) {
				matches = append(matches, obj)
				count++
				if count >= limit {
					break
				}
			}
		}
	}
	return matches, nil
}

// ListFiles returns all ingested file paths for a project.
func (s *GraphService) ListFiles(projectID string) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var files []string

	for fact, err := range store.Scan("", config.PredicateType, "") {
		if err != nil {
			continue
		}
		if obj, ok := fact.Object.(string); ok && obj == config.FileTypeFile {
			f := string(fact.Subject)
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files, nil
}

// GetProjectMap returns a high-level view of file dependencies (imports only).
func (s *GraphService) GetProjectMap(ctx context.Context, projectID string) (*export.D3Graph, error) {
	s.cacheMu.RLock()
	if graph, ok := s.projectMapCache[projectID]; ok {
		s.cacheMu.RUnlock()
		return graph, nil
	}
	s.cacheMu.RUnlock()

	query := fmt.Sprintf(`triples(?s, "%s", ?o)`, config.PredicateImports)

	graph, err := s.ExportGraph(ctx, projectID, query, false, false)
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

	g1, err := s.ExportGraph(ctx, projectID, q1, true, true)
	if err == nil {
		mergedGraph.Nodes = append(mergedGraph.Nodes, g1.Nodes...)
		mergedGraph.Links = append(mergedGraph.Links, g1.Links...)
	}

	g2, err := s.ExportGraph(ctx, projectID, q2, false, true)
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
			fmt.Printf("Backbone enrichment warning: %v\n", err)
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

// SemanticSearchResult represents a single semantic search result.
type SemanticSearchResult struct {
	SymbolID string  `json:"symbol_id"`
	Score    float32 `json:"score"`
	Name     string  `json:"name,omitempty"`
}

// SemanticSearch performs vector similarity search on embedded documentation.
func (s *GraphService) SemanticSearch(ctx context.Context, projectID, query string, k int, gemini interface {
	GetEmbedding(ctx context.Context, text string) ([]float32, error)
}) ([]SemanticSearchResult, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	embedding, err := gemini.GetEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	vecIter := store.Vectors().Search(embedding, k)

	results := make([]SemanticSearchResult, 0, k)
	for vr, err := range vecIter {
		if err != nil {
			break
		}
		symbolID, err := store.ResolveID(vr.ID)
		if err != nil {
			continue
		}
		name := symbolID
		if parts := strings.Split(symbolID, ":"); len(parts) > 1 {
			name = parts[len(parts)-1]
		}
		results = append(results, SemanticSearchResult{
			SymbolID: symbolID,
			Score:    vr.Score,
			Name:     name,
		})
	}

	return results, nil
}
