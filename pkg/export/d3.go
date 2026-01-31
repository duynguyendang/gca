package export

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/duynguyendang/gca/pkg/datalog"
	"github.com/duynguyendang/gca/pkg/meb"
)

// D3Node represents a node in the D3 force-directed graph.
type D3Node struct {
	ID         string            `json:"id"`                    // Full absolute path (unique identifier)
	Name       string            `json:"name"`                  // Display name (filename:symbol)
	Kind       string            `json:"kind,omitempty"`        // e.g. "func", "struct", "interface"
	Language   string            `json:"language,omitempty"`    // e.g. "go", "typescript"
	Group      string            `json:"group,omitempty"`       // Grouping for visualization (uses Language)
	Code       string            `json:"code,omitempty"`        // Source code snippet
	Children   []D3Node          `json:"children,omitempty"`    // Recursive children
	ParentID   string            `json:"parentId,omitempty"`    // ID of the parent file (for drilling down)
	IsInternal *bool             `json:"is_internal,omitempty"` // True if node is internal to the project
	Metadata   map[string]string `json:"metadata,omitempty"`    // Extra data (e.g. docs)
}

// D3Link represents a link/edge in the D3 force-directed graph.
type D3Link struct {
	Source           string  `json:"source"`
	Target           string  `json:"target"`
	Relation         string  `json:"relation"`
	Weight           float64 `json:"weight,omitempty"`
	Type             string  `json:"type"`                 // "ast" or "virtual"
	SourceProvenance string  `json:"provenance,omitempty"` // Renamed to avoid collision with Source field
}

// D3Graph represents the full graph structure for D3.js.
type D3Graph struct {
	Nodes []D3Node `json:"nodes"`
	Links []D3Link `json:"links"`
}

// D3Transformer handles the conversion of query results to D3 graph format.
type D3Transformer struct {
	IgnoredPredicates map[string]bool
	Store             *meb.MEBStore
	ExcludeTestFiles  bool
	InternalPrefixes  []string // Prefixes that identify internal project files
}

// NewD3Transformer creates a new transformer with reference to the store.
func NewD3Transformer(store *meb.MEBStore) *D3Transformer {
	t := &D3Transformer{
		IgnoredPredicates: map[string]bool{
			"source_code": true,
			"line_number": true,
			"start_line":  true,
			"end_line":    true,
		},
		Store:            store,
		InternalPrefixes: []string{},
	}

	// Try to detect internal prefixes from the store
	// For Go projects, we look for files that have been ingested
	// All ingested files are considered internal
	t.detectInternalPrefixes()

	return t
}

// detectInternalPrefixes identifies internal module/package prefixes from ingested files
func (t *D3Transformer) detectInternalPrefixes() {
	// Scan for files with hash (ingested files)
	// These are all internal to the project
	prefixSet := make(map[string]bool)

	for fact, _ := range t.Store.Scan("", meb.PredHash, "", "") {
		filePath := string(fact.Subject)

		// Extract package prefix (first part before colon if symbol, or directory)
		parts := strings.SplitN(filePath, ":", 2)
		basePath := parts[0]

		// Add the file path itself
		prefixSet[basePath] = true

		// Add parent directories as prefixes too
		for i := len(basePath) - 1; i >= 0; i-- {
			if basePath[i] == '/' {
				prefixSet[basePath[:i]] = true
			}
		}
	}

	for prefix := range prefixSet {
		t.InternalPrefixes = append(t.InternalPrefixes, prefix)
	}
}

