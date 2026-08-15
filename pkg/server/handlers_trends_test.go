package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/meb"
)

func TestHandleTrends(t *testing.T) {
	srv, s, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	// Seed KPI snapshots on the analytical store (same underlying store).
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	snaps := []ingest.KPISnapshot{
		{ID: "kpi:testproj:aaaa", CommitSHA: "aaaa", Timestamp: base, HealthScore: 80, HealthDebt: 100, SmellCount: 5},
		{ID: "kpi:testproj:bbbb", CommitSHA: "bbbb", Timestamp: base.Add(24 * time.Hour), HealthScore: 90, HealthDebt: 60, SmellCount: 3},
	}
	for _, snap := range snaps {
		body, _ := json.Marshal(snap)
		if err := s.AddFact(meb.Fact{Subject: snap.ID, Predicate: config.PredicateKPISnapshot, Object: string(body)}); err != nil {
			t.Fatal(err)
		}
	}

	w := doRequest(srv, "GET", "/api/v1/trends?project=testproj&metric=health", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		ProjectID string `json:"project_id"`
		Metric    string `json:"metric"`
		Points    []struct {
			Commit string `json:"commit"`
			Value  int    `json:"value"`
		} `json:"points"`
		Summary struct {
			Start int    `json:"start"`
			End   int    `json:"end"`
			Delta int    `json:"delta"`
			Trend string `json:"trend"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, w.Body.String())
	}
	if resp.ProjectID != "testproj" || resp.Metric != "health" {
		t.Errorf("project/metric mismatch: %+v", resp)
	}
	if len(resp.Points) != 2 {
		t.Fatalf("want 2 points, got %d", len(resp.Points))
	}
	if resp.Points[0].Value != 80 || resp.Points[1].Value != 90 {
		t.Errorf("point values = %d,%d want 80,90", resp.Points[0].Value, resp.Points[1].Value)
	}
	if resp.Summary.Start != 80 || resp.Summary.End != 90 || resp.Summary.Delta != 10 {
		t.Errorf("summary mismatch: %+v", resp.Summary)
	}
	if resp.Summary.Trend != "improving" {
		t.Errorf("trend = %q, want improving", resp.Summary.Trend)
	}
}

func TestHandleTrends_MissingProject(t *testing.T) {
	srv, _, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	w := doRequest(srv, "GET", "/api/v1/trends?metric=health", "")
	if w.Code != 200 && w.Code != 400 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
func TestHandleArchitectureReport(t *testing.T) {
	srv, s, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	_ = s.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateDefines, Object: "main"})
	_ = s.AddFact(meb.Fact{Subject: "big.go", Predicate: "has_smell_type", Object: "god_file"})
	_ = s.AddFact(meb.Fact{Subject: "main.go", Predicate: "is_entry_point", Object: "true"})

	body := `{"project_id":"testproj","sections":["overview","smells"]}`
	w := doJSONRequest(srv, "POST", "/api/v1/report/architecture", body)
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "## Overview") || !strings.Contains(w.Body.String(), "god_file") {
		t.Errorf("report missing expected content:\n%s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "## Entry Points") {
		t.Errorf("section filter not honored")
	}
}

func TestHandleArchitectureReport_MissingProject(t *testing.T) {
	srv, _, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	w := doJSONRequest(srv, "POST", "/api/v1/report/architecture", `{}`)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

func TestHandleImpactReport(t *testing.T) {
	srv, s, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	_ = s.AddFact(meb.Fact{Subject: "store.go", Predicate: config.PredicateDefines, Object: "Store"})
	_ = s.AddFact(meb.Fact{Subject: "store.go", Predicate: "has_hub_score", Object: "0.9"})
	_ = s.AddFact(meb.Fact{Subject: "store.go", Predicate: "has_smell_type", Object: "god_file"})

	diff := "diff --git a/store.go b/store.go\n--- a/store.go\n+++ b/store.go\n@@ -1,2 +1,3 @@\n func Store() {\n+    x()\n }\n"
	body := `{"project_id":"testproj","diff":"` + escapeJSON(diff) + `"}`
	w := doJSONRequest(srv, "POST", "/api/v1/impact/report", body)
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		SessionID      string `json:"session_id"`
		TouchedFileCnt int    `json:"touched_file_count"`
		Hubs           int    `json:"-"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, w.Body.String())
	}
	if resp.SessionID == "" {
		t.Error("expected session_id in response")
	}
	if resp.TouchedFileCnt != 1 {
		t.Errorf("touched_file_count = %d, want 1", resp.TouchedFileCnt)
	}
}

