package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/repl"
)

// ProjectStoreManager interface abstraction
type ProjectStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
	ListProjects() ([]manager.ProjectMetadata, error)
}

// GraphService handles graph query and enrichment operations.
type GraphService struct {
	manager ProjectStoreManager
}

// NewGraphService creates a new GraphService.
func NewGraphService(manager ProjectStoreManager) *GraphService {
	return &GraphService{manager: manager}
}

// ListProjects returns a list of available projects.
func (s *GraphService) ListProjects() ([]manager.ProjectMetadata, error) {
	return s.manager.ListProjects()
}

// ExecuteQuery executes a Datalog query for a specific project.
func (s *GraphService) ExecuteQuery(ctx context.Context, projectID, query string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	results, err := store.Query(ctx, query)
	if err != nil {
		// Can we differentiate invalid query vs internal error here?
		// Assuming Query returns standard errors, we might wrap them.
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	return results, nil
}

// ExportGraph executes a query and transforms the results into a D3 graph JSON.
// It also optionally hydrates the nodes with source code.
func (s *GraphService) ExportGraph(ctx context.Context, projectID, query string, hydrate bool) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// 1. Execute Query
	results, err := store.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	// 2. Transform to D3
	transformer := export.NewD3Transformer(store)
	graph, err := transformer.Transform(ctx, query, results)
	if err != nil {
		return nil, fmt.Errorf("%w: transformer failed: %v", errors.ErrInternal, err)
	}

	// 3. Hydrate if requested
	if hydrate && len(graph.Nodes) > 0 {
		if err := s.enrichNodes(ctx, store, graph); err != nil {
			// Log warning but return graph?
			// For service layer, we should return error or handle partials.
			// Let's return error to be explicit.
			return nil, fmt.Errorf("%w: hydration failed: %v", errors.ErrInternal, err)
		}
	}

	return graph, nil
}

// enrichNodes populates node content and kind from the store.
func (s *GraphService) enrichNodes(ctx context.Context, store *meb.MEBStore, graph *export.D3Graph) error {
	ids := make([]meb.DocumentID, len(graph.Nodes))
	for i, n := range graph.Nodes {
		ids[i] = meb.DocumentID(n.ID)
	}

	hydrated, err := store.Hydrate(ctx, ids)
	if err != nil {
		return err
	}

	hMap := make(map[meb.DocumentID]meb.HydratedSymbol)
	for _, h := range hydrated {
		hMap[h.ID] = h
	}

	for i := range graph.Nodes {
		n := &graph.Nodes[i]
		if h, ok := hMap[meb.DocumentID(n.ID)]; ok {
			n.Code = h.Content
			if h.Kind != "" {
				n.Kind = h.Kind
			}
			n.Children = s.mapChildren(h.Children)
		}
	}
	return nil
}

func (s *GraphService) mapChildren(hydrated []meb.HydratedSymbol) []export.D3Node {
	if len(hydrated) == 0 {
		return nil
	}
	nodes := make([]export.D3Node, len(hydrated))
	for i, h := range hydrated {
		// Fix Name generation first
		parts := strings.Split(string(h.ID), "/")
		name := parts[len(parts)-1]

		nodes[i] = export.D3Node{
			ID:       string(h.ID),
			Name:     name,
			Kind:     h.Kind,
			Code:     h.Content,
			Children: s.mapChildren(h.Children),
		}

		// Map Metadata if needed (Language/Group) - HydratedSymbol has Metadata map.
		if lang, ok := h.Metadata["language"].(string); ok {
			nodes[i].Language = lang
			nodes[i].Group = lang
		}
	}
	return nodes
}

// GetSource returns the content of a specific file/symbol.
func (s *GraphService) GetSource(projectID, docID string) (string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return "", err
	}

	doc, err := store.GetDocument(meb.DocumentID(docID))
	if err != nil {
		// Assumptions regarding GetDocument error
		return "", fmt.Errorf("%w: document not found", errors.ErrNotFound)
	}

	return string(doc.Content), nil
}

// Helper to get store with error mapping
func (s *GraphService) getStore(projectID string) (*meb.MEBStore, error) {
	if projectID == "" {
		return nil, fmt.Errorf("%w: missing project ID", errors.ErrInvalidInput)
	}
	store, err := s.manager.GetStore(projectID)
	if err != nil {
		// Heuristic to detect "not found" vs internal error
		sErr := err.Error()
		if os.IsNotExist(err) || sErr == fmt.Sprintf("project not found: %s", projectID) || strings.Contains(sErr, "not found") {
			return nil, fmt.Errorf("%w: %v", errors.ErrNotFound, err)
		}
		return nil, fmt.Errorf("%w: %v", errors.ErrInternal, err)
	}
	return store, nil
}