// Transform converts datalog query results into a D3Graph.
func (t *D3Transformer) Transform(ctx context.Context, query string, results []map[string]any) (*D3Graph, error) {
	// Short-circuit if no results to transform
	if len(results) == 0 {
		return &D3Graph{Nodes: []D3Node{}, Links: []D3Link{}}, nil
	}

	// Parse the query to find the 'triples' atom
	atoms, err := datalog.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query for export: %w", err)
	}

	var triplesAtom *datalog.Atom
	for _, atom := range atoms {
		if atom.Predicate == "triples" {
			triplesAtom = &atom
			break
		}
	}

	if triplesAtom == nil {
		return nil, fmt.Errorf("query must contain a 'triples' predicate to be exported")
	}

	if len(triplesAtom.Args) != 3 {
		return nil, fmt.Errorf("triples predicate must have 3 arguments")
	}

	sArg, pArg, oArg := triplesAtom.Args[0], triplesAtom.Args[1], triplesAtom.Args[2]

	nodesMap := make(map[string]D3Node)
	var links []D3Link

	// Helper to resolve argument value (variable or constant)
	resolve := func(arg string, row map[string]any) string {
		// If variable (starts with ? or Uppercase)
		if strings.HasPrefix(arg, "?") || (len(arg) > 0 && arg[0] >= 'A' && arg[0] <= 'Z') {
			if val, ok := row[arg]; ok {
				return fmt.Sprintf("%v", val)
			}
			return "" // Should not happen if binding worked
		}
		// Constant, strip quotes
		return strings.Trim(arg, "\"'")
	}

	for _, row := range results {
		sVal := resolve(sArg, row)
		pVal := resolve(pArg, row)
		oVal := resolve(oArg, row)

		if sVal == "" || oVal == "" {
			continue
		}

		// Filter unwanted predicates
		if t.IgnoredPredicates[pVal] {
			continue
		}

		// Filter test files if requested
		if t.ExcludeTestFiles {
			if strings.Contains(sVal, "_test.go") || strings.Contains(oVal, "_test.go") {
				continue
			}
		}

		// Metadata Handling (Docs, Comments)
		// Instead of creating nodes for these, attach them to the Subject Node's Metadata
		if pVal == "has_doc" || pVal == "has_comment" {
			// Ensure Subject exists
			if _, exists := nodesMap[sVal]; !exists {
				nodesMap[sVal] = t.createNode(sVal)
			}
			node := nodesMap[sVal]
			if node.Metadata == nil {
				node.Metadata = make(map[string]string)
			}
			// Use predicate as key (e.g. "has_doc")
			node.Metadata[pVal] = oVal
			nodesMap[sVal] = node // Update map struct copy
			continue              // Skip creating Object Node and Link
		}

		// SAFETY: Skip literal text nodes (newlines or very long strings) if they weren't caught above
		if strings.Contains(sVal, "\n") || len(sVal) > 200 || strings.Contains(oVal, "\n") || len(oVal) > 200 {
			continue
		}

		// Add Subject Node
		if _, exists := nodesMap[sVal]; !exists {
			nodesMap[sVal] = t.createNode(sVal)
		}

		// Add Object Node
		if _, exists := nodesMap[oVal]; !exists {
			nodesMap[oVal] = t.createNode(oVal)
		}

		// Extract metadata
		var weight float64 = 1.0
		if w, ok := row["_weight"].(float64); ok {
			weight = w
		}

		var provenance string = "ast"
		if s, ok := row["_source"].(string); ok {
			provenance = s
		}

		// Add Link
		linkType := "ast"
		if provenance == "virtual" || provenance == "inference" {
			linkType = "virtual"
		}

		links = append(links, D3Link{
			Source:           sVal,
			Target:           oVal,
			Relation:         pVal,
			Weight:           weight,
			Type:             linkType,
			SourceProvenance: provenance,
		})
	}

	// Convert map to slice
	var nodes []D3Node
	for _, n := range nodesMap {
		nodes = append(nodes, n)
	}

	return &D3Graph{
		Nodes: nodes,
		Links: links,
	}, nil
}

