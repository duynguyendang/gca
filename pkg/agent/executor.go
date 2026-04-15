package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/circuit"
)

var stepRefRegex = regexp.MustCompile(`\{\{step_(\d+)_result\}\}`)

const DefaultQueryTimeout = 2 * time.Second

func WithQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, DefaultQueryTimeout)
}

type Executor struct {
	store *meb.MEBStore
}

func NewExecutor(store *meb.MEBStore) *Executor {
	store.SetCircuitBreakerConfig(&circuit.Config{
		QueryTimeout:     30 * time.Second,
		FailureThreshold: 5,
		SuccessThreshold: 3,
		OpenDuration:     30 * time.Second,
		MaxJoinResults:   5000,
	})
	return &Executor{
		store: store,
	}
}

func (e *Executor) ExecuteStep(ctx context.Context, session *ExecutionSession, stepIndex int) error {
	step := session.GetStep(stepIndex)
	if step == nil {
		return fmt.Errorf("step %d not found", stepIndex)
	}

	session.UpdateStep(stepIndex, func(s *PlanStep) {
		s.Status = StepStatusRunning
		now := time.Now()
		s.StartTime = &now
	})

	resolvedQuery := e.resolveVariables(step.Query, session, stepIndex)

	queryCtx, cancel := WithQueryTimeout(ctx)
	defer cancel()

	logger.Debug("Agent/Executor executing query", "stepIndex", stepIndex, "query", resolvedQuery)

	var results []map[string]any

	err := e.store.CircuitBreaker().ExecuteContext(queryCtx, func() error {
		r, err := gcamdb.Query(queryCtx, e.store, resolvedQuery)
		if err != nil {
			return err
		}
		results = r
		return nil
	})

	if err != nil {
		session.UpdateStep(stepIndex, func(s *PlanStep) {
			s.Status = StepStatusFailed
			s.Error = err.Error()
			now := time.Now()
			s.EndTime = &now
		})
		return fmt.Errorf("step %d query failed: %w", stepIndex, err)
	}

	hydrated := e.hydrateResults(ctx, results, 10)

	now := time.Now()
	session.UpdateStep(stepIndex, func(s *PlanStep) {
		s.Status = StepStatusSuccess
		s.Result = results
		s.Hydrated = hydrated
		s.EndTime = &now
	})

	logger.Debug("Agent/Executor step results", "stepIndex", stepIndex, "results", len(results), "hydrated", len(hydrated))
	return nil
}

func (e *Executor) resolveVariables(query string, session *ExecutionSession, currentIndex int) string {
	return stepRefRegex.ReplaceAllStringFunc(query, func(match string) string {
		m := stepRefRegex.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		refIndex := 0
		fmt.Sscanf(m[1], "%d", &refIndex)

		prevStep := session.GetStep(refIndex)
		if prevStep == nil || len(prevStep.Result) == 0 {
			return match
		}

		for _, val := range prevStep.Result[0] {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
		return match
	})
}

func (e *Executor) hydrateResults(ctx context.Context, results []map[string]any, maxCount int) []HydratedNode {
	seen := make(map[string]bool)
	var hydrated []HydratedNode

	for _, row := range results {
		if len(hydrated) >= maxCount {
			break
		}

		id := e.extractID(row)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		node := HydratedNode{ID: id, Metadata: make(map[string]string)}

		if name, ok := row["?s"].(string); ok {
			node.Name = name
		} else if name, ok := row["?o"].(string); ok {
			node.Name = name
		} else {
			parts := strings.Split(id, ":")
			node.Name = parts[len(parts)-1]
		}

		content, err := e.store.GetContentByKey(id)

		if err == nil && len(content) > 0 {
			code := string(content)
			if len(code) > 2000 {
				code = code[:2000] + "\n... (truncated)"
			}
			node.Code = code
		}

		kindCtx, cancel2 := context.WithTimeout(ctx, 500*time.Millisecond)
		kindResults, _ := gcamdb.Query(kindCtx, e.store, fmt.Sprintf(`triples("%s", "kind", ?o)`, id))
		cancel2()
		if len(kindResults) > 0 {
			if kind, ok := kindResults[0]["?o"].(string); ok {
				node.Kind = kind
			}
		}

		hydrated = append(hydrated, node)
	}

	return hydrated
}

func (e *Executor) extractID(row map[string]any) string {
	if s, ok := row["?s"].(string); ok && s != "" {
		return s
	}
	if o, ok := row["?o"].(string); ok && o != "" {
		return o
	}
	for _, v := range row {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func (e *Executor) ExecuteAllSteps(ctx context.Context, session *ExecutionSession) error {
	for i := 0; i < len(session.Steps); i++ {
		if err := e.ExecuteStep(ctx, session, i); err != nil {
			logger.Warn("Agent/Executor step failed", "stepIndex", i, "error", err)
			if corrected := e.attemptCorrection(ctx, session, i); corrected {
				continue
			}
			return fmt.Errorf("step %d failed and could not be corrected: %w", i, err)
		}
	}
	return nil
}

func (e *Executor) attemptCorrection(ctx context.Context, session *ExecutionSession, stepIndex int) bool {
	step := session.GetStep(stepIndex)
	if step == nil {
		return false
	}

	logger.Debug("Agent/Executor attempting correction", "stepIndex", stepIndex, "query", step.Query)

	originalQuery := step.Query

	corrected := regexp.MustCompile(`"([^"]+)"`).ReplaceAllString(originalQuery, `?o`)

	if corrected == originalQuery {
		corrected = `triples(?s, ?p, ?o) LIMIT 5`
	}

	logger.Debug("Agent/Executor corrected query", "query", corrected)

	session.UpdateStep(stepIndex, func(s *PlanStep) {
		s.Query = corrected
		s.Error = ""
		s.Status = StepStatusPending
	})

	if err := e.ExecuteStep(ctx, session, stepIndex); err == nil {
		session.UpdateStep(stepIndex, func(s *PlanStep) {
			s.Status = StepStatusCorrected
		})
		return true
	}

	session.UpdateStep(stepIndex, func(s *PlanStep) {
		s.Query = originalQuery
		s.Status = StepStatusFailed
	})
	return false
}
