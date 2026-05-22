package ooda

import (
	"context"
	"strings"
	"testing"

	"github.com/duynguyendang/gca/pkg/ingest"
)

func TestGCATask_Constants(t *testing.T) {
	tasks := []GCATask{
		TaskInsight, TaskChat, TaskPrune, TaskSummary, TaskNarrative,
		TaskResolveSymbol, TaskPathEndpoints, TaskDatalog, TaskPathNarrative,
		TaskSmartSearchAnalysis, TaskMultiFileSummary, TaskRefactor,
		TaskTestGeneration, TaskSecurityAudit, TaskPerformance,
	}
	expectedStrings := []string{
		"insight", "chat", "prune", "summary", "narrative",
		"resolve_symbol", "path_endpoints", "datalog", "path_narrative",
		"smart_search_analysis", "multi_file_summary", "refactor",
		"test_generation", "security_audit", "performance",
	}

	if len(tasks) != len(expectedStrings) {
		t.Fatalf("GCATask constants count mismatch: got %d, want %d", len(tasks), len(expectedStrings))
	}

	for i, task := range tasks {
		if string(task) != expectedStrings[i] {
			t.Errorf("GCATask[%d] = %q, want %q", i, task, expectedStrings[i])
		}
	}
}

func TestNewGCAFrame(t *testing.T) {
	frame := NewGCAFrame("proj-1", "find main", TaskInsight)

	if frame.ProjectID != "proj-1" {
		t.Errorf("frame.ProjectID = %q, want %q", frame.ProjectID, "proj-1")
	}
	if frame.Task != TaskInsight {
		t.Errorf("frame.Task = %v, want %v", frame.Task, TaskInsight)
	}
	if frame.Input != "find main" {
		t.Errorf("frame.Input = %q, want %q", frame.Input, "find main")
	}
	if frame.CognitiveFrame == nil {
		t.Error("frame.CognitiveFrame should not be nil")
	}
}

func TestNewGCAFrame_AllTaskTypes(t *testing.T) {
	taskTypes := []GCATask{
		TaskInsight, TaskChat, TaskPrune, TaskSummary, TaskNarrative,
		TaskResolveSymbol, TaskPathEndpoints, TaskDatalog, TaskPathNarrative,
		TaskSmartSearchAnalysis, TaskMultiFileSummary, TaskRefactor,
		TaskTestGeneration, TaskSecurityAudit, TaskPerformance,
	}

	for _, task := range taskTypes {
		frame := NewGCAFrame("proj", "query", task)
		if frame.Task != task {
			t.Errorf("NewGCAFrame with task %v failed: got %v", task, frame.Task)
		}
	}
}

func TestGCAFrame_DataField(t *testing.T) {
	frame := NewGCAFrame("proj", "query", TaskChat)

	// Data is interface{}, can store anything
	testData := map[string]string{"key": "value"}
	frame.Data = testData

	if frame.Data == nil {
		t.Error("frame.Data should be settable")
	}
}

func TestNewGCALoop(t *testing.T) {
	loop := NewGCALoop(nil, nil, nil, nil, nil)
	if loop == nil {
		t.Fatal("NewGCALoop returned nil")
	}
	if loop.observer != nil || loop.orienter != nil || loop.decider != nil ||
		loop.verifier != nil || loop.actor != nil {
		t.Error("NewGCALoop should initialize all fields to nil")
	}
}

// Mock implementations for testing GCALoop.Run
type mockObserver struct {
	err error
}

func (m *mockObserver) Observe(ctx context.Context, frame *GCAFrame) error {
	return m.err
}

type mockOrienter struct {
	err error
}

func (m *mockOrienter) Orient(ctx context.Context, frame *GCAFrame) error {
	return m.err
}

type mockDecider struct {
	err error
}

func (m *mockDecider) Decide(ctx context.Context, frame *GCAFrame) error {
	return m.err
}

type mockVerifier struct {
	err error
}

func (m *mockVerifier) Verify(ctx context.Context, frame *GCAFrame) error {
	return m.err
}

