package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func seededStore(t *testing.T, snaps ...ingest.KPISnapshot) *meb.MEBStore {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "trend_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	for _, snap := range snaps {
		if snap.ID == "" {
			snap.ID = snapCommitIDForTest(snap.Timestamp)
		}
		body, err := json.Marshal(snap)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AddFact(meb.Fact{Subject: snap.ID, Predicate: config.PredicateKPISnapshot, Object: string(body)}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func snapCommitIDForTest(ts time.Time) string {
	return "kpi:test:t" + ts.Format("150405.000000000")
}

func TestTrendService_ListTrends_OrderAndSummary(t *testing.T) {
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	snaps := []ingest.KPISnapshot{
		{Timestamp: base, CommitSHA: "aaaa", HealthScore: 80, HealthDebt: 100, SmellCount: 5},
		{Timestamp: base.Add(24 * time.Hour), CommitSHA: "bbbb", HealthScore: 85, HealthDebt: 90, SmellCount: 4},
		{Timestamp: base.Add(48 * time.Hour), CommitSHA: "cccc", HealthScore: 90, HealthDebt: 70, SmellCount: 3},
	}
	s := seededStore(t, snaps...)
	svc := NewTrendService(&MockStoreManager{store: s})

	resp, err := svc.ListTrends(context.Background(), "proj", "health", "", "")
	if err != nil {
		t.Fatalf("ListTrends failed: %v", err)
	}
	if resp.ProjectID != "proj" || resp.Metric != "health" {
		t.Errorf("project/metric mismatch: %+v", resp)
	}
	if len(resp.Points) != 3 {
		t.Fatalf("want 3 points, got %d", len(resp.Points))
	}
	// Chronological order despite insertion order.
	got := []int{resp.Points[0].Value, resp.Points[1].Value, resp.Points[2].Value}
	want := []int{80, 85, 90}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("points[%d].value = %d, want %d", i, got[i], want[i])
		}
	}
	if resp.Summary.Start != 80 || resp.Summary.End != 90 || resp.Summary.Delta != 10 {
		t.Errorf("summary mismatch: %+v", resp.Summary)
	}
	if resp.Summary.Trend != "improving" {
		t.Errorf("trend = %q, want improving", resp.Summary.Trend)
	}
}

func TestTrendService_ListTrends_Filters(t *testing.T) {
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	snaps := []ingest.KPISnapshot{
		{Timestamp: base.Add(-48 * time.Hour), HealthScore: 50},
		{Timestamp: base, HealthScore: 60},
		{Timestamp: base.Add(48 * time.Hour), HealthScore: 70},
	}
	s := seededStore(t, snaps...)
	svc := NewTrendService(&MockStoreManager{store: s})

	from := base.Add(-12 * time.Hour).Format(time.RFC3339)
	to := base.Add(12 * time.Hour).Format(time.RFC3339)
	resp, err := svc.ListTrends(context.Background(), "proj", "health", from, to)
	if err != nil {
		t.Fatalf("ListTrends failed: %v", err)
	}
	if len(resp.Points) != 1 {
		t.Fatalf("filtered want 1 point, got %d", len(resp.Points))
	}
	if resp.Points[0].Value != 60 {
		t.Errorf("filtered point value = %d, want 60", resp.Points[0].Value)
	}
}

func TestTrendService_ListTrends_UnsupportedMetric(t *testing.T) {
	s := seededStore(t, ingest.KPISnapshot{Timestamp: time.Now(), HealthScore: 1})
	svc := NewTrendService(&MockStoreManager{store: s})
	if _, err := svc.ListTrends(context.Background(), "proj", "bogus", "", ""); err == nil {
		t.Error("expected error for unsupported metric")
	}
}

func TestTrendService_ListTrends_MetricsMapping(t *testing.T) {
	snaps := []ingest.KPISnapshot{
		{
			Timestamp:       time.Now(),
			HealthScore:     70,
			HealthDebt:      200,
			SmellCount:      10,
			DeadCodeCount:   2,
			ComplexityCount: 3,
			DuplicateCount:  4,
		},
	}
	s := seededStore(t, snaps...)
	svc := NewTrendService(&MockStoreManager{store: s})

	cases := map[string]int{
		"health":      70,
		"debt":        200,
		"smell_count": 10,
		"dead_code":   2,
		"complexity":  3,
		"duplicate":   4,
	}
	for metric, want := range cases {
		resp, err := svc.ListTrends(context.Background(), "proj", metric, "", "")
		if err != nil {
			t.Fatalf("metric %s failed: %v", metric, err)
		}
		if len(resp.Points) != 1 || resp.Points[0].Value != want {
			t.Errorf("metric %s = %d, want %d", metric, resp.Points[0].Value, want)
		}
	}
}
