package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
)

// TrendService reads KPI snapshots written by the analyzer (F2) and renders
// health-over-time series.
type TrendService struct {
	manager ProjectStoreManager
}

// NewTrendService creates a TrendService over a project store manager.
func NewTrendService(manager ProjectStoreManager) *TrendService {
	return &TrendService{manager: manager}
}

// TrendPoint is a single point in a trend series.
type TrendPoint struct {
	Commit    string    `json:"commit,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Value     int       `json:"value"`
}

// TrendResponse is the JSON shape returned by GET /api/v1/trends.
type TrendResponse struct {
	ProjectID string       `json:"project_id"`
	Metric    string       `json:"metric"`
	Points    []TrendPoint `json:"points"`
	Summary   TrendSummary `json:"summary"`
}

// TrendSummary summarizes the first→last deltas of a series.
type TrendSummary struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Delta int    `json:"delta"`
	Trend string `json:"trend"`
}

// ListTrends returns a time series for the requested metric.
// Supported metrics: health, debt, smell_count, dead_code, complexity, duplicate.
// Optional from/to filter the timestamps (RFC3339); empty means unbounded.
func (s *TrendService) ListTrends(ctx context.Context, projectID, metric, from, to string) (*TrendResponse, error) {
	store, err := s.manager.GetStore(projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	metric = normalizeMetric(metric)
	if metric == "" {
		return nil, fmt.Errorf("unsupported metric")
	}

	var fromT, toT time.Time
	if from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			fromT = t
		}
	}
	if to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			toT = t
		}
	}

	var snaps []ingest.KPISnapshot
	for fact := range store.ScanContext(ctx, "", config.PredicateKPISnapshot, "") {
		if fact.Subject == "" {
			continue
		}
		obj, ok := fact.Object.(string)
		if !ok {
			continue
		}
		var snap ingest.KPISnapshot
		if err := json.Unmarshal([]byte(obj), &snap); err != nil {
			continue
		}
		if !fromT.IsZero() && snap.Timestamp.Before(fromT) {
			continue
		}
		if !toT.IsZero() && snap.Timestamp.After(toT) {
			continue
		}
		snaps = append(snaps, snap)
	}

	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Timestamp.Before(snaps[j].Timestamp) })

	resp := &TrendResponse{
		ProjectID: projectID,
		Metric:    metric,
		Points:    make([]TrendPoint, 0, len(snaps)),
	}
	for _, s := range snaps {
		resp.Points = append(resp.Points, TrendPoint{
			Commit:    s.CommitSHA,
			Timestamp: s.Timestamp,
			Value:     kpiValue(s, metric),
		})
	}

	if len(resp.Points) > 0 {
		resp.Summary.Start = resp.Points[0].Value
		resp.Summary.End = resp.Points[len(resp.Points)-1].Value
		resp.Summary.Delta = resp.Summary.End - resp.Summary.Start
		switch {
		case resp.Summary.Delta > 0:
			resp.Summary.Trend = "improving"
		case resp.Summary.Delta < 0:
			resp.Summary.Trend = "declining"
		default:
			resp.Summary.Trend = "flat"
		}
	}

	return resp, nil
}

// normalizeMetric validates and canonicalizes a metric name.
func normalizeMetric(m string) string {
	switch m {
	case "health", "debt", "smell_count", "dead_code", "complexity", "duplicate":
		return m
	case "smells":
		return "smell_count"
	default:
		return ""
	}
}

// kpiValue extracts the scalar value for a metric from a snapshot.
func kpiValue(s ingest.KPISnapshot, metric string) int {
	switch metric {
	case "health":
		return s.HealthScore
	case "debt":
		return s.HealthDebt
	case "smell_count":
		return s.SmellCount
	case "dead_code":
		return s.DeadCodeCount
	case "complexity":
		return s.ComplexityCount
	case "duplicate":
		return s.DuplicateCount
	default:
		return 0
	}
}
