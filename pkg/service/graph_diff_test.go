package service

import (
	"context"
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func setupTestStore(t *testing.T) (*meb.MEBStore, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "graph_diff_test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	return s, tmpDir
}

func closeTestStore(s *meb.MEBStore, tmpDir string) {
	s.Close()
	os.RemoveAll(tmpDir)
}

func TestTakeSnapshot_EmptyStore(t *testing.T) {
	s, tmpDir := setupTestStore(t)
	defer closeTestStore(s, tmpDir)

	svc := NewGraphDiffService()
	snap, err := svc.TakeSnapshot(context.Background(), s, "test-project")
	if err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}
	if snap.NodeCount != 0 {
		t.Errorf("Empty store: expected 0 nodes, got %d", snap.NodeCount)
	}
	if snap.EdgeCount != 0 {
		t.Errorf("Empty store: expected 0 edges, got %d", snap.EdgeCount)
	}
}

func TestTakeSnapshot_WithSymbols(t *testing.T) {
	s, tmpDir := setupTestStore(t)
	defer closeTestStore(s, tmpDir)

	ctx := context.Background()
	s.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateDefines, Object: "main.go:main"})
	s.AddFact(meb.Fact{Subject: "lib.go", Predicate: config.PredicateDefines, Object: "lib.go:helper"})

	svc := NewGraphDiffService()
	snap, err := svc.TakeSnapshot(ctx, s, "test-project")
	if err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}
	if snap.NodeCount != 2 {
		t.Errorf("Expected 2 nodes, got %d", snap.NodeCount)
	}
	if snap.Nodes["main.go:main"].Kind != "symbol" {
		t.Errorf("Expected kind 'symbol' for defined symbol")
	}
}

func TestTakeSnapshot_WithCalls(t *testing.T) {
	s, tmpDir := setupTestStore(t)
	defer closeTestStore(s, tmpDir)

	ctx := context.Background()
	s.AddFact(meb.Fact{Subject: "main.go:main", Predicate: config.PredicateCalls, Object: "lib.go:helper"})

	svc := NewGraphDiffService()
	snap, err := svc.TakeSnapshot(ctx, s, "test-project")
	if err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}
	if snap.NodeCount != 2 {
		t.Errorf("Expected 2 nodes (symbols), got %d", snap.NodeCount)
	}
	if snap.EdgeCount != 1 {
		t.Errorf("Expected 1 edge, got %d", snap.EdgeCount)
	}
	edgeKey := "main.go:main->lib.go:helper"
	if _, ok := snap.Edges[edgeKey]; !ok {
		t.Errorf("Expected edge key %q in edges map", edgeKey)
	}
}

func TestTakeSnapshot_WithImports(t *testing.T) {
	s, tmpDir := setupTestStore(t)
	defer closeTestStore(s, tmpDir)

	ctx := context.Background()
	s.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateImports, Object: "fmt"})

	svc := NewGraphDiffService()
	snap, err := svc.TakeSnapshot(ctx, s, "test-project")
	if err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}
	if snap.EdgeCount != 1 {
		t.Errorf("Expected 1 import edge, got %d", snap.EdgeCount)
	}
	edgeKey := "main.go->fmt"
	if _, ok := snap.Edges[edgeKey]; !ok {
		t.Errorf("Expected import edge %q", edgeKey)
	}
}

func TestTakeSnapshot_WithCommunities(t *testing.T) {
	s, tmpDir := setupTestStore(t)
	defer closeTestStore(s, tmpDir)

	ctx := context.Background()
	s.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateDefines, Object: "main"})
	s.AddFact(meb.Fact{Subject: "main", Predicate: "belongs_to_cluster", Object: "cluster_3"})

	svc := NewGraphDiffService()
	snap, err := svc.TakeSnapshot(ctx, s, "test-project")
	if err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}
	if snap.Communities["main"] != 3 {
		t.Errorf("Expected cluster 3 for 'main', got %d", snap.Communities["main"])
	}
	if snap.Nodes["main"].ClusterID != 3 {
		t.Errorf("Expected NodeSnap.ClusterID=3, got %d", snap.Nodes["main"].ClusterID)
	}
}

