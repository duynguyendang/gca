package service

import (
	"context"
	"os"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

// MockStoreManager
type MockStoreManager struct {
	store *meb.MEBStore
}

func (m *MockStoreManager) GetStore(id string) (*meb.MEBStore, error) {
	return m.store, nil
}
func (m *MockStoreManager) ListProjects() ([]manager.ProjectMetadata, error) {
	return nil, nil
}

func TestGetFileGraph_Lazy(t *testing.T) {
	// 1. Setup Store
	tmpDir, err := os.MkdirTemp("", "graph_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	cfg.BypassLockGuard = true // For testing
	s, err := meb.Open(tmpDir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 2. Add Data
	// File "main.go" defines "main"
	// "main" calls "foo"
	file := "main.go"
	mainFunc := "main.go:main"
	fooFunc := "pkg/foo.go:foo"

	ctx := context.Background()

	// Add facts
	// AddFact(Fact) error
	if err := s.AddFact(meb.Fact{Subject: meb.DocumentID(file), Predicate: "defines", Object: mainFunc, Graph: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFact(meb.Fact{Subject: meb.DocumentID(mainFunc), Predicate: "calls", Object: fooFunc, Graph: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFact(meb.Fact{Subject: meb.DocumentID(file), Predicate: "imports", Object: "fmt", Graph: "default"}); err != nil {
		t.Fatal(err)
	}

	// 3. Setup Service
	svc := NewGraphService(&MockStoreManager{store: s})

	// 4. Test NOT Lazy (default) -> Expect calls
	gFull, err := svc.GetFileGraph(ctx, "test", file, false)
	if err != nil {
		t.Fatalf("GetFileGraph(lazy=false) failed: %v", err)
	}

	hasCall := false
	for _, l := range gFull.Links {
		if l.Relation == "calls" && l.Source == mainFunc && l.Target == fooFunc {
			hasCall = true
			break
		}
	}
	if !hasCall {
		t.Errorf("Expected call edge in full graph, got none")
	}

	// 5. Test Lazy -> Expect NO calls
	gLazy, err := svc.GetFileGraph(ctx, "test", file, true)
	if err != nil {
		t.Fatalf("GetFileGraph(lazy=true) failed: %v", err)
	}

	for _, l := range gLazy.Links {
		if l.Relation == "calls" {
			t.Errorf("Expected NO calls in lazy graph, found one: %v", l)
		}
	}

	// Verify Imports still exist in Lazy
	hasImport := false
	for _, l := range gLazy.Links {
		if l.Relation == "imports" {
			hasImport = true
			break
		}
	}
	if !hasImport {
		t.Errorf("Expected imports in lazy graph, found none")
	}

	// Verify Defines still exist in Lazy
	hasDefine := false
	for _, l := range gLazy.Links {
		if l.Relation == "defines" {
			hasDefine = true
			break
		}
	}
	if !hasDefine {
		t.Errorf("Expected defines in lazy graph, found none")
	}
}

func TestGetFlowPath(t *testing.T) {
	// 1. Setup Store
	tmpDir, err := os.MkdirTemp("", "graph_flow_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	cfg.BypassLockGuard = true // For testing
	s, err := meb.Open(tmpDir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 2. Add Data: fileA -> fileB -> fileC
	if err := s.AddFact(meb.Fact{Subject: "fileA", Predicate: "calls", Object: "fileB", Graph: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFact(meb.Fact{Subject: "fileB", Predicate: "calls", Object: "fileC", Graph: "default"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFact(meb.Fact{Subject: "fileA", Predicate: "calls", Object: "fileD", Graph: "default"}); err != nil {
		t.Fatal(err)
	}

	svc := NewGraphService(&MockStoreManager{store: s})

	// 3. Test Path fileA -> fileC
	g, err := svc.GetFlowPath(context.Background(), "test", "fileA", "fileC")
	if err != nil {
		t.Fatalf("GetFlowPath failed: %v", err)
	}

	// Expect 3 nodes: fileA, fileB, fileC
	if len(g.Nodes) != 3 {
		t.Logf("Nodes: %+v", g.Nodes)
		t.Logf("Links: %+v", g.Links)
		t.Errorf("Expected 3 nodes, got %d", len(g.Nodes))
	}

	// Expect path: fileA->fileB, fileB->fileC
	// Check links
	hasAB := false
	hasBC := false
	for _, l := range g.Links {
		if l.Source == "fileA" && l.Target == "fileB" {
			hasAB = true
		}
		if l.Source == "fileB" && l.Target == "fileC" {
			hasBC = true
		}
	}
	if !hasAB || !hasBC {
		t.Errorf("Path incomplete. hasAB=%v, hasBC=%v", hasAB, hasBC)
	}
}
