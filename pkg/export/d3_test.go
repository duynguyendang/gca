package export

import (
	"context"
	"testing"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func TestD3Transformer(t *testing.T) {
	// Setup InMemory store
	cfg := &store.Config{
		InMemory:       true,
		DataDir:        "", // Should be ignored for InMemory
		DictDir:        "",
		BlockCacheSize: 10 << 20, // 10MB
		IndexCacheSize: 10 << 20, // 10MB
		LRUCacheSize:   1000,
	}
	// meb.NewMEBStore validates config.
	// If validation fails on empty DataDir/DictDir even for InMemory, we'll see.
	// Assuming standard behavior of allowing empty for InMemory.

	// Workaround if validation is strict:
	cfg.DataDir = t.TempDir()
	cfg.DictDir = t.TempDir()

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// 1. Setup metadata in store
	queryID := "/pkg/a.go:FuncA"
	targetID := "/pkg/b.go:FuncB"

	if err := s.AddFact(meb.NewFact(queryID, "has_kind", "func")); err != nil {
		t.Fatalf("Failed to add fact: %v", err)
	}
	s.AddFact(meb.NewFact(queryID, "has_language", "go"))
	// Target has no explicit metadata, should fallback

	// 2. Mock results from a query like `triples(?s, "calls", ?o)`
	results := []map[string]any{
		{"?s": queryID, "?p": "calls", "?o": targetID},
	}

	// 3. Test Transformation
	transformer := NewD3Transformer(s)
	graph, err := transformer.Transform(ctx, `triples(?s, ?p, ?o)`, results)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	// Verify Nodes
	if len(graph.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes))
	}

	var nodeA, nodeB D3Node
	for _, n := range graph.Nodes {
		if n.ID == queryID {
			nodeA = n
		} else if n.ID == targetID {
			nodeB = n
		}
	}

	// Verify Node A (Enriched)
	if nodeA.ID == "" {
		t.Fatal("Node A not found")
	}
	if nodeA.Name != "a.go:FuncA" {
		t.Errorf("Expected Name 'a.go:FuncA', got '%s'", nodeA.Name)
	}
	if nodeA.Kind != "func" {
		t.Errorf("Expected Kind 'func', got '%s'", nodeA.Kind)
	}
	if nodeA.Language != "go" {
		t.Errorf("Expected Language 'go', got '%s'", nodeA.Language)
	}

	// Verify Node B (Fallback)
	if nodeB.ID == "" {
		t.Fatal("Node B not found")
	}
	if nodeB.Name != "b.go:FuncB" {
		t.Errorf("Expected Name 'b.go:FuncB', got '%s'", nodeB.Name)
	}
	if nodeB.Language != "go" {
		// Fallback for .go extension
		t.Errorf("Expected Language 'go' (inferred), got '%s'", nodeB.Language)
	}

	// 4. Test FilterTests
	resultsWithTest := []map[string]any{
		{"?s": "/pkg/a_test.go:TestFunc", "?p": "calls", "?o": queryID},
	}
	transformer.ExcludeTestFiles = true
	graphTest, err := transformer.Transform(ctx, `triples(?s, ?p, ?o)`, resultsWithTest)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if len(graphTest.Nodes) != 0 {
		t.Errorf("Expected 0 nodes filtered out, got %d", len(graphTest.Nodes))
	}
}