type mockActor struct {
	err           error
	capturedPhase Phase
	capturedInput string
}

func (m *mockActor) Act(ctx context.Context, frame *GCAFrame) error {
	m.capturedPhase = frame.Phase
	m.capturedInput = frame.Input
	return m.err
}

func TestGCALoop_Run_Success(t *testing.T) {
	loop := NewGCALoop(
		&mockObserver{},
		&mockOrienter{},
		&mockDecider{},
		&mockVerifier{},
		&mockActor{},
	)

	frame := NewGCAFrame("proj", "query", TaskChat)
	result, err := loop.Run(context.Background(), frame)

	if err != nil {
		t.Fatalf("loop.Run() error = %v", err)
	}
	if result == nil {
		t.Fatal("loop.Run() returned nil frame")
	}
}

func TestGCALoop_Run_ObserverError(t *testing.T) {
	errObserve := context.DeadlineExceeded
	loop := NewGCALoop(
		&mockObserver{err: errObserve},
		&mockOrienter{},
		&mockDecider{},
		&mockVerifier{},
		&mockActor{},
	)

	frame := NewGCAFrame("proj", "query", TaskChat)
	_, err := loop.Run(context.Background(), frame)

	if err == nil {
		t.Fatal("loop.Run() should return error when observer fails")
	}
	if !strings.Contains(err.Error(), "observe") {
		t.Errorf("error should mention 'observe' phase, got: %v", err)
	}
}

func TestGCALoop_Run_OrienterError(t *testing.T) {
	errOrient := context.DeadlineExceeded
	loop := NewGCALoop(
		&mockObserver{},
		&mockOrienter{err: errOrient},
		&mockDecider{},
		&mockVerifier{},
		&mockActor{},
	)

	frame := NewGCAFrame("proj", "query", TaskChat)
	_, err := loop.Run(context.Background(), frame)

	if err == nil {
		t.Fatal("loop.Run() should return error when orienter fails")
	}
	if !strings.Contains(err.Error(), "orient") {
		t.Errorf("error should mention 'orient' phase, got: %v", err)
	}
}

func TestGCALoop_Run_DeciderError(t *testing.T) {
	errDecide := context.DeadlineExceeded
	loop := NewGCALoop(
		&mockObserver{},
		&mockOrienter{},
		&mockDecider{err: errDecide},
		&mockVerifier{},
		&mockActor{},
	)

	frame := NewGCAFrame("proj", "query", TaskChat)
	_, err := loop.Run(context.Background(), frame)

	if err == nil {
		t.Fatal("loop.Run() should return error when decider fails")
	}
	if !strings.Contains(err.Error(), "decide") {
		t.Errorf("error should mention 'decide' phase, got: %v", err)
	}
}

func TestGCALoop_Run_VerifierError(t *testing.T) {
	errVerify := context.DeadlineExceeded
	loop := NewGCALoop(
		&mockObserver{},
		&mockOrienter{},
		&mockDecider{},
		&mockVerifier{err: errVerify},
		&mockActor{},
	)

	frame := NewGCAFrame("proj", "query", TaskChat)
	_, err := loop.Run(context.Background(), frame)

	if err == nil {
		t.Fatal("loop.Run() should return error when verifier fails")
	}
	if !strings.Contains(err.Error(), "verify") {
		t.Errorf("error should mention 'verify' phase, got: %v", err)
	}
}

func TestGCALoop_Run_ActorError(t *testing.T) {
	errAct := context.DeadlineExceeded
	actor := &mockActor{err: errAct}
	loop := NewGCALoop(
		&mockObserver{},
		&mockOrienter{},
		&mockDecider{},
		&mockVerifier{},
		actor,
	)

	frame := NewGCAFrame("proj", "query", TaskChat)
	_, err := loop.Run(context.Background(), frame)

	if err == nil {
		t.Fatal("loop.Run() should return error when actor fails")
	}
	if !strings.Contains(err.Error(), "act") {
		t.Errorf("error should mention 'act' phase, got: %v", err)
	}
}

