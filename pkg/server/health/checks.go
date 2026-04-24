package health

import (
	"context"
	"time"

	"github.com/duynguyendang/meb"
)

type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

type HealthResult struct {
	Check    string       `json:"check"`
	Status   HealthStatus `json:"status"`
	Message  string       `json:"message,omitempty"`
	Duration string       `json:"duration"`
}

type HealthCheck interface {
	Name() string
	Check(ctx context.Context, store *meb.MEBStore) *HealthResult
}

type SmellCheck struct{}

func (c *SmellCheck) Name() string { return "smell_detection" }

func (c *SmellCheck) Check(ctx context.Context, store *meb.MEBStore) *HealthResult {
	start := time.Now()
	result := &HealthResult{Check: c.Name()}

	count := 0
	for range store.Scan("", "has_smell", "") {
		count++
	}

	result.Duration = time.Since(start).String()
	if count > 100 {
		result.Status = StatusDegraded
		result.Message = "High number of smells detected"
	} else {
		result.Status = StatusHealthy
		result.Message = "Smell detection operational"
	}
	return result
}

type HubScoreCheck struct{}

func (c *HubScoreCheck) Name() string { return "hub_score_calculation" }

func (c *HubScoreCheck) Check(ctx context.Context, store *meb.MEBStore) *HealthResult {
	start := time.Now()
	result := &HealthResult{Check: c.Name()}

	count := 0
	for range store.Scan("", "has_hub_score", "") {
		count++
	}

	result.Duration = time.Since(start).String()
	if count == 0 {
		result.Status = StatusHealthy
		result.Message = "No hub anomalies detected"
	} else {
		result.Status = StatusDegraded
		result.Message = "Hub anomalies detected"
	}
	return result
}

type EntryPointCheck struct{}

func (c *EntryPointCheck) Name() string { return "entry_point_detection" }

func (c *EntryPointCheck) Check(ctx context.Context, store *meb.MEBStore) *HealthResult {
	start := time.Now()
	result := &HealthResult{Check: c.Name()}

	count := 0
	for range store.Scan("", "is_entry_point", "true") {
		count++
	}

	result.Duration = time.Since(start).String()
	if count == 0 {
		result.Status = StatusUnhealthy
		result.Message = "No entry points found - possible data issue"
	} else {
		result.Status = StatusHealthy
		result.Message = "Entry points detected"
	}
	return result
}

type Registry struct {
	checks map[string]HealthCheck
}

func NewRegistry() *Registry {
	return &Registry{
		checks: make(map[string]HealthCheck),
	}
}

func (r *Registry) Register(check HealthCheck) {
	r.checks[check.Name()] = check
}

func (r *Registry) RunAll(ctx context.Context, store *meb.MEBStore) []*HealthResult {
	results := make([]*HealthResult, 0, len(r.checks))
	for _, check := range r.checks {
		result := check.Check(ctx, store)
		results = append(results, result)
	}
	return results
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&SmellCheck{})
	r.Register(&HubScoreCheck{})
	r.Register(&EntryPointCheck{})
	return r
}
