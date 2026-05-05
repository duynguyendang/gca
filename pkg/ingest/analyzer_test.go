package ingest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func setupMEBStore(t *testing.T) (*meb.MEBStore, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "analyzer_test")
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

func closeMEBStore(s *meb.MEBStore, tmpDir string) {
	s.Close()
	os.RemoveAll(tmpDir)
}

func TestWriteDegreeFacts_EmptyStore(t *testing.T) {
	src, tmpDir := setupMEBStore(t)
	defer closeMEBStore(src, tmpDir)

	analyzer := NewAnalyzer(nil, nil)
	err := analyzer.writeDegreeFacts(context.Background(), src, src)
	if err != nil {
		t.Fatalf("writeDegreeFacts should not error on empty store: %v", err)
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		var i int
		fmt.Sscanf(n, "%d", &i)
		return i, true
	default:
		return 0, false
	}
}

func TestWriteDegreeFacts_SingleCall(t *testing.T) {
	src, tmpDir := setupMEBStore(t)
	defer closeMEBStore(src, tmpDir)

	src.AddFact(meb.Fact{Subject: "main.go:main", Predicate: config.PredicateCalls, Object: "lib.go:helper"})

	analyzer := NewAnalyzer(nil, nil)
	err := analyzer.writeDegreeFacts(context.Background(), src, src)
	if err != nil {
		t.Fatalf("writeDegreeFacts failed: %v", err)
	}

	foundIn := 0
	foundOut := 0
	for fact := range src.Scan("", "has_in_degree", "") {
		if fact.Subject == "lib.go:helper" {
			if n, ok := toInt(fact.Object); ok && n == 1 {
				foundIn++
			}
		}
	}
	for fact := range src.Scan("", "has_out_degree", "") {
		if fact.Subject == "main.go:main" {
			if n, ok := toInt(fact.Object); ok && n == 1 {
				foundOut++
			}
		}
	}
	if foundIn != 1 {
		t.Errorf("Expected in_degree=1 for helper, got %d matches", foundIn)
	}
	if foundOut != 1 {
		t.Errorf("Expected out_degree=1 for main, got %d matches", foundOut)
	}
}

func TestWriteDegreeFacts_Chained(t *testing.T) {
	src, tmpDir := setupMEBStore(t)
	defer closeMEBStore(src, tmpDir)

	src.AddFact(meb.Fact{Subject: "a", Predicate: config.PredicateCalls, Object: "b"})
	src.AddFact(meb.Fact{Subject: "b", Predicate: config.PredicateCalls, Object: "c"})

	analyzer := NewAnalyzer(nil, nil)
	analyzer.writeDegreeFacts(context.Background(), src, src)

	expectDegree := map[string]map[string]int{
		"a": {"has_out_degree": 1, "has_in_degree": 0},
		"b": {"has_out_degree": 1, "has_in_degree": 1},
		"c": {"has_out_degree": 0, "has_in_degree": 1},
	}
	for sym, expected := range expectDegree {
		for pred, val := range expected {
			found := false
			for fact := range src.Scan(sym, pred, "") {
				if n, ok := toInt(fact.Object); ok && n == val {
					found = true
				}
			}
			if !found {
				t.Errorf("Expected %s %s=%d", sym, pred, val)
			}
		}
	}
}

func TestWriteDegreeFacts_MultiCaller(t *testing.T) {
	src, tmpDir := setupMEBStore(t)
	defer closeMEBStore(src, tmpDir)

	src.AddFact(meb.Fact{Subject: "a", Predicate: config.PredicateCalls, Object: "c"})
	src.AddFact(meb.Fact{Subject: "b", Predicate: config.PredicateCalls, Object: "c"})

	analyzer := NewAnalyzer(nil, nil)
	analyzer.writeDegreeFacts(context.Background(), src, src)

	found := 0
	for fact := range src.Scan("c", "has_in_degree", "") {
		if n, ok := toInt(fact.Object); ok && n == 2 {
			found++
		}
	}
	if found != 1 {
		t.Errorf("Expected in_degree=2 for c, got %d matches", found)
	}
}

func TestWriteDegreeFacts_WithImports(t *testing.T) {
	src, tmpDir := setupMEBStore(t)
	defer closeMEBStore(src, tmpDir)

	src.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateCalls, Object: "lib.go:helper"})
	src.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateImports, Object: "fmt"})

	analyzer := NewAnalyzer(nil, nil)
	analyzer.writeDegreeFacts(context.Background(), src, src)

	foundFmt := false
	for fact := range src.Scan("fmt", "has_in_degree", "") {
		if n, ok := toInt(fact.Object); ok && n == 1 {
			foundFmt = true
		}
	}
	if !foundFmt {
		t.Errorf("Expected in_degree=1 for fmt from imports")
	}
}

