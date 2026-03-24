package agent

import (
	"sync"
	"time"
)

// StepStatus represents the lifecycle of a PlanStep.
type StepStatus string

const (
	StepStatusPending   StepStatus = "Pending"
	StepStatusRunning   StepStatus = "Running"
	StepStatusSuccess   StepStatus = "Success"
	StepStatusFailed    StepStatus = "Failed"
	StepStatusCorrected StepStatus = "Corrected"
)

// PlanStep is a single unit of work produced by the Planner.
type PlanStep struct {
	Index     int              `json:"index"`
	Task      string           `json:"task"`
	Query     string           `json:"query"`
	Status    StepStatus       `json:"status"`
	Result    []map[string]any `json:"result,omitempty"`
	Hydrated  []HydratedNode   `json:"hydrated,omitempty"`
	Error     string           `json:"error,omitempty"`
	StartTime *time.Time       `json:"start_time,omitempty"`
	EndTime   *time.Time       `json:"end_time,omitempty"`
}

// HydratedNode is a graph node with source code injected.
type HydratedNode struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Kind     string            `json:"kind"`
	Code     string            `json:"code,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ExecutionSession tracks a multi-step agent reasoning session.
type ExecutionSession struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Query     string     `json:"query"`
	Steps     []PlanStep `json:"steps"`
	Narrative string     `json:"narrative,omitempty"`
	CreatedAt time.Time  `json:"created_at"`

	mu sync.RWMutex
}

// NewExecutionSession creates a new session.
func NewExecutionSession(id, projectID, query string) *ExecutionSession {
	return &ExecutionSession{
		ID:        id,
		ProjectID: projectID,
		Query:     query,
		Steps:     make([]PlanStep, 0),
		CreatedAt: time.Now(),
	}
}

// AddStep appends a plan step.
func (s *ExecutionSession) AddStep(step PlanStep) {
	s.mu.Lock()
	defer s.mu.Unlock()
	step.Status = StepStatusPending
	s.Steps = append(s.Steps, step)
}

// UpdateStep modifies a step by index.
func (s *ExecutionSession) UpdateStep(index int, fn func(*PlanStep)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index >= 0 && index < len(s.Steps) {
		fn(&s.Steps[index])
	}
}

// GetStep returns a copy of the step at the given index.
func (s *ExecutionSession) GetStep(index int) *PlanStep {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index >= 0 && index < len(s.Steps) {
		step := s.Steps[index]
		return &step
	}
	return nil
}

// SetNarrative sets the final narrative.
func (s *ExecutionSession) SetNarrative(n string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Narrative = n
}

// AgentRequest is the JSON body for the /api/v1/agent/execute endpoint.
type AgentRequest struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
}

// AgentResponse is the JSON response for the /api/v1/agent/execute endpoint.
type AgentResponse struct {
	SessionID string     `json:"session_id"`
	Steps     []PlanStep `json:"steps"`
	Narrative string     `json:"narrative"`
}
