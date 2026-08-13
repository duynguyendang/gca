package ingest

import (
	"context"
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

type deadCodeMgr struct {
	src *meb.MEBStore
	ana *meb.MEBStore
}

func (m *deadCodeMgr) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	return m.src, nil
}
func (m *deadCodeMgr) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return m.ana, nil
}

func newDeadCodeStores(t *testing.T) (src, ana *meb.MEBStore, cleanup func()) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "deadcode_src")
	if err != nil {
		t.Fatal(err)
	}
	anaDir, err := os.MkdirTemp("", "deadcode_ana")
	if err != nil {
		os.RemoveAll(srcDir)
		t.Fatal(err)
	}
	cfg1 := store.DefaultConfig(srcDir)
	cfg2 := store.DefaultConfig(anaDir)
	src, err = meb.NewMEBStore(cfg1)
	if err != nil {
		os.RemoveAll(srcDir)
		os.RemoveAll(anaDir)
		t.Fatal(err)
	}
	ana, err = meb.NewMEBStore(cfg2)
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

func TestDetectDeadCode(t *testing.T) {
	src, ana, cleanup := newDeadCodeStores(t)
	defer cleanup()

	// active.go/Used is called by main; active.go/Dead is never called.
	src.AddFact(meb.Fact{Subject: "active.go", Predicate: config.PredicateDefines, Object: "active.go:Used"})
	src.AddFact(meb.Fact{Subject: "active.go:Used", Predicate: config.PredicateHasKind, Object: "func"})
	src.AddFact(meb.Fact{Subject: "active.go", Predicate: config.PredicateDefines, Object: "active.go:Dead"})
	src.AddFact(meb.Fact{Subject: "active.go:Dead", Predicate: config.PredicateHasKind, Object: "func"})
	src.AddFact(meb.Fact{Subject: "active.go", Predicate: config.PredicateDefines, Object: "active.go:handler"})
	src.AddFact(meb.Fact{Subject: "active.go:handler", Predicate: config.PredicateHasKind, Object: "func"})
	src.AddFact(meb.Fact{Subject: "active.go:handler", Predicate: config.PredicateHasRole, Object: "api_handler"})

	// main calls Used, nothing calls Dead.
	src.AddFact(meb.Fact{Subject: "main.go:main", Predicate: config.PredicateCalls, Object: "active.go:Used"})

	mgr := &deadCodeMgr{src: src, ana: ana}
	analyzer := NewAnalyzer(mgr, nil)
	if err := analyzer.detectDeadCode(context.Background(), "proj"); err != nil {
		t.Fatalf("detectDeadCode failed: %v", err)
	}

	// Only active.go:Dead should be flagged; Used is called, handler is api_handler.
	found := map[string]bool{}
	for f := range ana.ScanContext(context.Background(), "", "has_smell_type", "dead_code") {
		if f.Subject != "" {
			found[f.Subject] = true
		}
	}

	if len(found) != 1 {
		t.Fatalf("expected exactly 1 dead-code file, got %d: %v", len(found), found)
	}
	if !found["active.go"] {
		t.Errorf("expected active.go to be flagged, got %v", found)
	}

	// Severity facts written.
	sevOk := false
	for fact := range ana.Scan("active.go", "has_smell_severity", "") {
		if v, ok := fact.Object.(string); ok && v == "low" {
			sevOk = true
		}
	}
	if !sevOk {
		t.Error("expected has_smell_severity=low for active.go")
	}
}

func TestDetectDeadCode_NoFalsePositiveOnTestSymbol(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "deadcode_test_src")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)
	anaDir, err := os.MkdirTemp("", "deadcode_test_ana")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(anaDir)

	src, err := meb.NewMEBStore(store.DefaultConfig(srcDir))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	ana, err := meb.NewMEBStore(store.DefaultConfig(anaDir))
	if err != nil {
		t.Fatal(err)
	}
	defer ana.Close()

	src.AddFact(meb.Fact{Subject: "util_test.go", Predicate: config.PredicateDefines, Object: "util_test.go:helper"})
	src.AddFact(meb.Fact{Subject: "util_test.go:helper", Predicate: config.PredicateHasKind, Object: "func"})

	mgr := &deadCodeMgr{src: src, ana: ana}
	analyzer := NewAnalyzer(mgr, nil)
	if err := analyzer.detectDeadCode(context.Background(), "proj"); err != nil {
		t.Fatalf("detectDeadCode failed: %v", err)
	}

	count := 0
	for f := range ana.ScanContext(context.Background(), "", "has_smell_type", "dead_code") {
		if f.Subject != "" {
			count++
		}
	}
	if count != 0 {
		t.Errorf("expected no dead-code facts for test symbols, got %d", count)
	}
}