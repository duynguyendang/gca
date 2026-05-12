package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

// HydratedSymbol replaces the removed meb.HydratedSymbol schema.
type HydratedSymbol struct {
	ID       string                 `json:"id"`
	Kind     string                 `json:"kind"`
	Content  string                 `json:"code"`
	Metadata map[string]interface{} `json:"metadata"`
	Children []HydratedSymbol       `json:"children,omitempty"`
}

// ProjectStoreManager interface abstraction
type ProjectStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
	ListProjects() ([]manager.ProjectMetadata, error)
}

// GraphService handles graph query and enrichment operations.
type GraphService struct {
	manager         ProjectStoreManager
	projectMapCache map[string]*export.D3Graph
	cacheMu         sync.RWMutex
}

// NewGraphService creates a new GraphService.
func NewGraphService(manager ProjectStoreManager) *GraphService {
	return &GraphService{
		manager:         manager,
		projectMapCache: make(map[string]*export.D3Graph),
	}
}

// ListProjects returns a list of available projects.
func (s *GraphService) ListProjects() ([]manager.ProjectMetadata, error) {
	return s.manager.ListProjects()
}

// ensureTimeout ensures the context has a timeout, wrapping if necessary
func ensureTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// ExportGraph executes a query and transforms the results into a D3 graph JSON.
// It also optionally hydrates the nodes with source code.
func (s *GraphService) ExportGraph(ctx context.Context, projectID, query string, hydrate bool, lazy bool, limit, offset int) (*export.D3Graph, error) {
	ctx, cancel := ensureTimeout(ctx, config.QueryTimeout)
	defer cancel()

	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// 1. Execute Query
	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = config.QueryResultLimit
	}

	// If offset is specified, fetch offset+limit rows then slice to get the correct page
	fetchLimit := effectiveLimit
	if offset > 0 {
		fetchLimit = effectiveLimit + offset
	}

	results, err := gcamdb.QueryWithLimit(ctx, store, query, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	// Apply offset to get the correct page
	if offset > 0 && offset < len(results) {
		end := offset + effectiveLimit
		if end > len(results) {
			end = len(results)
		}
		results = results[offset:end]
	} else if offset >= len(results) {
		results = []map[string]any{}
	}

	// 2. Transform to D3
	transformer := export.NewD3Transformer(store)
	graph, err := transformer.Transform(ctx, query, results)
	if err != nil {
		return nil, fmt.Errorf("%w: transformer failed: %v", errors.ErrInternal, err)
	}

	// 3. Hydrate if requested
	if hydrate && len(graph.Nodes) > 0 {
		if err := s.enrichNodes(ctx, store, graph, lazy); err != nil {
			return nil, fmt.Errorf("%w: hydration failed: %v", errors.ErrInternal, err)
		}
	}

	return graph, nil
}

// Helper to get store with error mapping
func (s *GraphService) getStore(projectID string) (*meb.MEBStore, error) {
	if projectID == "" {
		return nil, fmt.Errorf("%w: missing project ID", errors.ErrInvalidInput)
	}
	store, err := s.manager.GetStore(projectID)
	if err != nil {
		sErr := err.Error()
		if os.IsNotExist(err) || sErr == fmt.Sprintf("project not found: %s", projectID) || strings.Contains(sErr, "not found") {
			return nil, fmt.Errorf("%w: %v", errors.ErrNotFound, err)
		}
		return nil, fmt.Errorf("%w: %v", errors.ErrInternal, err)
	}
	return store, nil
}

// GetCentralityRanking returns symbols ranked by their graph centrality
func (s *GraphService) GetCentralityRanking(ctx context.Context, projectID string, limit int) ([]CentralityResult, error) {
	ctx, cancel := ensureTimeout(ctx, config.QueryTimeout)
	defer cancel()

	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	inDegree := make(map[string]int)
	outDegree := make(map[string]int)

	for fact := range store.Scan("", config.PredicateCalls, "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}

	for fact := range store.Scan("", config.PredicateImports, "") {
		if obj, ok := fact.Object.(string); ok {
			inDegree[obj]++
		}
		outDegree[fact.Subject]++
	}

	type scoredSymbol struct {
		symbol    string
		inDegree  int
		outDegree int
		score     float64
	}

	var symbols []scoredSymbol
	seen := make(map[string]bool)

	for sym := range inDegree {
		if !seen[sym] {
			seen[sym] = true
			symbols = append(symbols, scoredSymbol{
				symbol:    sym,
				inDegree:  inDegree[sym],
				outDegree: outDegree[sym],
				score:     float64(inDegree[sym] + outDegree[sym]),
			})
		}
	}
	for sym := range outDegree {
		if !seen[sym] {
			seen[sym] = true
			symbols = append(symbols, scoredSymbol{
				symbol:    sym,
				inDegree:  inDegree[sym],
				outDegree: outDegree[sym],
				score:     float64(inDegree[sym] + outDegree[sym]),
			})
		}
	}

	maxScore := 0.0
	for i := range symbols {
		boost := 1.0
		lower := strings.ToLower(symbols[i].symbol)
		if strings.Contains(lower, ":main") || strings.Contains(lower, ".main") ||
			strings.Contains(lower, ":init") || strings.Contains(lower, ".init") {
			boost = 2.5
		}
		if symbols[i].outDegree > 10 && symbols[i].inDegree > 5 {
			boost *= config.CentralityBoostHub
		}
		if IsInterfacePattern(symbols[i].symbol) {
			boost *= config.CentralityBoostInterface
		}
		symbols[i].score *= boost
		if symbols[i].score > maxScore {
			maxScore = symbols[i].score
		}
	}

	if maxScore > 0 {
		for i := range symbols {
			symbols[i].score /= maxScore
		}
	}

	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].score > symbols[j].score
	})

	if limit <= 0 {
		limit = 20
	}
	if limit > len(symbols) {
		limit = len(symbols)
	}

	results := make([]CentralityResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = CentralityResult{
			SymbolID:   symbols[i].symbol,
			Centrality: symbols[i].score,
			InDegree:   symbols[i].inDegree,
			OutDegree:  symbols[i].outDegree,
		}
	}

	return results, nil
}