func TestDetectCommunitiesLeidenLocal_Empty(t *testing.T) {
	result := detectCommunitiesLeidenLocal(nil)
	if len(result.Clusters) != 0 || len(result.NodeCluster) != 0 {
		t.Errorf("Empty input should return empty result, got clusters=%v", result.Clusters)
	}
}

func TestDetectCommunitiesLeidenLocal_SingleNode(t *testing.T) {
	nodes := []clusterNode{
		{ID: "solo", Weight: 1.0, Neighbors: map[int]float64{}},
	}
	result := detectCommunitiesLeidenLocal(nodes)
	if len(result.Clusters) != 1 {
		t.Errorf("Single node should form 1 cluster, got %d", len(result.Clusters))
	}
	if result.NodeCluster["solo"] != 0 {
		t.Errorf("Solo node should be in cluster 0, got %d", result.NodeCluster["solo"])
	}
}

func TestDetectCommunitiesLeidenLocal_DisconnectedNodes(t *testing.T) {
	nodes := []clusterNode{
		{ID: "a", Weight: 1.0, Neighbors: map[int]float64{}},
		{ID: "b", Weight: 1.0, Neighbors: map[int]float64{}},
	}
	result := detectCommunitiesLeidenLocal(nodes)
	if result.NodeCluster["a"] < 0 || result.NodeCluster["b"] < 0 {
		t.Errorf("All nodes must have cluster assignments, got a=%d b=%d", result.NodeCluster["a"], result.NodeCluster["b"])
	}
}

func TestDetectCommunitiesLeidenLocal_ConnectedPair(t *testing.T) {
	nodes := []clusterNode{
		{ID: "a", Weight: 1.0, Neighbors: map[int]float64{1: 1.0}},
		{ID: "b", Weight: 1.0, Neighbors: map[int]float64{0: 1.0}},
	}
	result := detectCommunitiesLeidenLocal(nodes)
	if result.NodeCluster["a"] != result.NodeCluster["b"] {
		t.Errorf("Connected pair should be in same cluster: a=%d b=%d", result.NodeCluster["a"], result.NodeCluster["b"])
	}
}

func TestDetectCommunitiesLeidenLocal_TwoClusters(t *testing.T) {
	nodes := []clusterNode{
		{ID: "a", Weight: 1.0, Neighbors: map[int]float64{1: 1.0}},
		{ID: "b", Weight: 1.0, Neighbors: map[int]float64{0: 1.0}},
		{ID: "c", Weight: 1.0, Neighbors: map[int]float64{3: 1.0}},
		{ID: "d", Weight: 1.0, Neighbors: map[int]float64{2: 1.0}},
	}
	result := detectCommunitiesLeidenLocal(nodes)
	if result.NodeCluster["a"] != result.NodeCluster["b"] {
		t.Errorf("a and b should be together")
	}
	if result.NodeCluster["c"] != result.NodeCluster["d"] {
		t.Errorf("c and d should be together")
	}
	if result.NodeCluster["a"] < 0 || result.NodeCluster["b"] < 0 || result.NodeCluster["c"] < 0 || result.NodeCluster["d"] < 0 {
		t.Errorf("All nodes must be assigned, got %+v", result.NodeCluster)
	}
}

func TestWriteCommunityFacts_EmptyStore(t *testing.T) {
	src, tmpDir := setupMEBStore(t)
	defer closeMEBStore(src, tmpDir)

	analyzer := NewAnalyzer(nil, nil)
	err := analyzer.writeCommunityFacts(context.Background(), src, src)
	if err != nil {
		t.Fatalf("writeCommunityFacts should not error on empty store: %v", err)
	}
}

func TestWriteCommunityFacts_WithEdges(t *testing.T) {
	src, tmpDir := setupMEBStore(t)
	defer closeMEBStore(src, tmpDir)

	src.AddFact(meb.Fact{Subject: "a.go:symbolA", Predicate: config.PredicateCalls, Object: "b.go:symbolB"})

	analyzer := NewAnalyzer(nil, nil)
	err := analyzer.writeCommunityFacts(context.Background(), src, src)
	if err != nil {
		t.Fatalf("writeCommunityFacts failed: %v", err)
	}

	found := 0
	for fact := range src.Scan("", "belongs_to_cluster", "") {
		found++
		obj, ok := fact.Object.(string)
		if !ok {
			t.Errorf("belongs_to_cluster object must be string")
			continue
		}
		if len(obj) < 8 || obj[:8] != "cluster_" {
			t.Errorf("Cluster ID should start with 'cluster_', got %q", obj)
		}
	}
	if found < 2 {
		t.Errorf("Expected at least 2 cluster facts, got %d", found)
	}
}