package ingest

import (
	"context"
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

type securityMgr struct {
	src *meb.MEBStore
	ana *meb.MEBStore
}

func (m *securityMgr) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	return m.src, nil
}
func (m *securityMgr) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return m.ana, nil
}

func newSecurityStores(t *testing.T) (src, ana *meb.MEBStore, cleanup func()) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "security_src")
	if err != nil {
		t.Fatal(err)
	}
	anaDir, err := os.MkdirTemp("", "security_ana")
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

func TestDetectSecuritySmells(t *testing.T) {
	src, ana, cleanup := newSecurityStores(t)
	defer cleanup()

	// Public API handler that directly calls a DB symbol.
	_ = src.AddFact(meb.Fact{Subject: "api/handlers.go", Predicate: config.PredicateDefines, Object: "api/handlers.go:HandleUser"})
	_ = src.AddFact(meb.Fact{Subject: "api/handlers.go", Predicate: config.PredicateDefines, Object: "api/handlers.go:CheckToken"})

	// Database file that owns the callee.
	_ = src.AddFact(meb.Fact{Subject: "db/repo.go", Predicate: config.PredicateDefines, Object: "db/repo.go:QueryUser"})

	// DB client that calls a DB symbol but has no error-handling symbol.
	_ = src.AddFact(meb.Fact{Subject: "db/client.go", Predicate: config.PredicateDefines, Object: "db/client.go:RunQuery"})

	// Non-tagged utility file: has a validation symbol (no missing-error-check).
	_ = src.AddFact(meb.Fact{Subject: "internal/util/safe.go", Predicate: config.PredicateDefines, Object: "internal/util/safe.go:GetThing"})
	_ = src.AddFact(meb.Fact{Subject: "internal/util/safe.go", Predicate: config.PredicateDefines, Object: "internal/util/safe.go:ValidateX"})

	_ = src.AddFact(meb.Fact{Subject: "api/handlers.go:HandleUser", Predicate: config.PredicateCalls, Object: "db/repo.go:QueryUser"})
	_ = src.AddFact(meb.Fact{Subject: "internal/util/safe.go:GetThing", Predicate: config.PredicateCalls, Object: "db/repo.go:QueryUser"})
	_ = src.AddFact(meb.Fact{Subject: "db/client.go:RunQuery", Predicate: config.PredicateCalls, Object: "db/repo.go:QueryUser"})

	mgr := &securityMgr{src: src, ana: ana}
	analyzer := NewAnalyzer(mgr, nil)
	if err := analyzer.detectSecuritySmells(context.Background(), "proj"); err != nil {
		t.Fatalf("detectSecuritySmells failed: %v", err)
	}

	unsanitized := map[string]bool{}
	for f := range ana.ScanContext(context.Background(), "", "has_smell_type", "unsanitized_db_access") {
		if f.Subject != "" {
			unsanitized[f.Subject] = true
		}
	}
	if len(unsanitized) != 1 || !unsanitized["api/handlers.go"] {
		t.Errorf("expected unsanitized_db_access on api/handlers.go only, got %v", unsanitized)
	}

	missing := map[string]bool{}
	for f := range ana.ScanContext(context.Background(), "", "has_smell_type", "missing_error_check") {
		if f.Subject != "" {
			missing[f.Subject] = true
		}
	}
	if len(missing) != 1 || !missing["db/client.go"] {
		t.Errorf("expected missing_error_check on db/client.go only, got %v", missing)
	}

	// Category + severity facts written for both.
	for _, smell := range []string{"unsanitized_db_access", "missing_error_check"} {
		ok := false
		for f := range ana.Scan("", "has_smell_category", "security") {
			if f.Subject != "" {
				ok = true
			}
		}
		if !ok {
			t.Errorf("expected has_smell_category=security for %s", smell)
		}
	}
}
