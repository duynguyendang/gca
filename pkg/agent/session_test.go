package agent

import (
	"sync"
	"testing"
)

func TestNewExecutionSession(t *testing.T) {
	session := NewExecutionSession("sess-123", "proj-456", "find main")

	if session.ID != "sess-123" {
		t.Errorf("Session.ID = %q, want %q", session.ID, "sess-123")
	}
	if session.ProjectID != "proj-456" {
		t.Errorf("Session.ProjectID = %q, want %q", session.ProjectID, "proj-456")
	}
	if session.Query != "find main" {
		t.Errorf("Session.Query = %q, want %q", session.Query, "find main")
	}
	if len(session.Steps) != 0 {
		t.Errorf("Session.Steps len = %d, want 0", len(session.Steps))
	}
	if session.Narrative != "" {
		t.Errorf("Session.Narrative = %q, want empty", session.Narrative)
	}
}

func TestExecutionSession_AddStep(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Step 1", Query: "query1"})

	if len(session.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(session.Steps))
	}
	if session.Steps[0].Task != "Step 1" {
		t.Errorf("Steps[0].Task = %q, want %q", session.Steps[0].Task, "Step 1")
	}
	if session.Steps[0].Status != StepStatusPending {
		t.Errorf("Steps[0].Status = %v, want %v", session.Steps[0].Status, StepStatusPending)
	}
}

func TestExecutionSession_AddStep_AppendsNotReplaces(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Step 1"})
	session.AddStep(PlanStep{Index: 1, Task: "Step 2"})

	if len(session.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(session.Steps))
	}
}

func TestExecutionSession_UpdateStep(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Original"})

	session.UpdateStep(0, func(step *PlanStep) {
		step.Status = StepStatusSuccess
		step.Task = "Updated"
	})

	if session.Steps[0].Task != "Updated" {
		t.Errorf("Updated Task = %q, want %q", session.Steps[0].Task, "Updated")
	}
	if session.Steps[0].Status != StepStatusSuccess {
		t.Errorf("Updated Status = %v, want %v", session.Steps[0].Status, StepStatusSuccess)
	}
}

func TestExecutionSession_UpdateStep_OutOfBounds(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Step 1"})

	// Should not panic
	session.UpdateStep(99, func(step *PlanStep) {
		step.Task = "Should not happen"
	})

	if session.Steps[0].Task != "Step 1" {
		t.Errorf("Out of bounds update should not modify step")
	}
}

func TestExecutionSession_UpdateStep_NegativeIndex(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Step 1"})

	// Should not panic
	session.UpdateStep(-1, func(step *PlanStep) {
		step.Task = "Should not happen"
	})

	if session.Steps[0].Task != "Step 1" {
		t.Errorf("Negative index update should not modify step")
	}
}

func TestExecutionSession_GetStep(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Step 1"})

	step := session.GetStep(0)
	if step == nil {
		t.Fatal("GetStep returned nil")
	}
	if step.Task != "Step 1" {
		t.Errorf("GetStep(0).Task = %q, want %q", step.Task, "Step 1")
	}
}

func TestExecutionSession_GetStep_OutOfBounds(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Step 1"})

	step := session.GetStep(99)
	if step != nil {
		t.Errorf("GetStep(99) = %v, want nil", step)
	}
}

func TestExecutionSession_GetStep_NegativeIndex(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.AddStep(PlanStep{Index: 0, Task: "Step 1"})

	step := session.GetStep(-1)
	if step != nil {
		t.Errorf("GetStep(-1) = %v, want nil", step)
	}
}

func TestExecutionSession_SetNarrative(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.SetNarrative("This is the final answer.")

	if session.Narrative != "This is the final answer." {
		t.Errorf("Narrative = %q, want %q", session.Narrative, "This is the final answer.")
	}
}

func TestExecutionSession_SetNarrative_Overwrite(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	session.SetNarrative("First narrative")
	session.SetNarrative("Second narrative")

	if session.Narrative != "Second narrative" {
		t.Errorf("Narrative = %q, want %q", session.Narrative, "Second narrative")
	}
}

func TestExecutionSession_Concurrent(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			session.AddStep(PlanStep{Index: idx, Task: "concurrent step"})
		}(i)
	}
	wg.Wait()

	if len(session.Steps) != 10 {
		t.Errorf("len(Steps) = %d after concurrent adds, want 10", len(session.Steps))
	}
}

func TestExecutionSession_Concurrent_Update(t *testing.T) {
	session := NewExecutionSession("s1", "p1", "q1")
	for i := 0; i < 5; i++ {
		session.AddStep(PlanStep{Index: i, Task: "Initial"})
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			session.UpdateStep(idx, func(step *PlanStep) {
				step.Status = StepStatusSuccess
			})
		}(i)
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		if session.Steps[i].Status != StepStatusSuccess {
			t.Errorf("Steps[%d].Status = %v, want %v", i, session.Steps[i].Status, StepStatusSuccess)
		}
	}
}

func TestAgentRequest_Structure(t *testing.T) {
	req := AgentRequest{
		ProjectID: "my-project",
		Query:     "find the auth flow",
	}

	if req.ProjectID != "my-project" {
		t.Errorf("AgentRequest.ProjectID = %q, want %q", req.ProjectID, "my-project")
	}
	if req.Query != "find the auth flow" {
		t.Errorf("AgentRequest.Query = %q, want %q", req.Query, "find the auth flow")
	}
}

func TestAgentResponse_Structure(t *testing.T) {
	resp := AgentResponse{
		SessionID: "session-abc",
		Steps: []PlanStep{
			{Index: 0, Task: "Find main", Query: "query1"},
		},
		Narrative: "The main function is at...",
	}

	if resp.SessionID != "session-abc" {
		t.Errorf("AgentResponse.SessionID = %q, want %q", resp.SessionID, "session-abc")
	}
	if len(resp.Steps) != 1 {
		t.Errorf("len(AgentResponse.Steps) = %d, want 1", len(resp.Steps))
	}
	if resp.Narrative == "" {
		t.Error("AgentResponse.Narrative should not be empty")
	}
}
