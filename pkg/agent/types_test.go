package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"text/template"

	"github.com/duynguyendang/gca/pkg/prompts"
)

func TestPlanStep_StatusConstants(t *testing.T) {
	statuses := []StepStatus{StepStatusPending, StepStatusRunning, StepStatusSuccess, StepStatusFailed, StepStatusCorrected}
	expected := []string{"Pending", "Running", "Success", "Failed", "Corrected"}

	if len(statuses) != len(expected) {
		t.Fatalf("Status constants mismatch: got %d, want %d", len(statuses), len(expected))
	}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("Status[%d] = %q, want %q", i, status, expected[i])
		}
	}
}

func TestHydratedNode_Structure(t *testing.T) {
	node := HydratedNode{
		ID:   "test-id",
		Name: "TestFunc",
		Kind: "function",
		Code: "func TestFunc() {}",
		Metadata: map[string]string{
			"file": "test.go",
		},
	}

	if node.ID != "test-id" {
		t.Errorf("HydratedNode.ID = %q, want %q", node.ID, "test-id")
	}
	if node.Name != "TestFunc" {
		t.Errorf("HydratedNode.Name = %q, want %q", node.Name, "TestFunc")
	}
	if node.Kind != "function" {
		t.Errorf("HydratedNode.Kind = %q, want %q", node.Kind, "function")
	}
	if node.Metadata["file"] != "test.go" {
		t.Errorf("HydratedNode.Metadata[file] = %q, want %q", node.Metadata["file"], "test.go")
	}
}

func TestPlanResult_JSON(t *testing.T) {
	jsonData := `{
		"steps": [
			{"task": "Find entry points", "query": "triples(?s, \"defines\", \"main\")"},
			{"task": "Trace calls", "query": "triples(?s, \"calls\", ?o)"}
		]
	}`

	var result PlanResult
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		t.Fatalf("Failed to unmarshal PlanResult: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Errorf("PlanResult.Steps len = %d, want 2", len(result.Steps))
	}
	if result.Steps[0].Task != "Find entry points" {
		t.Errorf("Steps[0].Task = %q, want %q", result.Steps[0].Task, "Find entry points")
	}
	if result.Steps[1].Query != "triples(?s, \"calls\", ?o)" {
		t.Errorf("Steps[1].Query = %q, want %q", result.Steps[1].Query, "triples(?s, \"calls\", ?o)")
	}
}

// MockModelAdapter is a test double for ModelAdapter.
type MockModelAdapter struct {
	Response string
	Err      error
}

func (m *MockModelAdapter) GenerateContent(ctx context.Context, prompt string) (string, error) {
	return m.Response, m.Err
}

// mockPlannerPromptLoader returns a minimal prompt for testing.
func mockPlannerPromptLoader(name string) (*prompts.Prompt, error) {
	// Use the actual prompt file for more realistic testing
	p, err := prompts.LoadPrompt(name)
	if err != nil {
		// Return a minimal inline prompt if file not found (for CI environments)
		if name == "prompts/agent_planner.prompt" {
			tmpl, _ := template.New("prompt").Parse("Available predicates:\n{{.Predicates}}\n\nUser Question: {{.Query}}\n\nRespond with ONLY a JSON array.")
			return &prompts.Prompt{
				Template: tmpl,
			}, nil
		}
		return nil, err
	}
	return p, nil
}

func TestNewPlanner(t *testing.T) {
	mock := &MockModelAdapter{Response: "{}"}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)
	if p == nil {
		t.Fatal("NewPlanner returned nil")
	}
	if p.model != mock {
		t.Errorf("Planner.model = %v, want %v", p.model, mock)
	}
}

func TestPlanner_Plan_ModelError(t *testing.T) {
	mock := &MockModelAdapter{
		Response: "{}",
		Err:      context.DeadlineExceeded,
	}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)

	_, err := p.Plan(context.Background(), "test query", []string{"calls"})
	if err == nil {
		t.Error("Plan() should return error when model fails")
	}
	if !strings.Contains(err.Error(), "planner model call failed") {
		t.Errorf("Plan() error = %v, want error containing 'planner model call failed'", err)
	}
}

func TestPlanner_Plan_InvalidJSON(t *testing.T) {
	mock := &MockModelAdapter{Response: "not valid json"}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)

	_, err := p.Plan(context.Background(), "test query", []string{"calls"})
	if err == nil {
		t.Error("Plan() should return error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse plan response") {
		t.Errorf("Plan() error = %v, want error containing 'failed to parse plan response'", err)
	}
}

