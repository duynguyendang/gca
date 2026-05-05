package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/gca/pkg/prompts"
)

// ModelAdapter is an alias for the shared LLMClient interface.
type ModelAdapter = common.LLMClient

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
	model        ModelAdapter
	promptLoader func(name string) (*prompts.Prompt, error)
}

// NewPlanner creates a planner backed by the given model adapter.
func NewPlanner(model ModelAdapter) *Planner {
	return &Planner{
		model:        model,
		promptLoader: prompts.LoadPrompt,
	}
}

// NewPlannerWithLoader creates a planner with a custom prompt loader (for testing).
func NewPlannerWithLoader(model ModelAdapter, loader func(name string) (*prompts.Prompt, error)) *Planner {
	return &Planner{
		model:        model,
		promptLoader: loader,
	}
}

// Plan asks the LLM to decompose the query and returns the PlanSteps.
// The context should already carry a timeout (e.g. 30s).
func (p *Planner) Plan(ctx context.Context, query string, predicates []string) ([]PlanStep, error) {
	prompt, err := p.buildPlannerPrompt(query, predicates)
	if err != nil {
		return nil, fmt.Errorf("failed to build planner prompt: %w", err)
	}

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

func (p *Planner) buildPlannerPrompt(query string, predicates []string) (string, error) {
	var predList strings.Builder
	for _, pred := range predicates {
		predList.WriteString(fmt.Sprintf("- `%s`\n", pred))
	}

	prompt, err := p.promptLoader("prompts/agent_planner.prompt")
	if err != nil {
		return "", err
	}
	if prompt == nil {
		return "", fmt.Errorf("agent_planner.prompt not loaded")
	}

	return prompt.Execute(map[string]interface{}{
		"Query":      query,
		"Predicates": predList.String(),
	})
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
