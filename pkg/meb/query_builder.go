package meb

import (
	"fmt"
	"iter"
	"strconv"

	"github.com/duynguyendang/gca/pkg/meb/utils"
	"github.com/duynguyendang/gca/pkg/meb/vector"
)

const (
	// DefaultCandidateMultiplier is used to fetch more candidates from vector search
	// to account for graph filtering. We fetch limit * multiplier candidates.
	DefaultCandidateMultiplier = 10
)

// Filter represents a graph filter constraint.
type GraphFilter struct {
	Predicate string
	Object    interface{} // string, int, float64, etc.
	Graph     string      // Graph context for this filter. Defaults to "default".
}

// Store defines the interface required by the query builder.
// This avoids circular import dependencies.
type Store interface {
	Vectors() *vector.VectorRegistry
	Scan(s, p, o, g string) iter.Seq2[Fact, error]
	GetContent(id uint64) ([]byte, error)
}

// Builder provides a fluent API for neuro-symbolic search.
// It combines vector similarity search with graph filtering.
type Builder struct {
	store               Store
	vectorQuery         []float32
	threshold           float32 // Minimum similarity threshold (0-1)
	filters             []GraphFilter
	graph               string // Default graph for queries
	limit               int
	candidateMultiplier int
}

// NewBuilder creates a new query builder.
func NewBuilder(store Store) *Builder {
	return &Builder{
		store:               store,
		graph:               "default", // Default graph context
		limit:               10,        // Default limit
		candidateMultiplier: DefaultCandidateMultiplier,
	}
}

// SimilarTo sets the vector similarity query.
// The vector must be 1536-dimensional (OpenAI embedding standard).
// Optionally accepts a threshold (0-1) as the second argument.
func (b *Builder) SimilarTo(vec []float32, threshold ...float32) *Builder {
	b.vectorQuery = vec
	if len(threshold) > 0 {
		b.threshold = threshold[0]
	}
	return b
}

// Where adds a graph filter constraint.
// The filter requires that a fact matching predicate(subject, object) exists.
func (b *Builder) Where(predicate string, object interface{}) *Builder {
	b.filters = append(b.filters, GraphFilter{
		Predicate: predicate,
		Object:    object,
		Graph:     b.graph, // Use current graph context
	})
	return b
}

// WhereIn adds a graph filter constraint for a specific graph context.
// This allows filtering facts from different graphs in the same query.
func (b *Builder) WhereIn(graph, predicate string, object interface{}) *Builder {
	b.filters = append(b.filters, GraphFilter{
		Predicate: predicate,
		Object:    object,
		Graph:     graph,
	})
	return b
}

// Graph sets the default graph context for the query.
func (b *Builder) Graph(graph string) *Builder {
	b.graph = graph
	return b
}

// Limit sets the maximum number of results to return.
func (b *Builder) Limit(n int) *Builder {
	b.limit = n
	return b
}

// CandidateMultiplier sets how many candidates to fetch from vector search
// relative to the limit. Higher values allow for more aggressive filtering.
// Default is 10.
func (b *Builder) CandidateMultiplier(multiplier int) *Builder {
	b.candidateMultiplier = multiplier
	return b
}

// Execute runs the neuro-symbolic query using the Neuro-First strategy:
// 1. Run vector search to get candidate IDs
// 2. Filter candidates through graph constraints (using Scan)
// 3. Return hydrated results with content
func (b *Builder) Execute() ([]Result, error) {
	// Validate: Must have vector query
	if len(b.vectorQuery) == 0 {
		return nil, fmt.Errorf("query must include SimilarTo() vector search")
	}

	// Step 1: Run vector search to get candidates
	// Fetch more candidates to account for graph filtering
	candidateK := b.limit * b.candidateMultiplier
	if candidateK < 100 {
		candidateK = 100 // Minimum candidate pool
	}

	vectorResults, err := b.store.Vectors().Search(b.vectorQuery, candidateK)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// Step 2: Filter candidates through graph constraints using Scan
	results := make([]Result, 0, b.limit)

	// Pre-compile filters to avoid repeated fmt.Sprintf in the loop
	compiledFilters := make([]compiledFilter, len(b.filters))
	for i, f := range b.filters {
		compiledFilters[i] = compiledFilter{
			Predicate: f.Predicate,
			Object:    fmt.Sprintf("%v", f.Object),
			Graph:     f.Graph,
		}
	}

	for _, vecResult := range vectorResults {
		// Apply threshold filter
		if b.threshold > 0 && vecResult.Score < b.threshold {
			continue
		}

		// Convert ID to string for Scan
		// Optimization: Use buffer to avoid allocations in hot path
		var buf [32]byte
		keyBuf := append(buf[:0], "id:"...)
		keyBuf = strconv.AppendUint(keyBuf, vecResult.ID, 10)
		candidateKey := string(keyBuf)

		// Apply graph filters using Scan for each filter
		if b.matchesFilters(candidateKey, compiledFilters) {
			// Hydrate result
			contentBytes, err := b.store.GetContent(vecResult.ID)
			contentStr := ""
			if err == nil && contentBytes != nil {
				// Convert bytes to string without copying using unsafe util
				contentStr = utils.BytesToString(contentBytes)
			}
			// Ignore errors - content is optional

			results = append(results, Result{
				ID:      vecResult.ID,
				Key:     candidateKey,
				Score:   vecResult.Score,
				Content: contentStr,
			})

			// Stop when we reach limit
			if len(results) >= b.limit {
				break
			}
		}
	}

	return results, nil
}

// compiledFilter is an optimized internal representation of GraphFilter
type compiledFilter struct {
	Predicate string
	Object    string
	Graph     string
}

// matchesFilters checks if a candidate (by key) matches all graph filters.
// Uses the Scan API to check for facts matching the filter constraints.
func (b *Builder) matchesFilters(candidateKey string, filters []compiledFilter) bool {
	for _, filter := range filters {
		// Use Scan to check if fact exists: Scan(candidateKey, filter.Predicate, filter.Object, filter.Graph)
		matched := false

		// We just need to find ONE matching fact to satisfy the filter
		for _, err := range b.store.Scan(candidateKey, filter.Predicate, filter.Object, filter.Graph) {
			if err != nil {
				return false
			}
			// Found at least one matching fact
			matched = true
			break
		}

		if !matched {
			return false
		}
	}

	return true
}
