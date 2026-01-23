package meb

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
)

// HydratedSymbol represents a symbol with both its relational facts and raw content.
type HydratedSymbol struct {
	ID       DocumentID       `json:"id"`
	Kind     string           `json:"kind"` // From Facts
	Content  string           `json:"code"` // From DocStore
	Metadata map[string]any   `json:"meta"` // From DocStore
	Children []HydratedSymbol `json:"children,omitempty"`
}

// Hydrate fetches content and metadata for a list of document IDs, parallelizing the I/O.
// If lazy is true, it skips fetching large content bodies and only returns metadata/structure.
func (m *MEBStore) Hydrate(ctx context.Context, ids []DocumentID, lazy bool) ([]HydratedSymbol, error) {
	if len(ids) == 0 {
		return []HydratedSymbol{}, nil
	}

	// We'll limit concurrency to avoid overwhelming if list is huge.
	// We use a shared semaphore via errgroup limit? No, errgroup limit is per group.
	// If we curse recursively, we might spawn too many goroutines.
	// For now, let's keep it simple: Top level parallel, children serial or limited.
	// Given the depth isn't usually huge, simple recursion might be okay, but
	// "defines" tree can be large. Let's stick to top-level parallelism for now.

	results := make([]HydratedSymbol, len(ids))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	var mu sync.Mutex

	for i, id := range ids {
		i, id := i, id
		g.Go(func() error {
			sym, err := m.hydrateOne(ctx, id, lazy)
			if err != nil {
				return err
			}
			mu.Lock()
			results[i] = sym
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// hydrateOne hydrates a single symbol and its children recursively.
func (m *MEBStore) hydrateOne(ctx context.Context, id DocumentID, lazy bool) (HydratedSymbol, error) {
	// 1. Fetch details
	var doc *Document
	var err error

	if lazy {
		doc, err = m.GetDocumentMetadata(id)
	} else {
		doc, err = m.GetDocument(id)
	}

	if err != nil {
		// If document missing, we might still want to return partial info if it exists in graph?
		// For now, fail as before.
		return HydratedSymbol{}, fmt.Errorf("failed to get document %s: %w", id, err)
	}

	// 2. Fetch Kind
	kind := ""
	for fact := range m.ScanContext(ctx, string(id), PredType, "", "") {
		if k, ok := fact.Object.(string); ok {
			kind = k
			break
		}
	}

	// 3. Fetch Children (recursive "defines")
	var children []HydratedSymbol
	// Scan for triples(id, "defines", ?child)
	for fact := range m.ScanContext(ctx, string(id), "defines", "", "") {
		if childIDStr, ok := fact.Object.(string); ok {
			// Check for cycles? "defines" implies strict hierarchy usually.
			// To be safe we could pass a visited map, but for now assuming DAG.
			// Recursive call
			childSym, err := m.hydrateOne(ctx, DocumentID(childIDStr), lazy)
			if err != nil {
				// Log warning? Or fail?
				// If a child fails to hydrate (defines a symbol that has no doc?), maybe skip it.
				continue
			}
			children = append(children, childSym)
		}
	}

	content := ""
	if doc.Content != nil {
		content = string(doc.Content)
	}

	return HydratedSymbol{
		ID:       id,
		Kind:     kind,
		Content:  content,
		Metadata: doc.Metadata,
		Children: children,
	}, nil
}