func TestGCALoop_Run_NilPhases(t *testing.T) {
	// With nil observers, loop should still complete without error
	loop := NewGCALoop(nil, nil, nil, nil, nil)

	frame := NewGCAFrame("proj", "query", TaskChat)
	result, err := loop.Run(context.Background(), frame)

	if err != nil {
		t.Fatalf("loop.Run() with nil phases should succeed, got error: %v", err)
	}
	if result == nil {
		t.Fatal("loop.Run() returned nil frame")
	}
}

func TestGCALoop_Run_PhaseOrder(t *testing.T) {
	observedPhases := make([]Phase, 0)
	phaseTracker := &phaseCollecter{phases: &observedPhases}

	loop := NewGCALoop(phaseTracker, phaseTracker, phaseTracker, phaseTracker, phaseTracker)
	frame := NewGCAFrame("proj", "query", TaskChat)

	_, err := loop.Run(context.Background(), frame)
	if err != nil {
		t.Fatalf("loop.Run() error = %v", err)
	}

	expected := []Phase{PhaseObserve, PhaseOrient, PhaseDecide, PhaseVerify, PhaseAct}
	if len(observedPhases) != len(expected) {
		t.Errorf("observed phases count = %d, want %d", len(observedPhases), len(expected))
	}
	for i, phase := range observedPhases {
		if phase != expected[i] {
			t.Errorf("phases[%d] = %v, want %v", i, phase, expected[i])
		}
	}
}

type phaseCollecter struct {
	phases *[]Phase
}

func (p *phaseCollecter) Observe(ctx context.Context, frame *GCAFrame) error {
	*p.phases = append(*p.phases, frame.Phase)
	return nil
}

func (p *phaseCollecter) Orient(ctx context.Context, frame *GCAFrame) error {
	*p.phases = append(*p.phases, frame.Phase)
	return nil
}

func (p *phaseCollecter) Decide(ctx context.Context, frame *GCAFrame) error {
	*p.phases = append(*p.phases, frame.Phase)
	return nil
}

func (p *phaseCollecter) Verify(ctx context.Context, frame *GCAFrame) error {
	*p.phases = append(*p.phases, frame.Phase)
	return nil
}

func (p *phaseCollecter) Act(ctx context.Context, frame *GCAFrame) error {
	*p.phases = append(*p.phases, frame.Phase)
	return nil
}

func TestIntentClassifier_Classify(t *testing.T) {
	classifier := &IntentClassifier{}

	tests := []struct {
		input string
		want  GCATask
	}{
		{"analyze the code", TaskInsight},
		{"show me an insight", TaskInsight},
		{"what is the role of X", TaskInsight},
		{"explain this function", TaskChat},
		{"how does the auth work", TaskChat},
		{"what is main.go:main", TaskChat},
		{"trace the path", TaskNarrative},
		{"show me the flow", TaskNarrative},
		{"call chain for login", TaskNarrative},
		{"summarize this module", TaskSummary},
		{"give me a summary", TaskSummary},
		{"resolve which file has the handler", TaskResolveSymbol},
		{"which function handles auth", TaskResolveSymbol},
		{"random query without keywords", TaskChat},
		{"just some text", TaskChat},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := classifier.Classify(tt.input)
			if got != tt.want {
				t.Errorf("Classify(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractPotentialSymbols(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantLen int
	}{
		{
			name:    "empty query",
			query:   "",
			wantLen: 0,
		},
		{
			name:    "only short words",
			query:   "the and but or",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPotentialSymbols(tt.query)
			if len(got) != tt.wantLen {
				t.Errorf("ExtractPotentialSymbols(%q) returned %d symbols, want %d", tt.query, len(got), tt.wantLen)
			}
		})
	}
}

func TestExtractPotentialSymbols_ReturnsSymbols(t *testing.T) {
	result := ExtractPotentialSymbols("Find main.go:main")
	if len(result) == 0 {
		t.Error("Should extract symbol from query with path-like content")
	}
	// Verify result contains something reasonable
	for _, sym := range result {
		if len(sym) < 4 {
			t.Errorf("Symbol %q too short (min 4 chars)", sym)
		}
	}
}

// --- Neuro-Symbolic Executor tests ---

func TestParseTemplateResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantID   string
		wantSkip bool
		wantKey  string
		wantVal  string
	}{
		{
			name:     "template with single param",
			response: "TEMPLATE: smell_circular_direct\nPARAMS: File=handlers/auth.go",
			wantID:   "smell_circular_direct",
			wantKey:  "File",
			wantVal:  "handlers/auth.go",
		},
		{
			name:     "template with multiple params",
			response: "TEMPLATE: query_who_calls\nPARAMS: Symbol=authService, depth=3",
			wantID:   "query_who_calls",
			wantKey:  "Symbol",
			wantVal:  "authService",
		},
		{
			name:     "no template matched",
			response: "TEMPLATE: none\nREASON: Conversational query",
			wantSkip: true,
		},
		{
			name:     "template without params",
			response: "TEMPLATE: smell_hub_anomaly\nPARAMS:",
			wantID:   "smell_hub_anomaly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTemplateResponse(tt.response)
			if result.Skip != tt.wantSkip {
				t.Errorf("Skip = %v, want %v", result.Skip, tt.wantSkip)
			}
			if result.TemplateID != tt.wantID {
				t.Errorf("TemplateID = %q, want %q", result.TemplateID, tt.wantID)
			}
			if tt.wantKey != "" {
				val, ok := result.Params[tt.wantKey]
				if !ok {
					t.Errorf("Missing param key %q", tt.wantKey)
				} else if val != tt.wantVal {
					t.Errorf("Params[%q] = %q, want %q", tt.wantKey, val, tt.wantVal)
				}
			}
		})
	}
}

