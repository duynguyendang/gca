package ingest

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

type scoringMgr struct {
	src *meb.MEBStore
	ana *meb.MEBStore
}

func (m *scoringMgr) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	return m.src, nil
}
func (m *scoringMgr) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return m.ana, nil
}

func newScoringStores(t *testing.T) (src, ana *meb.MEBStore, cleanup func()) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "scoring_src")
	if err != nil {
		t.Fatal(err)
	}
	anaDir, err := os.MkdirTemp("", "scoring_ana")
	if err != nil {
		os.RemoveAll(srcDir)
		t.Fatal(err)
	}
	src, err = meb.NewMEBStore(store.DefaultConfig(srcDir))
	if err != nil {
		os.RemoveAll(srcDir)
		os.RemoveAll(anaDir)
		t.Fatal(err)
	}
	ana, err = meb.NewMEBStore(store.DefaultConfig(anaDir))
	if err != nil {
		src.Close()
		os.RemoveAll(srcDir)
		os.RemoveAll(anaDir)
		t.Fatal(err)
	}
	cleanup = func() {
		ana.Close()
		src.Close()
		os.RemoveAll(anaDir)
		os.RemoveAll(srcDir)
	}
	return src, ana, cleanup
}

func TestComputeHealthScores(t *testing.T) {
	src, ana, cleanup := newScoringStores(t)
	defer cleanup()

	// Files enumerated from source-store defines subjects.
	_ = src.AddFact(meb.Fact{Subject: "a.go", Predicate: config.PredicateDefines, Object: "a.go:FA"})
	_ = src.AddFact(meb.Fact{Subject: "b.go", Predicate: config.PredicateDefines, Object: "b.go:FB"})
	_ = src.AddFact(meb.Fact{Subject: "clean.go", Predicate: config.PredicateDefines, Object: "clean.go:FC"})

	// a.go has one smell (god_file=6). b.go has two smells plus hub penalty.
	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: "has_smell_type", Object: "god_file"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: "has_smell_type", Object: "circular_dependency"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: "has_smell_type", Object: "dead_code"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: "has_hub_score", Object: "12"})

	mgr := &scoringMgr{src: src, ana: ana}
	analyzer := NewAnalyzer(mgr, nil)
	if err := analyzer.computeHealthScores(context.Background(), "proj"); err != nil {
		t.Fatalf("computeHealthScores failed: %v", err)
	}

	// Expected debt: a.go=6, b.go=10+3+5=18; clean.go has no debt fact.
	debt := map[string]int{}
	for f := range ana.ScanContext(context.Background(), "", config.PredicateHasHealthDebt, "") {
		if f.Subject == "" {
			continue
		}
		if v, ok := f.Object.(string); ok {
			if n, err := strconv.Atoi(v); err == nil {
				debt[f.Subject] = n
			}
		}
	}
	if len(debt) != 2 {
		t.Fatalf("expected 2 debt facts, got %d: %v", len(debt), debt)
	}
	if debt["a.go"] != 6 {
		t.Errorf("expected a.go debt 6, got %d", debt["a.go"])
	}
	if debt["b.go"] != 18 {
		t.Errorf("expected b.go debt 18, got %d", debt["b.go"])
	}
	if _, ok := debt["clean.go"]; ok {
		t.Errorf("clean.go should have no debt fact, got %d", debt["clean.go"])
	}

	// Scores: a.go=94, b.go=82, clean.go=100.
	score := map[string]int{}
	for f := range ana.ScanContext(context.Background(), "", config.PredicateHasHealthScore, "") {
		if f.Subject == "" {
			continue
		}
		if v, ok := f.Object.(string); ok {
			if n, err := strconv.Atoi(v); err == nil {
				score[f.Subject] = n
			}
		}
	}
	if score["a.go"] != 94 {
		t.Errorf("expected a.go score 94, got %d", score["a.go"])
	}
	if score["b.go"] != 82 {
		t.Errorf("expected b.go score 82, got %d", score["b.go"])
	}
	if score["clean.go"] != 100 {
		t.Errorf("expected clean.go score 100, got %d", score["clean.go"])
	}
}