// GenerateSummary generates a project summary.
func (s *GraphService) GenerateSummary(projectID string) (*repl.ProjectSummary, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}
	// Recalculate stats on demand? Optional.
	return repl.GenerateProjectSummary(store)
}

// GetPredicates returns known predicates.
func (s *GraphService) GetPredicates(projectID string) ([]map[string]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	preds, err := store.GetAllPredicates()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInternal, err)
	}

	var results []map[string]string
	for _, p := range preds {
		if m, ok := meb.SystemPredicates[p]; ok {
			results = append(results, map[string]string{
				"name":        p,
				"description": m.Description,
				"example":     m.Example,
			})
		}
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
		limit = 50
	}

	return store.SearchSymbols(query, limit, predicate)
}

// ListFiles returns all ingested file paths for a project.
func (s *GraphService) ListFiles(projectID string) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// Query for all files that have a hash
	// Use explicit predicate string to avoid depending on meb package details here if possible,
	// but meb.PredHash is cleaner.
	q := fmt.Sprintf("triples(?f, \"%s\", ?h)", meb.PredHash)
	results, err := store.Query(context.Background(), q)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInternal, err)
	}

	files := make([]string, 0, len(results))
	for _, res := range results {
		if f, ok := res["?f"].(string); ok {
			// clean quotes if any (datalog sometimes returns quoted strings if not careful,
			// but here they should be DocumentIDs which are strings)
			f = strings.Trim(f, "\"")
			files = append(files, f)
		}
	}
	// Sort for consistent output
	// Not strictly necessary but good for UI
	return files, nil
}

// GetFileGraph returns a composite graph for a specific file (Defines + Imports + Calls).
func (s *GraphService) GetFileGraph(ctx context.Context, projectID, fileID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// Clean fileID (remove quotes if present, though handler should pass clean string)
	cleanFileID := strings.Trim(fileID, "\"")
	quotedFileID := fmt.Sprintf("\"%s\"", cleanFileID)

	// We will collect all results and then transform.
	// Actually, D3Transformer expects a single query string for parsing "triples" atom.
	// But here we present a unified view.
	// Hack: We can construct a synthetic result set and a dummy query for the transformer,
	// OR we can manually merge D3Graphs. Merging D3Graphs is safer.

	var mergedGraph *export.D3Graph = &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}

	// Map for link deduplication
	linkMap := make(map[string]bool)

	// Helper to merge
	merge := func(query string) error {
		results, err := store.Query(ctx, query)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return nil
		}

		subGraph, err := export.ExportD3(ctx, store, query, results)
		if err != nil {
			return err
		}

		// Merge Nodes (deduplicate by ID)
		nodeMap := make(map[string]export.D3Node)
		for _, n := range mergedGraph.Nodes {
			nodeMap[n.ID] = n
		}
		for _, n := range subGraph.Nodes {
			if _, exists := nodeMap[n.ID]; !exists {
				nodeMap[n.ID] = n
				mergedGraph.Nodes = append(mergedGraph.Nodes, n)
			}
		}

		// Merge Links with Deduplication
		for _, l := range subGraph.Links {
			// Create a unique key for the link
			key := fmt.Sprintf("%s-%s-%s", l.Source, l.Relation, l.Target)
			if _, exists := linkMap[key]; !exists {
				linkMap[key] = true
				mergedGraph.Links = append(mergedGraph.Links, l)
			}
		}
		return nil
	}

	// 1. Defines: triples("file", "defines", ?s)
	q1 := fmt.Sprintf("triples(%s, \"defines\", ?s)", quotedFileID)
	if err := merge(q1); err != nil {
		return nil, fmt.Errorf("failed to get definitions: %w", err)
	}

	// 2. Imports: triples("file", "imports", ?t)
	q2 := fmt.Sprintf("triples(%s, \"imports\", ?t)", quotedFileID)
	if err := merge(q2); err != nil {
		return nil, fmt.Errorf("failed to get imports: %w", err)
	}

	// 3. Calls: triples(?s, "calls", ?t), triples("file", "defines", ?s)
	// This finds calls originating from symbols defined in this file.
	q3 := fmt.Sprintf("triples(?s, \"calls\", ?t), triples(%s, \"defines\", ?s)", quotedFileID)
	if err := merge(q3); err != nil {
		return nil, fmt.Errorf("failed to get calls: %w", err)
	}

	// 4. Hydrate
	if len(mergedGraph.Nodes) > 0 {
		if err := s.enrichNodes(ctx, store, mergedGraph); err != nil {
			return nil, fmt.Errorf("hydration failed: %w", err)
		}
	}

	return mergedGraph, nil
}
