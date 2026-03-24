package repl

import (
	"fmt"
	"strings"
)

// PlanStep represents a single step in a multi-step execution plan.
type PlanStep struct {
	ID          int    `json:"id"`
	Task        string `json:"task"`
	Query       string `json:"query"`
	Explanation string `json:"explanation"`
}

// ExecutionSession maintains state across plan execution.
type ExecutionSession struct {
	Goal        string
	Steps       []PlanStep
	CurrentStep int
	Results     map[int][]map[string]string // Maps StepID to its query results
}

// NewExecutionSession creates a new execution session for a plan.
func NewExecutionSession(goal string, steps []PlanStep) *ExecutionSession {
	return &ExecutionSession{
		Goal:        goal,
		Steps:       steps,
		CurrentStep: 0,
		Results:     make(map[int][]map[string]string),
	}
}

// StoreResults stores the results for a given step.
func (s *ExecutionSession) StoreResults(stepID int, results []map[string]string) {
	s.Results[stepID] = results
}

// GetResults retrieves results for a given step.
func (s *ExecutionSession) GetResults(stepID int) []map[string]string {
	return s.Results[stepID]
}

// NextStep returns the next step to execute and advances the counter.
func (s *ExecutionSession) NextStep() *PlanStep {
	if s.CurrentStep >= len(s.Steps) {
		return nil
	}
	step := &s.Steps[s.CurrentStep]
	s.CurrentStep++
	return step
}

// IsComplete returns true if all steps have been executed.
func (s *ExecutionSession) IsComplete() bool {
	return s.CurrentStep >= len(s.Steps)
}

// GetAllVariableBindings extracts all values for a variable from a step's results.
// For example, if Step 1 returned [{A: "foo"}, {A: "bar"}], GetAllVariableBindings(1, "A")
// returns ["foo", "bar"].
func (s *ExecutionSession) GetAllVariableBindings(stepID int, varName string) []string {
	results := s.GetResults(stepID)
	if results == nil {
		return nil
	}

	bindings := make([]string, 0)
	seen := make(map[string]bool)

	for _, result := range results {
		if value, ok := result[varName]; ok && !seen[value] {
			bindings = append(bindings, value)
			seen[value] = true
		}
	}

	return bindings
}

// DisplayPlan prints the execution plan to the console.
func DisplayPlan(session *ExecutionSession) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("📋 Execution Plan: %s\n", session.Goal)
	fmt.Println(strings.Repeat("=", 60) + "\n")

	for _, step := range session.Steps {
		fmt.Printf("Step %d: %s\n", step.ID, step.Task)
		fmt.Printf("  Query: %s\n", step.Query)
		fmt.Printf("  Why:   %s\n\n", step.Explanation)
	}

	fmt.Println(strings.Repeat("=", 60))
}
