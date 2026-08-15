package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

// reportMgr provides separate source/analytical stores for report tests.
type reportMgr struct {
	src *meb.MEBStore
	ana *meb.MEBStore
}

func (m *reportMgr) GetStore(projectID string) (*meb.MEBStore, error) { return m.src, nil }
func (m *reportMgr) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	return m.src, nil
}
func (m *reportMgr) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return m.ana, nil
}
func (m *reportMgr) ListProjects() ([]manager.ProjectMetadata, error) { return nil, nil }

func newReportStores(t *testing.T) (src, ana *meb.MEBStore, cleanup func()) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "report_src")
	if err != nil {
		t.Fatal(err)
	}
	anaDir, err := os.MkdirTemp("", "report_ana")
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

func TestReportService_GenerateMarkdown_AllSections(t *testing.T) {
	src, ana, cleanup := newReportStores(t)
	defer cleanup()

	// Source graph
	_ = src.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateDefines, Object: "main"})
	_ = src.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateImports, Object: "fmt"})
	_ = src.AddFact(meb.Fact{Subject: "main", Predicate: config.PredicateCalls, Object: "helper"})
	_ = src.AddFact(meb.Fact{Subject: "main.go", Predicate: config.PredicateCalls, Object: "main:helper"})

	// Analytical facts
	_ = ana.AddFact(meb.Fact{Subject: "main.go", Predicate: "is_entry_point", Object: "true"})
	_ = ana.AddFact(meb.Fact{Subject: "hub.go", Predicate: "has_hub_score", Object: "0.9"})
	_ = ana.AddFact(meb.Fact{Subject: "big.go", Predicate: "has_smell_type", Object: "god_file"})
	_ = ana.AddFact(meb.Fact{Subject: "a.go", Predicate: "belongs_to_cluster", Object: "cluster_0"})

	// KPI snapshot for overview
	snap := ingest.KPISnapshot{
		ID: "kpi:proj:aaaa", Timestamp: time.Now(), HealthScore: 88, HealthDebt: 50,
		SmellCount: 3, TopSmell: "god_file",
	}
	body, _ := json.Marshal(snap)
	_ = ana.AddFact(meb.Fact{Subject: snap.ID, Predicate: config.PredicateKPISnapshot, Object: string(body)})

	svc := NewReportService(&reportMgr{src: src, ana: ana}, nil)
	md, err := svc.GenerateMarkdown(context.Background(), ReportOptions{ProjectID: "proj"})
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}

	for _, section := range []string{"## Overview", "## Entry Points", "## Hubs", "## Smells", "## Clusters", "## Call Flows", "## OKF Concepts"} {
		if !strings.Contains(md, section) {
			t.Errorf("missing section %q in report", section)
		}
	}
	if !strings.Contains(md, "god_file") || !strings.Contains(md, "main.go") || !strings.Contains(md, "cluster_0") {
		t.Errorf("report missing expected content")
	}
	if !strings.Contains(md, "88") {
		t.Errorf("report missing health score")
	}
}

func TestReportService_GenerateMarkdown_SectionFilter(t *testing.T) {
	src, ana, cleanup := newReportStores(t)
	defer cleanup()

	_ = ana.AddFact(meb.Fact{Subject: "big.go", Predicate: "has_smell_type", Object: "god_file"})

	svc := NewReportService(&reportMgr{src: src, ana: ana}, nil)
	md, err := svc.GenerateMarkdown(context.Background(), ReportOptions{ProjectID: "proj", Sections: []string{"smells"}})
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}
	if !strings.Contains(md, "## Smells") {
		t.Error("smells section should be present")
	}
	if strings.Contains(md, "## Entry Points") {
		t.Error("entry_points section should be excluded when filtered")
	}
}

func TestReportService_GenerateMarkdown_RequiresProject(t *testing.T) {
	svc := NewReportService(nil, nil)
	if _, err := svc.GenerateMarkdown(context.Background(), ReportOptions{ProjectID: ""}); err == nil {
		t.Error("expected error for empty project_id")
	}
}
