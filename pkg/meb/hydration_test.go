package meb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/duynguyendang/gca/pkg/meb/store"
)

func TestHydration(t *testing.T) {
	// Setup Store
	tmpDir := t.TempDir()
	cfg := store.DefaultConfig(tmpDir)
	cfg.DictDir = filepath.Join(tmpDir, "dict")
	// InMemory might behave differently regarding persistence, but logic should hold.
	// We use disk-based with temp dir for realism, or InMemory if preferred.
	// Let's use InMemory for speed.
	cfg.InMemory = true

	s, err := NewMEBStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	// 1. Ingest a Document
	docID := DocumentID("pkg/auth:Login")
	content := []byte("func Login() {\n  // logic\n}")
	metadata := map[string]any{
		"language": "go",
		"lines":    10,
	}

	err = s.AddDocument(docID, content, nil, metadata)
	if err != nil {
		t.Fatalf("Failed to add document: %v", err)
	}

	// 2. Add Facts (Type/Kind)
	// We manually add the 'type' fact which might be extracted separately
	// In the real pipeline, 'type' might be added during ingestion.
	// AddDocument adds metadata facts.
	// Let's add a explicit type fact.
	typeFact := Fact{
		Subject:   docID,
		Predicate: "type", // Assuming "type" is the predicate for Kind
		Object:    "function",
		Graph:     "default",
	}
	// Need to register predicate if not default?
	// NewMEBStore registers default (triples).
	// "type" might not be registered.
	// Let's use "triples" with pred="type"?
	// Or register "type".
	// meb.store registers "triples".
	// AddFact uses internal logic.
	// Let's see if we can use "triples" directly if AddFact supports dynamic predicates?
	// AddFact checks if predicate is registered.
	// We might need to register "type".

	// Register "type" predicate manually for test
	// But predicates are private map?
	// We can use s.predicates if we are in package meb.
	// Yes, we are in package meb.

	// Wait, internal access allows us to modify predicates map?
	// Ideally we use a public API to register predicates.
	// But there isn't one visible in store.go.
	// Let's assume we use "triples(docID, 'type', 'function')" if "type" isn't available.
	// But Hydrate looks for predicate "type".
	// line 64 of hydration.go: for fact := range m.ScanContext(ctx, string(id), PredType, "", "")
	// PredType is a constant? I need to check constants.
	// If PredType is not defined, code won't compile.
	// I assumed PredType existed.
	// Let's define it or check types.go.
	// If types.go doesn't have it, I should update hydration.go to use "type" string.
	//
	// Let's check imports in hydration.go again.
	// I used PredType.

	// Correction: I should verify if PredType exists.
	// If not, I will fix hydration.go first.

	// Proceeding assuming PredType is "type" or string "type".

	// Checking store.go, RegisterPredicate is NOT exported.
	// registerDefaultPredicates is not exported.

	// workaround: Modify s.predicates directly since we are in same package.
	// s.predicates[ast.PredicateSym{Symbol: "type", Arity: 3}] = ...
	// But predicates.NewPredicateTable requires imports.

	// Easier: Just use "type" in AddFact if it auto-registers?
	// Or maybe AddFact fails if not registered.
	// Let's try to register it.

	// Create table
	// need import "github.com/duynguyendang/gca/pkg/meb/keys"
	// need import "github.com/google/mangle/ast"

	// Hack: implementation of Hydrate used "PredType".
	// I will check if PredType is defined. if not, I'll pass "type" string.

	// For the test, I'll register it.

	// Actually, let's just run it and see if it fails compilation due to PredType.

	if err := s.AddFact(typeFact); err != nil {
		t.Fatalf("Failed to add type fact: %v", err)
	}

	// 3. Test Hydrate
	ctx := context.Background()
	results, err := s.Hydrate(ctx, []DocumentID{docID}, false)
	if err != nil {
		t.Fatalf("Hydrate failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	res := results[0]

	// Verify ID
	if res.ID != docID {
		t.Errorf("Expected ID %s, got %s", docID, res.ID)
	}

	// Verify Content
	if res.Content != string(content) {
		t.Errorf("Expected content %q, got %q", string(content), res.Content)
	}

	// Verify Kind
	if res.Kind != "function" {
		t.Errorf("Expected kind 'function', got %q", res.Kind)
	}

	// Verify Metadata
	// Metadata might contain "language" and "lines" added via AddDocument
	if val, ok := res.Metadata["language"]; !ok || val != "go" {
		t.Errorf("Expected metadata language='go', got %v", val)
	}
}

func TestRecursiveHydration(t *testing.T) {
	// Setup Store
	tmpDir := t.TempDir()
	cfg := store.DefaultConfig(tmpDir)
	cfg.DictDir = filepath.Join(tmpDir, "dict")
	cfg.InMemory = true

	s, err := NewMEBStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	// 1. Ingest Parent (File)
	parentID := DocumentID("pkg/auth/login.go")
	parentContent := []byte("package auth\n...")
	if err := s.AddDocument(parentID, parentContent, nil, nil); err != nil {
		t.Fatalf("Failed to add parent: %v", err)
	}

	// 2. Ingest Child (Function)
	childID := DocumentID("pkg/auth/login.go:Login")
	childContent := []byte("func Login() {}")
	if err := s.AddDocument(childID, childContent, nil, nil); err != nil {
		t.Fatalf("Failed to add child: %v", err)
	}

	// 3. Add Relationship: File defines Function
	definesFact := Fact{
		Subject:   parentID,
		Predicate: "defines",
		Object:    string(childID), // Object is string for now
		Graph:     "default",
	}
	if err := s.AddFact(definesFact); err != nil {
		t.Fatalf("Failed to add defines fact: %v", err)
	}

	// 4. Hydrate Parent
	ctx := context.Background()
	results, err := s.Hydrate(ctx, []DocumentID{parentID}, false)
	if err != nil {
		t.Fatalf("Hydrate failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	parent := results[0]
	if parent.ID != parentID {
		t.Errorf("Expected parent ID %s, got %s", parentID, parent.ID)
	}

	// 5. Verify Child
	if len(parent.Children) != 1 {
		t.Fatalf("Expected 1 child, got %d", len(parent.Children))
	}
	child := parent.Children[0]
	if child.ID != childID {
		t.Errorf("Expected child ID %s, got %s", childID, child.ID)
	}
	if child.Content != string(childContent) {
		t.Errorf("Expected child content %q, got %q", string(childContent), child.Content)
	}
}