func TestParameterizeTemplate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		params   map[string]string
		want     string
		noChange bool // expect body unchanged when params don't match
	}{
		{
			name:   "single param",
			body:   `triples(File, "calls", Callee) :- File = "{{File}}"`,
			params: map[string]string{"File": "handlers/auth.go"},
			want:   `triples(File, "calls", Callee) :- File = "handlers/auth.go"`,
		},
		{
			name:   "multiple params",
			body:   `triples({{Symbol}}, "calls", {{Target}})`,
			params: map[string]string{"Symbol": "main", "Target": "helper"},
			want:   `triples(main, "calls", helper)`,
		},
		{
			name:     "no params",
			body:     `triples(File, "calls", _)`,
			params:   map[string]string{},
			want:     `triples(File, "calls", _)`,
			noChange: true,
		},
		{
			name:   "unknown placeholders removed",
			body:   `triples({{File}}, "calls", {{Symbol}})`,
			params: map[string]string{"File": "main.go"},
			want:   `triples(main.go, "calls", )`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parameterizeTemplate(tt.body, tt.params)
			if got != tt.want {
				t.Errorf("parameterizeTemplate() = %q, want %q", got, tt.want)
			}
			if tt.noChange && got != tt.body {
				t.Errorf("expected no change, got %q", got)
			}
		})
	}
}

func TestParameterizeWithFallback(t *testing.T) {
	// parameterizeWithFallback is best-effort — it extracts the first
	// symbol-like token from the query. Verify it extracts something
	// reasonable without panicking (the regex was previously broken in Go).
	tests := []struct {
		name        string
		body        string
		query       string
		noRemaining bool // expect no remaining {{}} placeholders
	}{
		{
			name:        "normal query runs without panic",
			body:        `triples("{{target}}", "calls", _)`,
			query:       "what calls authService",
			noRemaining: true,
		},
		{
			name:        "empty query removes placeholders",
			body:        `triples("{{target}}", "calls", _)`,
			query:       "",
			noRemaining: true,
		},
		{
			name:        "unused placeholders stripped",
			body:        `triples({{File}}, "calls", {{Symbol}})`,
			query:       "find LoginHandler",
			noRemaining: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parameterizeWithFallback(tt.body, tt.query)
			if tt.noRemaining && strings.Contains(got, "{{") {
				t.Errorf("parameterizeWithFallback(%q, %q) = %q, should have no remaining placeholders", tt.body, tt.query, got)
			}
		})
	}
}

