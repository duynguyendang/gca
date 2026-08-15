package service

import (
	"context"
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ephemeral"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func newImpactStores(t *testing.T) (src, ana *meb.MEBStore, cleanup func()) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "impact_src")
	if err != nil {
		t.Fatal(err)
	}
	anaDir, err := os.MkdirTemp("", "impact_ana")
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

func TestImpactReportService_Generate(t *testing.T) {
	src, ana, cleanup := newImpactStores(t)
	defer cleanup()

	// Source graph: "store.go" defines Store; "main.go" defines main and calls Store.
	_ = src.AddFact(meb.Fact{Subject: "pkg/db/store.go", Predicate: config.PredicateDefines, Object: "Store"})
	_ = src.AddFact(meb.Fact{Subject: "cmd/main.go", Predicate: config.PredicateDefines, Object: "main"})
	_ = src.AddFact(meb.Fact{Subject: "main", Predicate: config.PredicateCalls, Object: "Store"})

	// Analytical: store.go is a hub and entry point; store.go has a smell.
	_ = ana.AddFact(meb.Fact{Subject: "pkg/db/store.go", Predicate: "has_hub_score", Object: "0.85"})
	_ = ana.AddFact(meb.Fact{Subject: "cmd/main.go", Predicate: "is_entry_point", Object: "true"})
	_ = ana.AddFact(meb.Fact{Subject: "pkg/db/store.go", Predicate: "has_smell_type", Object: "god_file"})

	diff := "diff --git a/pkg/db/store.go b/pkg/db/store.go\n" +
		"--- a/pkg/db/store.go\n+++ b/pkg/db/store.go\n" +
		"@@ -1,3 +1,4 @@\n func Store() {\n+    newCode()\n }\n"

	es := ephemeral.NewEphemeralStore(0)
	defer es.Close()

	svc := NewImpactReportService(es, &reportMgr{src: src, ana: ana}, nil)
	report, err := svc.Generate(context.Background(), "proj", diff)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if report.SessionID == "" {
		t.Error("expected session id")
	}
	if len(report.TouchedFiles) != 1 || report.TouchedFiles[0] != "pkg/db/store.go" {
		t.Errorf("touched files = %v, want [pkg/db/store.go]", report.TouchedFiles)
	}
	if len(report.HubFilesHit) != 1 || report.HubFilesHit[0] != "pkg/db/store.go" {
		t.Errorf("hub files hit = %v", report.HubFilesHit)
	}
	if len(report.SmellsPreExisting) != 1 || report.SmellsPreExisting["god_file"] != 1 {
		t.Errorf("pre-existing smells = %v", report.SmellsPreExisting)
	}
	// "Store" is touched (defined in touched file). main calls Store.
	if report.ReachableCallersCount != 1 {
		t.Errorf("reachable callers = %d, want 1", report.ReachableCallersCount)
	}
	// Session must be cleaned up.
	if _, err := es.GetSession(report.SessionID); err == nil {
		t.Error("session should be expired/deleted after report generation")
	}
}

func TestImpactReportService_Generate_RequiresInputs(t *testing.T) {
	es := ephemeral.NewEphemeralStore(0)
	defer es.Close()
	svc := NewImpactReportService(es, &reportMgr{}, nil)

	if _, err := svc.Generate(context.Background(), "", "diff"); err == nil {
		t.Error("expected error for empty project_id")
	}
	if _, err := svc.Generate(context.Background(), "proj", ""); err == nil {
		t.Error("expected error for empty diff")
	}
}

func TestImpactReport_BlockedGate(t *testing.T) {
	r := &ImpactReport{SmellsNew: map[string]int{"dead_code": 3}}
	if r.SmellsNewCount() != 3 {
		t.Errorf("SmellsNewCount = %d, want 3", r.SmellsNewCount())
	}
}
