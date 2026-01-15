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
	ID       string `json:"id"`                 // Full absolute path (unique identifier)
	Name     string `json:"name"`               // Display name (filename:symbol)
	Kind     string `json:"kind,omitempty"`     // e.g. "func", "struct", "interface"
	Language string `json:"language,omitempty"` // e.g. "go", "typescript"
	Group    string `json:"group,omitempty"`    // Grouping for visualization (uses Language)
}

// D3Link represents a link/edge in the D3 force-directed graph.
type D3Link struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
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
}

// NewD3Transformer creates a new transformer with reference to the store.
func NewD3Transformer(store *meb.MEBStore) *D3Transformer {
	return &D3Transformer{
		IgnoredPredicates: map[string]bool{
			"source_code": true,
			"line_number": true,
			"start_line":  true,
			"end_line":    true,
		},
		Store: store,
	}
}

// Transform converts datalog query results into a D3Graph.
func (t *D3Transformer) Transform(ctx context.Context, query string, results []map[string]any) (*D3Graph, error) {
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

		// Add Subject Node
		if _, exists := nodesMap[sVal]; !exists {
			nodesMap[sVal] = t.createNode(sVal)
		}

		// Add Object Node
		if _, exists := nodesMap[oVal]; !exists {
			nodesMap[oVal] = t.createNode(oVal)
		}

		// Add Link
		links = append(links, D3Link{
			Source:   sVal,
			Target:   oVal,
			Relation: pVal,
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
	displayName := t.generateDisplayName(id)
	kind, language := t.getMetadata(id)

	// Default group to language, fallback to "unknown"
	group := language
	if group == "" {
		group = "unknown"
	}

	return D3Node{
		ID:       id,
		Name:     displayName,
		Kind:     kind,
		Language: language,
		Group:    group,
	}
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

// getMetadata fetches has_kind and has_language from the store.
func (t *D3Transformer) getMetadata(id string) (string, string) {
	kind := ""
	language := ""

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

	return kind, language
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
