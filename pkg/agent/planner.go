package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/logger"
)

// ModelAdapter abstracts the LLM call so the planner is testable.
type ModelAdapter interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

// PlanResult is the JSON structure we expect the LLM to return.
type PlanResult struct {
	Steps []PlanStepSpec `json:"steps"`
}

type PlanStepSpec struct {
	Task  string `json:"task"`
	Query string `json:"query"`
}

// Planner decomposes a natural-language query into a sequence of Datalog PlanSteps.
type Planner struct {
	model ModelAdapter
}

// NewPlanner creates a planner backed by the given model adapter.
func NewPlanner(model ModelAdapter) *Planner {
	return &Planner{model: model}
}

// Plan asks the LLM to decompose the query and returns the PlanSteps.
// The context should already carry a timeout (e.g. 30s).
func (p *Planner) Plan(ctx context.Context, query string, predicates []string) ([]PlanStep, error) {
	prompt := buildPlannerPrompt(query, predicates)

	logger.Debug("Agent/Planner Sending plan request", "query", query, "predicates", len(predicates))

	response, err := p.model.GenerateContent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("planner model call failed: %w", err)
	}

	steps, err := parsePlanResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan response: %w", err)
	}

	logger.Debug("Agent/Planner Generated steps", "steps", len(steps))
	return steps, nil
}

func buildPlannerPrompt(query string, predicates []string) string {
	var predList strings.Builder
	for _, p := range predicates {
		predList.WriteString(fmt.Sprintf("- `%s`\n", p))
	}

	return fmt.Sprintf(`You are a code analysis planner. Decompose the user's question into a sequence of Datalog queries.

Available predicates:
%s

Rules:
1. Each step MUST be a valid Datalog triple query like: triples(?s, "predicate", ?o)
2. Steps are executed sequentially. Step N+1 can reference results from step N using {{step_N_result}}.
3. Limit to 3-5 steps maximum.
4. The final step should produce the answer the user is looking for.
5. If the question can be answered in one query, return exactly one step.

User Question: %s

Respond with ONLY a JSON object:
{
  "steps": [
    {"task": "Find entry points", "query": "triples(?s, \"defines\", \"main\")"},
    {"task": "Trace calls", "query": "triples(\"{{step_0_result}}\", \"calls\", ?o)"}
  ]
}`, predList.String(), query)
}

// parsePlanResponse extracts PlanSteps from the LLM JSON response.
func parsePlanResponse(response string) ([]PlanStep, error) {
	// Try to find JSON in the response (handle markdown code fences)
	cleaned := response
	if idx := strings.Index(response, "{"); idx >= 0 {
		if endIdx := strings.LastIndex(response, "}"); endIdx > idx {
			cleaned = response[idx : endIdx+1]
		}
	}

	var result PlanResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON in plan response: %w (raw: %s)", err, cleaned[:min(200, len(cleaned))])
	}

	steps := make([]PlanStep, len(result.Steps))
	now := time.Now()
	for i, spec := range result.Steps {
		steps[i] = PlanStep{
			Index:     i,
			Task:      spec.Task,
			Query:     spec.Query,
			Status:    StepStatusPending,
			StartTime: &now,
		}
	}

	return steps, nil
}
