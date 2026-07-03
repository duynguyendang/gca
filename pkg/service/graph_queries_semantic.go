package service

import (
	"context"
	"fmt"
	"strings"
)

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

	results := make([]SemanticSearchResult, 0, k)

	vecIter := store.Vectors().Search(ctx, embedding, k)
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

// SemanticSearchFiltered performs vector similarity search with graph predicate filtering.
func (s *GraphService) SemanticSearchFiltered(ctx context.Context, projectID, query string, k int, predicate string, object string, gemini interface {
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

	builder := store.Find().
		SimilarTo(embedding).
		Limit(k)

	if predicate != "" {
		builder = builder.Where(predicate, object)
	}

	queryResults, err := builder.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("query builder execution failed: %w", err)
	}

	results := make([]SemanticSearchResult, 0, len(queryResults))
	for _, qr := range queryResults {
		name := qr.Key
		if parts := strings.Split(qr.Key, ":"); len(parts) > 1 {
			name = parts[len(parts)-1]
		}
		results = append(results, SemanticSearchResult{
			SymbolID: qr.Key,
			Score:    qr.Score,
			Name:     name,
		})
	}

	return results, nil
}
