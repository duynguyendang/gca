package repl

import (
	"log"
	"testing"
)

func TestExecutionSession(t *testing.T) {
	steps := []PlanStep{
		{ID: 1, Task: "Step 1", Query: "query1", Explanation: "exp1"},
		{ID: 2, Task: "Step 2", Query: "query2", Explanation: "exp2"},
	}

	session := NewExecutionSession("Test Goal", steps)

	if session.IsComplete() {
		t.Error("New session should not be complete")
	}

	if session.CurrentStep != 0 {
		t.Errorf("Expected CurrentStep 0, got %d", session.CurrentStep)
	}

	// Calculate and consume step 1
	step1 := session.NextStep()
	if step1.ID != 1 {
		t.Errorf("Expected Step ID 1, got %d", step1.ID)
	}

	if session.IsComplete() {
		t.Error("Session should not be complete after 1 step")
	}

	// Calculate and consume step 2
	step2 := session.NextStep()
	if step2.ID != 2 {
		t.Errorf("Expected Step ID 2, got %d", step2.ID)
	}

	if !session.IsComplete() {
		t.Error("Session should be complete after 2 steps")
	}

	// Try consuming past end
	step3 := session.NextStep()
	if step3 != nil {
		t.Error("NextStep should return nil when complete")
	}
}

func TestVariableBindings(t *testing.T) {
	steps := []PlanStep{{ID: 1}}
	session := NewExecutionSession("Goal", steps)

	// Simulate results for Step 1
	results1 := []map[string]string{
		{"A": "foo", "B": "1"},
		{"A": "bar", "B": "2"},
		{"A": "foo", "B": "3"}, // Duplicate A value
	}
	session.StoreResults(1, results1)

	// Test extracting bindings for A
	bindingsA := session.GetAllVariableBindings(1, "A")
	if len(bindingsA) != 2 {
		t.Errorf("Expected 2 unique bindings for A, got %d: %v", len(bindingsA), bindingsA)
	}

	// Check contents (order not guaranteed due to map iteration if implemented that way,
	// but implementation uses slice iteration + map check, so order should be preserved as encountered)
	// "foo" first, then "bar". "foo" again ignored.
	if bindingsA[0] != "foo" || bindingsA[1] != "bar" {
		t.Errorf("Unexpected bindings: %v", bindingsA)
	}

	// Test nonexistent variable
	bindingsC := session.GetAllVariableBindings(1, "C")
	if len(bindingsC) != 0 {
		t.Errorf("Expected 0 bindings for C, got %d", len(bindingsC))
	}
}

func TestJSONParsing(t *testing.T) {
	validJSON := `
	[
		{
			"id": 1,
			"task": "Test Task",
			"query": "triples(A, \"calls\", \"panic\")",
			"explanation": "Test Explanation"
		}
	]`

	steps, err := parseJSONPlan(validJSON)
	if err != nil {
		t.Fatalf("Failed to parse valid JSON: %v", err)
	}

	if len(steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(steps))
	}

	if steps[0].Query != "triples(A, \"calls\", \"panic\")" {
		t.Errorf("Unexpected query: %s", steps[0].Query)
	}

	// Test with markdown backticks
	markdownJSON := "```json\n" + validJSON + "\n```"
	stepsMD, err := parseJSONPlan(markdownJSON)
	if err != nil {
		t.Fatalf("Failed to parse markdown JSON: %v", err)
	}
	if len(stepsMD) != 1 {
		t.Errorf("Expected 1 step from markdown, got %d", len(stepsMD))
	}

	// Test invalid JSON
	_, err = parseJSONPlan("invalid json")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestVariableInjectionLogic(t *testing.T) {
	steps := []PlanStep{{ID: 1}}
	session := NewExecutionSession("Goal", steps)

	// Simulate results for Step 1
	results1 := []map[string]string{
		{"A": "main.go"},
	}
	session.StoreResults(1, results1)

	// Test Injection
	query := "triples($1.A, \"calls\", B)"
	expanded := processVariableInjection(query, session)
	expected := "triples(\"main.go\", \"calls\", B)"

	if expanded != expected {
		t.Errorf("Expected '%s', got '%s'", expected, expanded)
	}

	// Test no injection if variable not found (should keep placeholder or handle gracefully)
	// Current implementation keeps placeholder if no bindings
	query2 := "triples($99.A, \"calls\", B)"
	expanded2 := processVariableInjection(query2, session)
	if expanded2 != query2 {
		t.Errorf("Expected no change '%s', got '%s'", query2, expanded2)
	}
}

// Silence logger API for clean test output
func init() {
	log.SetOutput(log.Writer())
}
