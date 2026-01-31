package meb

import (
	"context"
	"fmt"
	"strings"
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

// HydrateShallow fetches metadata for a list of document IDs without recursing into children.
// This is optimized for bulk operations like backbone generation where children are not needed.
// It also accepts a lazy flag to skip content fetching.
func (m *MEBStore) HydrateShallow(ctx context.Context, ids []DocumentID, lazy bool) ([]HydratedSymbol, error) {
	if len(ids) == 0 {
		return []HydratedSymbol{}, nil
	}

	results := make([]HydratedSymbol, len(ids))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	var mu sync.Mutex

	for i, id := range ids {
		i, id := i, id
		g.Go(func() error {
			sym, err := m.hydrateOne(ctx, id, lazy, true) // shallow=true
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

// Hydrate fetches content and metadata for a list of document IDs, parallelizing the I/O.
// If lazy is true, it skips fetching large content bodies and only returns metadata/structure.
func (m *MEBStore) Hydrate(ctx context.Context, ids []DocumentID, lazy bool) ([]HydratedSymbol, error) {
	if len(ids) == 0 {
		return []HydratedSymbol{}, nil
	}

	results := make([]HydratedSymbol, len(ids))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	var mu sync.Mutex

	for i, id := range ids {
		i, id := i, id
		g.Go(func() error {
			sym, err := m.hydrateOne(ctx, id, lazy, false) // shallow=false
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

// hydrateOne hydrates a single symbol.
// shallow=true skips recursive child fetching.
func (m *MEBStore) hydrateOne(ctx context.Context, id DocumentID, lazy bool, shallow bool) (HydratedSymbol, error) {
	// 1. Fetch details
	var doc *Document
	var err error

	if lazy {
		doc, err = m.GetDocumentMetadata(id)
	} else {
		doc, err = m.GetDocument(id)
	}

	if err != nil {
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

	// 3. Fetch Children (recursive "defines") - SKIP if shallow
	var children []HydratedSymbol
	if !shallow {
		// Scan for triples(id, "defines", ?child)
		for fact := range m.ScanContext(ctx, string(id), "defines", "", "") {
			if childIDStr, ok := fact.Object.(string); ok {
				// Recursive call with same flags (propagate shallow? usually children are fetched fully if parent is fetched fully?
				// Actually original code was always recursive.
				// If we are deep fetching, we probably want deep children.
				childSym, err := m.hydrateOne(ctx, DocumentID(childIDStr), lazy, shallow)
				if err != nil {
					continue
				}
				children = append(children, childSym)
			}
		}
	}

	content := ""
	if doc.Content != nil {
		content = string(doc.Content)
	}

	// 4. If content is empty but we have line metadata, extract snippet from parent file
	if content == "" && !lazy && doc.Metadata != nil {
		startLine, hasStart := doc.Metadata["start_line"]
		endLine, hasEnd := doc.Metadata["end_line"]
		fileID, hasFile := doc.Metadata["file"]

		if hasStart && hasEnd && hasFile {
			startLineInt, okStart := toInt(startLine)
			endLineInt, okEnd := toInt(endLine)
			fileIDStr, okFile := fileID.(string)

			if okStart && okEnd && okFile && startLineInt > 0 && endLineInt >= startLineInt {
				// Fetch parent file content
				parentDoc, parentErr := m.GetDocument(DocumentID(fileIDStr))
				if parentErr == nil && parentDoc.Content != nil {
					// Extract lines startLineInt to endLineInt (1-indexed)
					content = extractLines(string(parentDoc.Content), startLineInt, endLineInt)
				}
			}
		}
	}

	return HydratedSymbol{
		ID:       id,
		Kind:     kind,
		Content:  content,
		Metadata: doc.Metadata,
		Children: children,
	}, nil
}

// toInt converts various numeric types to int
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		// Parse string as int (for JSON-serialized values)
		var i int
		_, err := fmt.Sscanf(n, "%d", &i)
		return i, err == nil
	default:
		return 0, false
	}
}

// extractLines extracts lines from content (1-indexed, inclusive)
func extractLines(content string, startLine, endLine int) string {
	lines := strings.Split(content, "\n")
	if startLine < 1 || endLine > len(lines) {
		return ""
	}
	// Convert to 0-indexed
	start := startLine - 1
	end := endLine
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}
