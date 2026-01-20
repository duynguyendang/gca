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
		}
	}
	return nil
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