func TestTakeSnapshot_TypeAssertionSafety(t *testing.T) {
	s, tmpDir := setupTestStore(t)
	defer closeTestStore(s, tmpDir)

	ctx := context.Background()
	s.AddFact(meb.Fact{Subject: "main.go:main", Predicate: config.PredicateCalls, Object: 123})

	svc := NewGraphDiffService()
	snap, err := svc.TakeSnapshot(ctx, s, "test-project")
	if err != nil {
		t.Fatalf("TakeSnapshot should not fail on non-string object: %v", err)
	}
	if snap.EdgeCount != 0 {
		t.Errorf("Non-string object should be skipped, expected 0 edges, got %d", snap.EdgeCount)
	}
}

func TestDiffSnapshots_Identical(t *testing.T) {
	svc := NewGraphDiffService()
	s1 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {Kind: "symbol"}}, Edges: map[string]EdgeSnap{"a->b": {}}}
	s2 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {Kind: "symbol"}}, Edges: map[string]EdgeSnap{"a->b": {}}}
	diff := svc.DiffSnapshots(s1, s2)
	if len(diff.NewNodes) != 0 || len(diff.RemovedNodes) != 0 || len(diff.NewEdges) != 0 || len(diff.RemovedEdges) != 0 {
		t.Errorf("Identical snapshots should produce empty diff, got %+v", diff)
	}
}

func TestDiffSnapshots_NewNode(t *testing.T) {
	svc := NewGraphDiffService()
	s1 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {Kind: "symbol"}}, Edges: map[string]EdgeSnap{}}
	s2 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {Kind: "symbol"}, "b": {Kind: "symbol"}}, Edges: map[string]EdgeSnap{}}
	diff := svc.DiffSnapshots(s1, s2)
	if len(diff.NewNodes) != 1 || diff.NewNodes[0].ID != "b" {
		t.Errorf("Expected new node 'b', got %+v", diff.NewNodes)
	}
	if diff.Summary.NodesAdded != 1 {
		t.Errorf("Summary.NodesAdded=1, got %d", diff.Summary.NodesAdded)
	}
}

func TestDiffSnapshots_RemovedNode(t *testing.T) {
	svc := NewGraphDiffService()
	s1 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {}, "b": {}}, Edges: map[string]EdgeSnap{}}
	s2 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {}}, Edges: map[string]EdgeSnap{}}
	diff := svc.DiffSnapshots(s1, s2)
	if len(diff.RemovedNodes) != 1 || diff.RemovedNodes[0] != "b" {
		t.Errorf("Expected removed node 'b', got %+v", diff.RemovedNodes)
	}
}

func TestDiffSnapshots_NewEdge(t *testing.T) {
	svc := NewGraphDiffService()
	s1 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {}, "b": {}}, Edges: map[string]EdgeSnap{}}
	s2 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {}, "b": {}}, Edges: map[string]EdgeSnap{"a->b": {Predicate: "calls"}}}
	diff := svc.DiffSnapshots(s1, s2)
	if len(diff.NewEdges) != 1 {
		t.Errorf("Expected 1 new edge, got %d", len(diff.NewEdges))
	}
	if diff.NewEdges[0].Source != "a" || diff.NewEdges[0].Target != "b" {
		t.Errorf("NewEdge should parse source/target from key: %+v", diff.NewEdges[0])
	}
}

func TestDiffSnapshots_RemovedEdge(t *testing.T) {
	svc := NewGraphDiffService()
	s1 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {}, "b": {}}, Edges: map[string]EdgeSnap{"a->b": {}}}
	s2 := &GraphSnapshot{Nodes: map[string]NodeSnap{"a": {}, "b": {}}, Edges: map[string]EdgeSnap{}}
	diff := svc.DiffSnapshots(s1, s2)
	if len(diff.RemovedEdges) != 1 || diff.RemovedEdges[0] != "a->b" {
		t.Errorf("Expected removed edge 'a->b', got %+v", diff.RemovedEdges)
	}
}

func TestDiffSnapshots_CommunityMoves(t *testing.T) {
	svc := NewGraphDiffService()
	s1 := &GraphSnapshot{
		Communities: map[string]int{"main": 0, "lib": 1},
		Nodes:       map[string]NodeSnap{"main": {ClusterID: 0}, "lib": {ClusterID: 1}},
	}
	s2 := &GraphSnapshot{
		Communities: map[string]int{"main": 2, "lib": 1},
		Nodes:       map[string]NodeSnap{"main": {ClusterID: 2}, "lib": {ClusterID: 1}},
	}
	diff := svc.DiffSnapshots(s1, s2)
	if len(diff.CommunityChanges) != 1 {
		t.Fatalf("Expected 1 community change, got %d", len(diff.CommunityChanges))
	}
	cc := diff.CommunityChanges[0]
	if cc.Node != "main" || cc.BeforeCluster != 0 || cc.AfterCluster != 2 {
		t.Errorf("Community change mismatch: %+v", cc)
	}
	if diff.Summary.CommunityMoves != 1 {
		t.Errorf("Summary.CommunityMoves=1, got %d", diff.Summary.CommunityMoves)
	}
}