func TestPlanner_Plan_Success(t *testing.T) {
	jsonResponse := `{"steps": [{"task": "Find main", "query": "triples(?s, 'defines', 'main')"}]}`
	mock := &MockModelAdapter{Response: jsonResponse}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)

	steps, err := p.Plan(context.Background(), "find the main function", []string{"defines", "calls"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(steps))
	}
	if steps[0].Task != "Find main" {
		t.Errorf("steps[0].Task = %q, want %q", steps[0].Task, "Find main")
	}
	if steps[0].Status != StepStatusPending {
		t.Errorf("steps[0].Status = %v, want %v", steps[0].Status, StepStatusPending)
	}
	if steps[0].Index != 0 {
		t.Errorf("steps[0].Index = %d, want 0", steps[0].Index)
	}
}

func TestPlanner_Plan_WithMarkdownFence(t *testing.T) {
	jsonResponse := "Here's the plan:\n```json\n{\"steps\": [{\"task\": \"Find main\", \"query\": \"triples(?s, 'defines', 'main')\"}]}\n```\nJust do it."
	mock := &MockModelAdapter{Response: jsonResponse}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)

	steps, err := p.Plan(context.Background(), "find main", []string{"defines"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(steps))
	}
}

func TestPlanner_Plan_MultipleSteps(t *testing.T) {
	jsonResponse := `{"steps": [
		{"task": "Find entry points", "query": "triples(?s, 'defines', 'main')"},
		{"task": "Find callers", "query": "triples('main.go:main', 'calls', ?o)"},
		{"task": "Analyze impact", "query": "triples(?s, 'imports', 'main.go:main')"}
	]}`
	mock := &MockModelAdapter{Response: jsonResponse}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)

	steps, err := p.Plan(context.Background(), "analyze the auth flow", []string{"defines", "calls", "imports"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(steps))
	}
	for i, step := range steps {
		if step.Index != i {
			t.Errorf("steps[%d].Index = %d, want %d", i, step.Index, i)
		}
		if step.Status != StepStatusPending {
			t.Errorf("steps[%d].Status = %v, want %v", i, step.Status, StepStatusPending)
		}
	}
}

func TestBuildPlannerPrompt(t *testing.T) {
	mock := &MockModelAdapter{Response: "{}"}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)

	prompt, err := p.buildPlannerPrompt("find main", []string{"defines", "calls", "imports"})
	if err != nil {
		t.Fatalf("buildPlannerPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Available predicates:") {
		t.Error("Prompt should contain 'Available predicates:'")
	}
	if !strings.Contains(prompt, "- `defines`") {
		t.Error("Prompt should list 'defines' predicate")
	}
	if !strings.Contains(prompt, "find main") {
		t.Error("Prompt should contain the user query")
	}
}

func TestBuildPlannerPrompt_EmptyPredicates(t *testing.T) {
	mock := &MockModelAdapter{Response: "{}"}
	p := NewPlannerWithLoader(mock, mockPlannerPromptLoader)

	prompt, err := p.buildPlannerPrompt("test query", []string{})
	if err != nil {
		t.Fatalf("buildPlannerPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "test query") {
		t.Error("Prompt should contain user query even with empty predicates")
	}
}

func TestParsePlanResponse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSteps int
		wantErr   bool
	}{
		{
			name:      "valid JSON with steps",
			input:     `{"steps":[{"task":"Test","query":"query"}]}`,
			wantSteps: 1,
			wantErr:   false,
		},
		{
			name:      "JSON wrapped in markdown",
			input:     "```json\n{\"steps\":[{\"task\":\"Test\",\"query\":\"q\"}]}\n```",
			wantSteps: 1,
			wantErr:   false,
		},
		{
			name:      "JSON with text before and after",
			input:     "Before text {\"steps\":[{\"task\":\"T\",\"query\":\"Q\"}]} After text",
			wantSteps: 1,
			wantErr:   false,
		},
		{
			name:      "empty steps array",
			input:     `{"steps":[]}`,
			wantSteps: 0,
			wantErr:   false,
		},
		{
			name:    "invalid JSON",
			input:   `not json at all`,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := parsePlanResponse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePlanResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(steps) != tt.wantSteps {
				t.Errorf("parsePlanResponse() returned %d steps, want %d", len(steps), tt.wantSteps)
			}
		})
	}
}
