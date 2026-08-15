package compliance

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func newComplianceStores(t *testing.T) (src, ana *meb.MEBStore, cleanup func()) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "compliance_src")
	if err != nil {
		t.Fatal(err)
	}
	anaDir, err := os.MkdirTemp("", "compliance_ana")
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

func TestSnapshotRoundTrip(t *testing.T) {
	path := t.TempDir() + "/osv.json"
	snap := &Snapshot{
		Generated: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
		Packages: map[string][]Advisory{
			"github.com/gin-gonic/gin": {
				{ID: "GHSA-xxxx", Summary: "smuggling", Severity: "high", AffectedVersions: "< 1.9.0"},
			},
		},
	}
	if err := SaveSnapshot(path, snap); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}
	if len(loaded.Packages) != 1 {
		t.Fatalf("packages = %d, want 1", len(loaded.Packages))
	}
	adv := loaded.Lookup("github.com/gin-gonic/gin")
	if len(adv) != 1 || adv[0].ID != "GHSA-xxxx" || adv[0].Severity != "high" {
		t.Errorf("advisory mismatch: %+v", adv)
	}
	if loaded.SnapshotDate() != "2026-08-12T00:00:00Z" {
		t.Errorf("snapshot date = %q", loaded.SnapshotDate())
	}
}

func TestLoadSnapshot_Missing(t *testing.T) {
	if _, err := LoadSnapshot("/nonexistent/osv.json"); err == nil {
		t.Error("expected error for missing snapshot")
	}
}

func TestCollectInventory_DedupeAndCanonicalize(t *testing.T) {
	src, _, cleanup := newComplianceStores(t)
	defer cleanup()

	_ = src.AddFact(meb.Fact{Subject: "a.go", Predicate: "imports", Object: "github.com/foo/bar"})
	_ = src.AddFact(meb.Fact{Subject: "b.go", Predicate: "imports", Object: "github.com/foo/bar"})
	_ = src.AddFact(meb.Fact{Subject: "c.go", Predicate: "imports", Object: "github.com/foo/bar/v2"})
	_ = src.AddFact(meb.Fact{Subject: "d.go", Predicate: "imports", Object: "net/http"})
	_ = src.AddFact(meb.Fact{Subject: "e.go", Predicate: "imports", Object: "./local/mod"})

	inv, err := CollectInventory(context.Background(), src)
	if err != nil {
		t.Fatalf("CollectInventory failed: %v", err)
	}
	if inv.PackageCount != 2 {
		t.Fatalf("package_count = %d, want 2 (foo/bar deduped with v2; local dropped)", inv.PackageCount)
	}
	found := map[string]*Dependency{}
	for i := range inv.Dependencies {
		found[inv.Dependencies[i].Name] = &inv.Dependencies[i]
	}
	if dep := found["github.com/foo/bar"]; dep == nil {
		t.Error("expected github.com/foo/bar with v2 folded in")
	} else if len(dep.Files) != 3 {
		t.Errorf("foo/bar files = %d, want 3", len(dep.Files))
	}
}

func TestMatch_WritesVulnerabilityFacts(t *testing.T) {
	src, ana, cleanup := newComplianceStores(t)
	defer cleanup()

	_ = src.AddFact(meb.Fact{Subject: "main.go", Predicate: "imports", Object: "github.com/gin-gonic/gin"})

	snap := &Snapshot{
		Generated: time.Now(),
		Packages: map[string][]Advisory{
			"github.com/gin-gonic/gin": {
				{ID: "GHSA-xxxx", Summary: "smuggling", Severity: "high"},
				{ID: "GHSA-yyyy", Summary: "dos", Severity: "medium"},
			},
			"github.com/unused/pkg": {
				{ID: "GHSA-zzzz", Severity: "critical"},
			},
		},
	}

	result, err := Match(context.Background(), src, ana, snap)
	if err != nil {
		t.Fatalf("Match failed: %v", err)
	}
	if result.MatchedPackages != 1 {
		t.Errorf("matched packages = %d, want 1", result.MatchedPackages)
	}
	if len(result.Vulnerabilities) != 2 {
		t.Fatalf("vulnerabilities = %d, want 2", len(result.Vulnerabilities))
	}

	// Verify facts in the analytical store.
	sevCount, vulnCount := 0, 0
	for fact := range ana.ScanContext(context.Background(), "", PredicateHasVulnerability, "") {
		if fact.Subject != "" {
			vulnCount++
		}
	}
	for fact := range ana.ScanContext(context.Background(), "", PredicateVulnSeverity, "") {
		if fact.Subject != "" {
			sevCount++
		}
	}
	if vulnCount != 2 {
		t.Errorf("has_vulnerability facts = %d, want 2", vulnCount)
	}
	if sevCount != 2 {
		t.Errorf("vuln_severity facts = %d, want 2", sevCount)
	}
}

func TestCanonicalizeImport(t *testing.T) {
	cases := map[string]string{
		`"github.com/foo/bar"`:  "github.com/foo/bar",
		"github.com/foo/bar/v2": "github.com/foo/bar",
		"net/http":              "net/http",
		"./local/mod":           "",
		"/abs/path":             "",
		"":                      "",
	}
	for in, want := range cases {
		if got := CanonicalizeImport(in); got != want {
			t.Errorf("CanonicalizeImport(%q) = %q, want %q", in, got, want)
		}
	}
}