func TestDiffSnapshots_MixedChanges(t *testing.T) {
	svc := NewGraphDiffService()
	s1 := &GraphSnapshot{
		Nodes:   map[string]NodeSnap{"a": {}, "b": {}},
		Edges:   map[string]EdgeSnap{"a->b": {}},
	}
	s2 := &GraphSnapshot{
		Nodes:   map[string]NodeSnap{"a": {}, "c": {}},
		Edges:   map[string]EdgeSnap{"a->c": {}},
	}
	diff := svc.DiffSnapshots(s1, s2)
	if diff.Summary.NodesAdded != 1 || diff.Summary.NodesRemoved != 1 {
		t.Errorf("Mixed summary: expected 1 added/1 removed, got %+v", diff.Summary)
	}
	if diff.Summary.EdgesAdded != 1 || diff.Summary.EdgesRemoved != 1 {
		t.Errorf("Mixed edges: expected 1 added/1 removed, got %+v", diff.Summary)
	}
}

func TestSaveAndLoadSnapshot_Roundtrip(t *testing.T) {
	svc := NewGraphDiffService()
	snap := &GraphSnapshot{
		Nodes:     map[string]NodeSnap{"a": {Kind: "symbol", ClusterID: 1}},
		Edges:     map[string]EdgeSnap{"a->b": {Predicate: "calls"}},
		Communities: map[string]int{"a": 1},
		NodeCount: 1,
		EdgeCount: 1,
	}
	tmpPath := t.TempDir() + "/snapshot.json"

	err := svc.SaveSnapshot(snap, tmpPath)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	loaded, err := svc.LoadSnapshot(tmpPath)
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}
	if loaded.NodeCount != snap.NodeCount {
		t.Errorf("NodeCount mismatch: got %d, want %d", loaded.NodeCount, snap.NodeCount)
	}
	if loaded.EdgeCount != snap.EdgeCount {
		t.Errorf("EdgeCount mismatch: got %d, want %d", loaded.EdgeCount, snap.EdgeCount)
	}
	if _, ok := loaded.Nodes["a"]; !ok {
		t.Errorf("Node 'a' missing after roundtrip")
	}
	if _, ok := loaded.Edges["a->b"]; !ok {
		t.Errorf("Edge 'a->b' missing after roundtrip")
	}
}

func TestSaveSnapshot_Errors(t *testing.T) {
	svc := NewGraphDiffService()
	snap := &GraphSnapshot{Nodes: map[string]NodeSnap{}, Edges: map[string]EdgeSnap{}}
	err := svc.SaveSnapshot(snap, "/root/impossible.json")
	if err == nil {
		t.Error("Expected error saving to /root/")
	}
}

func TestLoadSnapshot_NotFound(t *testing.T) {
	svc := NewGraphDiffService()
	_, err := svc.LoadSnapshot("/nonexistent/path/snapshot.json")
	if err == nil {
		t.Error("Expected error loading nonexistent file")
	}
}

func TestSplitEdgeKey_Normal(t *testing.T) {
	parts := splitEdgeKey("a->b")
	if len(parts) != 2 || parts[0] != "a" || parts[1] != "b" {
		t.Errorf("splitEdgeKey(%q) = %v, want [a b]", "a->b", parts)
	}
}

func TestSplitEdgeKey_NoArrow(t *testing.T) {
	parts := splitEdgeKey("abc")
	if len(parts) != 2 || parts[0] != "abc" || parts[1] != "" {
		t.Errorf("splitEdgeKey(%q) = %v, want [abc ]", "abc", parts)
	}
}

func TestSplitEdgeKey_Empty(t *testing.T) {
	parts := splitEdgeKey("")
	if len(parts) != 2 || parts[0] != "" || parts[1] != "" {
		t.Errorf("splitEdgeKey(%q) = %v, want [ ]", "", parts)
	}
}