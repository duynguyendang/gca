package meb

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
)

// HydratedSymbol represents a symbol with both its relational facts and raw content.
type HydratedSymbol struct {
	ID       DocumentID     `json:"id"`
	Kind     string         `json:"kind"` // From Facts
	Content  string         `json:"code"` // From DocStore
	Metadata map[string]any `json:"meta"` // From DocStore
}

// Hydrate fetches content and metadata for a list of document IDs, parallelizing the I/O.
func (m *MEBStore) Hydrate(ctx context.Context, ids []DocumentID) ([]HydratedSymbol, error) {
	if len(ids) == 0 {
		return []HydratedSymbol{}, nil
	}

	results := make([]HydratedSymbol, len(ids))
	g, ctx := errgroup.WithContext(ctx)

	// We can process in chunks or just parallelize per ID.
	// Given BadgerDB is local, heavy parallelism might contend, but let's stick to errgroup.
	// We'll limit concurrency to avoid overwhelming if list is huge.
	g.SetLimit(10)

	var mu sync.Mutex

	for i, id := range ids {
		i, id := i, id // capture loop variables
		g.Go(func() error {
			// 1. Fetch from DocStore (using public API)
			doc, err := m.GetDocument(id)
			if err != nil {
				// If not found, maybe just partial result? Or error?
				// For now, let's assume valid IDs or just return partial data.
				// If error is "not found", we might want to continue.
				// But GetDocument currently (I need to check if it exists)
				// Assuming m.GetDocument exists or I need to implement it.
				// m.AddDocument exists. I probably need m.GetDocument.
				// Let's implement m.GetDocument in content.go if missing.
				return fmt.Errorf("failed to get document %s: %w", id, err)
			}

			// 2. Fetch Kind from Facts
			// We can use m.Query or specific lookup.
			// triples(ID, "type", ?k)
			// Assuming we have a helper or just query.
			kind := ""
			// This query inside a loop might be slow. We might want to batch query kinds first?
			// But for now, let's do it simply.
			// Using Scan for exact match on Subject=ID and Predicate="type"
			// But ScanContext takes strings.
			// We need to be careful about locking if multiple goroutines access Scan.
			// Badger transactions are thread-safe usually?
			// m.ScanContext creates a new transaction.

			// Note: If we are inside a read transaction, we should reuse it, but here we are top level.
			// ScanContext creates its own txn.
			for fact := range m.ScanContext(ctx, string(id), PredType, "", "") {
				if k, ok := fact.Object.(string); ok {
					kind = k
					break
				}
			}

			hydrated := HydratedSymbol{
				ID:       id,
				Kind:     kind,
				Content:  string(doc.Content),
				Metadata: doc.Metadata,
			}

			mu.Lock()
			results[i] = hydrated
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}
