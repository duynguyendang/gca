package graph

import (
	"context"
	"fmt"
	"iter"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/vector"
)

// KnowledgeGraph provides a higher-level interface over the MEB store.
type KnowledgeGraph struct {
	store *meb.MEBStore
}

// NewKnowledgeGraph creates a new KnowledgeGraph wrapper around the given store.
func NewKnowledgeGraph(store *meb.MEBStore) *KnowledgeGraph {
	return &KnowledgeGraph{
		store: store,
	}
}

// AddFact adds a single fact to the store.
func (kg *KnowledgeGraph) AddFact(subject, predicate string, object any) error {
	fact := meb.Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
	}
	return kg.store.AddFact(fact)
}

// AddFacts adds multiple facts to the store in a batch.
func (kg *KnowledgeGraph) AddFacts(facts []meb.Fact) error {
	return kg.store.AddFactBatch(facts)
}

// ScanOptions defines options for scanning facts.
type ScanOptions struct {
	Subject   string
	Predicate string
	Object    string
}

// Scan returns facts matching the given options.
func (kg *KnowledgeGraph) Scan(opts ScanOptions) ([]meb.Fact, error) {
	var results []meb.Fact

	iter := kg.store.Scan(
		opts.Subject,
		opts.Predicate,
		opts.Object,
	)

	for fact, err := range iter {
		if err != nil {
			return results, err
		}
		results = append(results, fact)
	}

	return results, nil
}

// ScanContext returns facts matching the given options, with context support.
func (kg *KnowledgeGraph) ScanContext(ctx context.Context, opts ScanOptions) iter.Seq2[meb.Fact, error] {
	return kg.store.ScanContext(ctx, opts.Subject, opts.Predicate, opts.Object)
}

// SemanticSearchOptions defines options for semantic search.
type SemanticSearchOptions struct {
	Embedding []float32
	Threshold float32
	Limit     int
}

// SearchResult represents a single semantic search result.
type SearchResult struct {
	ID      uint64
	Score   float32
	Subject string
}

// SemanticSearch performs vector similarity search on stored embeddings.
func (kg *KnowledgeGraph) SemanticSearch(ctx context.Context, opts SemanticSearchOptions) ([]SearchResult, error) {
	embedding := opts.Embedding

	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding required for semantic search")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	// Vector search returns iter.Seq2[vector.SearchResult, error]
	vectorIter := kg.store.Vectors().Search(embedding, limit*10)

	var results []SearchResult
	for vr, err := range vectorIter {
		if err != nil {
			return nil, fmt.Errorf("vector search failed: %w", err)
		}

		if opts.Threshold > 0 && vr.Score < opts.Threshold {
			continue
		}

		subject, err := kg.store.ResolveID(vr.ID)
		if err != nil {
			continue
		}

		results = append(results, SearchResult{
			ID:      vr.ID,
			Score:   vr.Score,
			Subject: subject,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetContent retrieves the content associated with a symbol ID.
func (kg *KnowledgeGraph) GetContent(id string) ([]byte, error) {
	dictID, found := kg.store.LookupID(id)
	if !found {
		return nil, fmt.Errorf("id not found: %s", id)
	}
	return kg.store.GetContent(dictID)
}

// LookupID resolves a string ID to its dictionary ID.
func (kg *KnowledgeGraph) LookupID(id string) (uint64, bool) {
	return kg.store.LookupID(id)
}

// ResolveID resolves a dictionary ID back to its string representation.
func (kg *KnowledgeGraph) ResolveID(id uint64) (string, error) {
	return kg.store.ResolveID(id)
}

// Vectors returns the vector registry for direct access.
func (kg *KnowledgeGraph) Vectors() *vector.VectorRegistry {
	return kg.store.Vectors()
}

// Store returns the underlying MEB store.
func (kg *KnowledgeGraph) Store() *meb.MEBStore {
	return kg.store
}

// GetFactsForSubject returns all facts with the given subject.
func (kg *KnowledgeGraph) GetFactsForSubject(subject string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Subject: subject,
	})
}

// GetFactsByPredicate returns all facts with the given predicate.
func (kg *KnowledgeGraph) GetFactsByPredicate(predicate string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Predicate: predicate,
	})
}

// GetIncomingEdges returns all facts where the given subject is the object.
func (kg *KnowledgeGraph) GetIncomingEdges(subject string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Object: subject,
	})
}

// GetOutgoingEdges returns all facts where the given subject is the subject.
func (kg *KnowledgeGraph) GetOutgoingEdges(subject string) ([]meb.Fact, error) {
	return kg.Scan(ScanOptions{
		Subject: subject,
	})
}