func TestFormatTemplatesForAI(t *testing.T) {
	tests := []struct {
		name      string
		templates []*ingest.TemplateStoreQuery
		wantLines int
	}{
		{
			name:      "empty templates",
			templates: nil,
			wantLines: 1, // "No templates available."
		},
		{
			name: "single template",
			templates: []*ingest.TemplateStoreQuery{
				{ID: "smell_circular_direct", Description: "Detect circular deps", Parameters: []ingest.TemplateParam{
					{Name: "File", Type: "string"},
				}},
			},
			wantLines: 1,
		},
		{
			name: "multiple templates",
			templates: []*ingest.TemplateStoreQuery{
				{ID: "a", Description: "first", Parameters: nil},
				{ID: "b", Description: "second", Parameters: []ingest.TemplateParam{
					{Name: "x", Type: "int"},
				}},
			},
			wantLines: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTemplatesForAI(tt.templates)
			if tt.templates == nil && result != "No templates available." {
				t.Errorf("expected 'No templates available.', got %q", result)
			}
			if tt.templates != nil {
				lines := strings.Count(result, "\n") + 1
				if lines < tt.wantLines {
					t.Errorf("expected at least %d lines, got %d: %q", tt.wantLines, lines, result)
				}
			}
		})
	}
}

func TestFormatTemplatesForAI_ShowsParameters(t *testing.T) {
	templates := []*ingest.TemplateStoreQuery{
		{
			ID:          "smell_circular_direct",
			Description: "Detect circular deps",
			Parameters: []ingest.TemplateParam{
				{Name: "File", Type: "string"},
			},
		},
		{
			ID:          "smell_hub",
			Description: "Detect hub anomalies",
			Parameters: []ingest.TemplateParam{
				{Name: "File", Type: "string"},
				{Name: "Threshold", Type: "int"},
			},
		},
	}

	result := formatTemplatesForAI(templates)

	if !strings.Contains(result, "File (string)") {
		t.Error("result should contain 'File (string)' for smell_circular_direct")
	}
	if !strings.Contains(result, "File (string), Threshold (int)") {
		t.Error("result should contain 'File (string), Threshold (int)' for smell_hub")
	}
}

func TestBuildSynthesisContext(t *testing.T) {
	result := &NeuroSymbolicResult{
		TemplateID: "test_template",
		Query:      `triples(File, "calls", _)`,
		Results: []map[string]any{
			{"File": "main.go", "Target": "helper.go"},
			{"File": "app.go", "Target": "db.go"},
		},
	}

	ctx := BuildSynthesisContext("find callers", result, "=== DIAGNOSTIC ===\nmsg\n===")

	if !strings.Contains(ctx, "test_template") {
		t.Error("context should contain template ID")
	}
	if !strings.Contains(ctx, "main.go") {
		t.Error("context should contain result data")
	}
	if !strings.Contains(ctx, "=== DIAGNOSTIC ===") {
		t.Error("context should include attention sink")
	}
	if !strings.Contains(ctx, "Datalog Query:") {
		t.Error("context should contain query label")
	}
}

func TestBuildNeuroSymbolicContext(t *testing.T) {
	result := &NeuroSymbolicResult{
		TemplateID: "query_who_calls",
		Query:      `triples(Symbol, "calls", _)`,
		Entity:     "authHandler",
		Reasoning:  "Selected query_who_calls to trace callers",
		Results: []map[string]any{
			{"Symbol": "authHandler", "Callee": "dbService"},
		},
	}

	ctx := BuildNeuroSymbolicContext("who calls authHandler", result, "=== ATTENTION ===\nentry points: main\n===")

	checks := []string{"query_who_calls", "authHandler", "Selected query_who_calls", "authHandler", "dbService", "=== ATTENTION ==="}
	for _, check := range checks {
		if !strings.Contains(ctx, check) {
			t.Errorf("context should contain %q", check)
		}
	}
}
