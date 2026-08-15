package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

func TestCollectKPISnapshot(t *testing.T) {
	src, ana, cleanup := newScoringStores(t)
	defer cleanup()

	_ = src.AddFact(meb.Fact{Subject: "a.go", Predicate: config.PredicateDefines, Object: "a.go:FA"})
	_ = src.AddFact(meb.Fact{Subject: "b.go", Predicate: config.PredicateDefines, Object: "b.go:FB"})

	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: config.PredicateHasHealthScore, Object: "80"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: config.PredicateHasHealthScore, Object: "60"})
	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: config.PredicateHasHealthDebt, Object: "10"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: config.PredicateHasHealthDebt, Object: "25"})
	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: "has_smell_type", Object: "god_file"})
	_ = ana.AddFact(meb.Fact{Subject: "a2.go", Predicate: "has_smell_type", Object: "god_file"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: "has_smell_type", Object: "dead_code"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: "has_smell_type", Object: "high_complexity"})
	_ = ana.AddFact(meb.Fact{Subject: "b.go", Predicate: "has_smell_type", Object: "duplicate_code"})

	mgr := &scoringMgr{src: src, ana: ana}
	a := NewAnalyzer(mgr, nil)

	snap, err := a.collectKPISnapshot(context.Background(), "proj", ana)
	if err != nil {
		t.Fatalf("collectKPISnapshot failed: %v", err)
	}
	if snap.HealthScore != 70 {
		t.Errorf("health score = %d, want 70", snap.HealthScore)
	}
	if snap.HealthDebt != 35 {
		t.Errorf("health debt = %d, want 35", snap.HealthDebt)
	}
	if snap.SmellCount != 5 {
		t.Errorf("smell count = %d, want 5", snap.SmellCount)
	}
	if snap.TopSmell != "god_file" {
		t.Errorf("top smell = %q, want god_file", snap.TopSmell)
	}
	if snap.DeadCodeCount != 1 || snap.ComplexityCount != 1 || snap.DuplicateCount != 1 {
		t.Errorf("tallies mismatch: dead=%d complex=%d dup=%d", snap.DeadCodeCount, snap.ComplexityCount, snap.DuplicateCount)
	}
	if snap.CommitSHA != "" {
		t.Errorf("commit SHA should be empty (no source store), got %q", snap.CommitSHA)
	}
	if snap.ID == "" {
		t.Error("snapshot ID must not be empty")
	}
}

func TestRecordKPISnapshot_PersistsAsJSONFact(t *testing.T) {
	src, ana, cleanup := newScoringStores(t)
	defer cleanup()

	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: config.PredicateHasHealthScore, Object: "80"})
	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: config.PredicateHasHealthDebt, Object: "10"})
	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: "has_smell_type", Object: "god_file"})

	mgr := &scoringMgr{src: src, ana: ana}
	a := NewAnalyzer(mgr, nil)

	if err := a.recordKPISnapshot(context.Background(), "proj"); err != nil {
		t.Fatalf("recordKPISnapshot failed: %v", err)
	}

	found := 0
	for fact := range ana.ScanContext(context.Background(), "", config.PredicateKPISnapshot, "") {
		if fact.Subject == "" {
			continue
		}
		obj, ok := fact.Object.(string)
		if !ok {
			t.Fatalf("snapshot fact object is not a string: %T", fact.Object)
		}
		var snap KPISnapshot
		if err := json.Unmarshal([]byte(obj), &snap); err != nil {
			t.Fatalf("snapshot is not valid JSON: %v", err)
		}
		if snap.HealthScore != 80 || snap.HealthDebt != 10 || snap.SmellCount != 1 {
			t.Errorf("snapshot mismatch: %+v", snap)
		}
		found++
	}
	if found != 1 {
		t.Errorf("expected exactly 1 snapshot fact, found %d", found)
	}
}

func TestPruneKPISnapshots_Retention(t *testing.T) {
	src, ana, cleanup := newScoringStores(t)
	defer cleanup()

	ctx := context.Background()
	// Seed more than retention with ascending timestamps.
	base := time.Now().Add(-time.Duration(config.KPISnapshotRetention+10) * time.Hour)
	for i := 0; i < config.KPISnapshotRetention+10; i++ {
		snap := KPISnapshot{
			ID:        "kpi:proj:t" + time.Now().Format("150405.000000000") + ":" + string(rune('a'+i%26)),
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}
		body, _ := json.Marshal(snap)
		_ = ana.AddFact(meb.Fact{Subject: snap.ID, Predicate: config.PredicateKPISnapshot, Object: string(body)})
	}

	mgr := &scoringMgr{src: src, ana: ana}
	a := NewAnalyzer(mgr, nil)

	pruned := a.pruneKPISnapshots(ctx, ana)
	if pruned != 10 {
		t.Fatalf("pruned = %d, want 10", pruned)
	}

	count := 0
	for fact := range ana.ScanContext(ctx, "", config.PredicateKPISnapshot, "") {
		if fact.Subject != "" {
			count++
		}
	}
	if count != config.KPISnapshotRetention {
		t.Errorf("remaining snapshots = %d, want %d", count, config.KPISnapshotRetention)
	}
}