func TestHandleImpactReport_Validation(t *testing.T) {
	srv, _, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	w := doJSONRequest(srv, "POST", "/api/v1/impact/report", `{}`)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
}

func TestHandleComplianceVulnerabilities(t *testing.T) {
	srv, s, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	_ = s.AddFact(meb.Fact{Subject: "pkg/gin", Predicate: "imports", Object: "github.com/gin-gonic/gin"})
	_ = s.AddFact(meb.Fact{Subject: "github.com/gin-gonic/gin", Predicate: "has_vulnerability", Object: "GHSA-xxxx"})
	_ = s.AddFact(meb.Fact{Subject: "GHSA-xxxx", Predicate: "vuln_severity", Object: "high"})
	_ = s.AddFact(meb.Fact{Subject: "GHSA-xxxx", Predicate: "vuln_summary", Object: "smuggling"})

	w := doRequest(srv, "GET", "/api/v1/compliance/vulnerabilities?project=testproj", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Vulns []struct {
			Package    string `json:"package"`
			AdvisoryID string `json:"advisory_id"`
			Severity   string `json:"severity"`
		} `json:"vulnerabilities"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, w.Body.String())
	}
	if resp.Total != 1 || len(resp.Vulns) != 1 {
		t.Fatalf("total = %d, vulns = %d", resp.Total, len(resp.Vulns))
	}
	if resp.Vulns[0].Package != "github.com/gin-gonic/gin" || resp.Vulns[0].Severity != "high" {
		t.Errorf("vuln mismatch: %+v", resp.Vulns[0])
	}

	// Severity filter.
	w = doRequest(srv, "GET", "/api/v1/compliance/vulnerabilities?project=testproj&severity=critical", "")
	var filtered struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if filtered.Total != 0 {
		t.Errorf("severity-filtered total = %d, want 0", filtered.Total)
	}
}

func TestHandleComplianceSBOM(t *testing.T) {
	srv, s, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	_ = s.AddFact(meb.Fact{Subject: "a.go", Predicate: "imports", Object: "github.com/foo/bar"})
	_ = s.AddFact(meb.Fact{Subject: "b.go", Predicate: "imports", Object: "net/http"})

	w := doRequest(srv, "GET", "/api/v1/compliance/sbom?project=testproj", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		PackageCount int `json:"package_count"`
		Deps         []struct {
			Name string `json:"name"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, w.Body.String())
	}
	if resp.PackageCount != 2 || len(resp.Deps) != 2 {
		t.Fatalf("package_count = %d, deps = %d", resp.PackageCount, len(resp.Deps))
	}

	// CycloneDX format.
	w = doRequest(srv, "GET", "/api/v1/compliance/sbom?project=testproj&format=cyclonedx", "")
	if w.Code != 200 {
		t.Fatalf("cyclonedx status = %d", w.Code)
	}
	var cd struct {
		BOMFormat string `json:"bomFormat"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cd); err != nil {
		t.Fatalf("cyclonedx JSON: %v", err)
	}
	if cd.BOMFormat != "CycloneDX" {
		t.Errorf("bomFormat = %q", cd.BOMFormat)
	}
}

func TestHandleCompliance_MissingProject(t *testing.T) {
	srv, _, cleanup := setupTestServer(t, testServerConfig{SkipTestData: true, SkipSmellData: true})
	defer cleanup()

	for _, path := range []string{
		"/api/v1/compliance/vulnerabilities",
		"/api/v1/compliance/sbom",
	} {
		w := doRequest(srv, "GET", path, "")
		if w.Code != 400 {
			t.Errorf("%s status = %d, want 400", path, w.Code)
		}
	}
}
