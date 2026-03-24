package agent

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/duynguyendang/meb"
)

var stepRefRegex = regexp.MustCompile(`\{\{step_(\d+)_result\}\}`)

// Executor runs PlanSteps against the MEB store with variable injection and hydration.
type Executor struct {
	store          *meb.MEBStore
	circuitBreaker *CircuitBreaker
}

// NewExecutor creates an executor bound to the given store.
func NewExecutor(store *meb.MEBStore) *Executor {
	return &Executor{
		store:          store,
		circuitBreaker: NewCircuitBreaker(5, 30*time.Second),
	}
}

// ExecuteStep runs a single PlanStep, injecting variables from previous results.
func (e *Executor) ExecuteStep(ctx context.Context, session *ExecutionSession, stepIndex int) error {
	step := session.GetStep(stepIndex)
	if step == nil {
		return fmt.Errorf("step %d not found", stepIndex)
	}

	// Check circuit breaker
	if err := e.circuitBreaker.Allow(); err != nil {
		session.UpdateStep(stepIndex, func(s *PlanStep) {
			s.Status = StepStatusFailed
			s.Error = err.Error()
		})
		return err
	}

	session.UpdateStep(stepIndex, func(s *PlanStep) {
		s.Status = StepStatusRunning
		now := time.Now()
		s.StartTime = &now
	})

	// Inject variables from previous step results
	resolvedQuery := e.resolveVariables(step.Query, session, stepIndex)

	// Execute with timeout
	queryCtx, cancel := WithQueryTimeout(ctx)
	defer cancel()

	log.Printf("[Agent/Executor] Step %d: executing query: %s", stepIndex, resolvedQuery)

	results, err := e.store.Query(queryCtx, resolvedQuery)
	if err != nil {
		e.circuitBreaker.RecordFailure()
		session.UpdateStep(stepIndex, func(s *PlanStep) {
			s.Status = StepStatusFailed
			s.Error = err.Error()
			now := time.Now()
			s.EndTime = &now
		})
		return fmt.Errorf("step %d query failed: %w", stepIndex, err)
	}

	e.circuitBreaker.RecordSuccess()

	// Hydrate results with source code
	hydrated := e.hydrateResults(ctx, results, 10)

	now := time.Now()
	session.UpdateStep(stepIndex, func(s *PlanStep) {
		s.Status = StepStatusSuccess
		s.Result = results
		s.Hydrated = hydrated
		s.EndTime = &now
	})

	log.Printf("[Agent/Executor] Step %d: returned %d results, hydrated %d nodes", stepIndex, len(results), len(hydrated))
	return nil
}

// resolveVariables replaces {{step_N_result}} placeholders with actual values.
func (e *Executor) resolveVariables(query string, session *ExecutionSession, currentIndex int) string {
	return stepRefRegex.ReplaceAllStringFunc(query, func(match string) string {
		m := stepRefRegex.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		// m[1] is the step index string
		refIndex := 0
		fmt.Sscanf(m[1], "%d", &refIndex)

		prevStep := session.GetStep(refIndex)
		if prevStep == nil || len(prevStep.Result) == 0 {
			return match // leave unresolved
		}

		// Take the first binding of the first result row
		// Convention: first variable in the previous query result
		for _, val := range prevStep.Result[0] {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
		return match
	})
}

// hydrateResults fetches source code for result IDs, limited to maxCount.
func (e *Executor) hydrateResults(ctx context.Context, results []map[string]any, maxCount int) []HydratedNode {
	seen := make(map[string]bool)
	var hydrated []HydratedNode

	for i, row := range results {
		if len(hydrated) >= maxCount {
			break
		}

		// Extract the primary ID from the result row
		id := e.extractID(row)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		node := HydratedNode{ID: id, Metadata: make(map[string]string)}

		// Try to get name/kind from result row
		if name, ok := row["?s"].(string); ok {
			node.Name = name
		} else if name, ok := row["?o"].(string); ok {
			node.Name = name
		} else {
			parts := strings.Split(id, ":")
			node.Name = parts[len(parts)-1]
		}

		// Fetch source code
		content, err := e.store.GetContentByKey(id)

		if err == nil && len(content) > 0 {
			code := string(content)
			if len(code) > 2000 {
				code = code[:2000] + "\n... (truncated)"
			}
			node.Code = code
		}

		// Fetch kind from triples
		kindCtx, cancel2 := context.WithTimeout(ctx, 500*time.Millisecond)
		kindResults, _ := e.store.Query(kindCtx, fmt.Sprintf(`triples("%s", "kind", ?o)`, id))
		cancel2()
		if len(kindResults) > 0 {
			if kind, ok := kindResults[0]["?o"].(string); ok {
				node.Kind = kind
			}
		}

		hydrated = append(hydrated, node)
		_ = i // suppress unused
	}

	return hydrated
}

// extractID pulls the most meaningful ID from a result row.
func (e *Executor) extractID(row map[string]any) string {
	// Prefer ?s (subject) as the primary ID
	if s, ok := row["?s"].(string); ok && s != "" {
		return s
	}
	// Fall back to ?o (object)
	if o, ok := row["?o"].(string); ok && o != "" {
		return o
	}
	// Try any value
	for _, v := range row {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// ExecuteAllSteps runs every step in sequence. Stops on first failure.
func (e *Executor) ExecuteAllSteps(ctx context.Context, session *ExecutionSession) error {
	for i := 0; i < len(session.Steps); i++ {
		if err := e.ExecuteStep(ctx, session, i); err != nil {
			log.Printf("[Agent/Executor] Step %d failed: %v", i, err)
			// Attempt self-correction
			if corrected := e.attemptCorrection(ctx, session, i); corrected {
				continue
			}
			return fmt.Errorf("step %d failed and could not be corrected: %w", i, err)
		}
	}
	return nil
}

// attemptCorrection tries to fix a failed step by broadening the query.
func (e *Executor) attemptCorrection(ctx context.Context, session *ExecutionSession, stepIndex int) bool {
	step := session.GetStep(stepIndex)
	if step == nil {
		return false
	}

	log.Printf("[Agent/Executor] Attempting correction for step %d: %s", stepIndex, step.Query)

	// Strategy: if the query has a specific literal, try replacing it with a variable
	originalQuery := step.Query

	// Try: replace quoted literals with ?o variables
	corrected := regexp.MustCompile(`"([^"]+)"`).ReplaceAllString(originalQuery, `?o`)

	if corrected == originalQuery {
		// No change possible, try a broader query
		corrected = `triples(?s, ?p, ?o) LIMIT 5`
	}

	log.Printf("[Agent/Executor] Corrected query: %s", corrected)

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

	// Restore original query on failure
	session.UpdateStep(stepIndex, func(s *PlanStep) {
		s.Query = originalQuery
		s.Status = StepStatusFailed
	})
	return false
}