// createNode builds a D3Node with enriched metadata.
func (t *D3Transformer) createNode(id string) D3Node {
	// Debug logging for node creation
	if strings.Contains(id, "_test.go") {
		fmt.Printf("[D3Transformer] Creating node for test file: %s\n", id)
	}
	displayName := t.generateDisplayName(id)
	kind, language, code := t.getMetadata(id)

	// Default group to language, fallback to "unknown"
	group := language
	if group == "" {
		group = "unknown"
	}

	// Determine if this node is internal to the project
	isInternal := t.isInternalNode(id)

	return D3Node{
		ID:         id,
		Name:       displayName,
		Kind:       kind,
		Language:   language,
		Group:      group,
		Code:       code,
		IsInternal: &isInternal,
	}
}

// isInternalNode checks if a node ID belongs to the internal project
func (t *D3Transformer) isInternalNode(id string) bool {
	// Extract the file path part (before colon if symbol)
	parts := strings.SplitN(id, ":", 2)
	basePath := parts[0]

	// Check if the file exists in the store (was ingested)
	// This is the most reliable way to detect internal files
	doc, err := t.Store.GetDocument(meb.DocumentID(basePath))
	if err == nil && len(doc.Content) > 0 {
		return true
	}

	// Check against known internal prefixes
	for _, prefix := range t.InternalPrefixes {
		if strings.HasPrefix(basePath, prefix) || basePath == prefix {
			return true
		}
	}

	// External indicators:
	// - Go stdlib (no dots, no slashes in package name for simple cases like "fmt")
	// - Looks like external import path but NOT in our project
	if !strings.Contains(basePath, "/") && !strings.Contains(basePath, ".") {
		// Simple name like "fmt", "strings", "errors" = stdlib
		return false
	}

	// If we can't determine, assume external (safer for the frontend)
	return false
}

// generateDisplayName creates a human-readable label (filename:symbol).
func (t *D3Transformer) generateDisplayName(id string) string {
	// ID format: /path/to/file.go:Symbol or /path/to/file.go
	// We want: file.go:Symbol

	// Split by '/' to get the last segment (filename...)
	parts := strings.Split(id, "/")
	lastPart := parts[len(parts)-1]

	return lastPart
}

// getMetadata fetches has_kind, has_language, and has_source_code from the store.
func (t *D3Transformer) getMetadata(id string) (string, string, string) {
	kind := ""
	language := ""
	code := ""

	// We use the store to scan for specific metadata predicates attached to this ID
	// Note: This performs individual scans per node. For massive exports, batching would be better,
	// but Scan is efficient enough for typical export sizes.

	// 1. Check for 'has_kind'
	for fact, _ := range t.Store.Scan(id, "has_kind", "", "") {
		if str, ok := fact.Object.(string); ok {
			kind = str
			break // Take the first one
		}
	}

	// 2. Check for 'has_language'
	for fact, _ := range t.Store.Scan(id, "has_language", "", "") {
		if str, ok := fact.Object.(string); ok {
			language = str
			break
		}
	}

	// 3. Get Source Code from DocStore (instead of FactStore)
	doc, err := t.Store.GetDocument(meb.DocumentID(id))
	if err == nil && len(doc.Content) > 0 {
		code = string(doc.Content)
	}

	// Fallback: Infer language from file extension if not found in DB
	if language == "" {
		if strings.Contains(id, ".go") {
			language = "go"
		} else if strings.Contains(id, ".ts") {
			language = "typescript"
		} else if strings.Contains(id, ".js") {
			language = "javascript"
		} else if strings.Contains(id, ".py") {
			language = "python"
		}
	}

	return kind, language, code
}

// ExportD3 is a convenience wrapper for D3Transformer.
func ExportD3(ctx context.Context, store *meb.MEBStore, query string, results []map[string]any) (*D3Graph, error) {
	transformer := NewD3Transformer(store)
	return transformer.Transform(ctx, query, results)
}

// SaveD3Graph writes the graph to a JSON file.
func SaveD3Graph(graph *D3Graph, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(graph)
}
