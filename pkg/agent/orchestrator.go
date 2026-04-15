package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
	"github.com/google/uuid"
)

// Orchestrator drives the full agent pipeline: Plan -> Execute -> Reflect -> Narrate.
type Orchestrator struct {
	planner   *Planner
	executor  *Executor
	reflector *Reflector
}

// NewOrchestrator wires the three components together.
func NewOrchestrator(model ModelAdapter, store *meb.MEBStore) *Orchestrator {
	return &Orchestrator{
		planner:   NewPlanner(model),
		executor:  NewExecutor(store),
		reflector: NewReflector(model),
	}
}

// Run executes the full agent pipeline and returns the completed session.
func (o *Orchestrator) Run(ctx context.Context, projectID, query string, predicates []string) (*ExecutionSession, error) {
	sessionID := uuid.New().String()
	session := NewExecutionSession(sessionID, projectID, query)

	logger.Info("Agent/Orchestrator Starting session", "sessionID", sessionID, "projectID", projectID)

	// Phase 1: Plan
	planCtx, planCancel := context.WithTimeout(ctx, 30*time.Second)
	defer planCancel()

	steps, err := o.planner.Plan(planCtx, query, predicates)
	if err != nil {
		return nil, fmt.Errorf("planning failed: %w", err)
	}

	for _, step := range steps {
		session.AddStep(step)
	}

	logger.Debug("Agent/Orchestrator Plan generated", "steps", len(steps))

	// Phase 2: Execute all steps
	if err := o.executor.ExecuteAllSteps(ctx, session); err != nil {
		logger.Error("Agent/Orchestrator Execution completed with errors", "error", err)
		// Continue to narrative synthesis even with partial failures
	}

	// Phase 3: Synthesize narrative
	narrCtx, narrCancel := context.WithTimeout(ctx, 30*time.Second)
	defer narrCancel()

	narrative, err := o.reflector.SynthesizeNarrative(narrCtx, session)
	if err != nil {
		logger.Warn("Agent/Orchestrator Narrative synthesis failed", "error", err)
		narrative = o.buildFallbackNarrative(session)
	}

	session.SetNarrative(narrative)

	logger.Info("Agent/Orchestrator Session completed", "sessionID", sessionID, "duration", time.Since(session.CreatedAt))
	return session, nil
}

// buildFallbackNarrative creates a simple summary when AI synthesis fails.
func (o *Orchestrator) buildFallbackNarrative(session *ExecutionSession) string {
	var sb string
	sb += fmt.Sprintf("Analysis of: %s\n\n", session.Query)

	successCount := 0
	totalResults := 0
	for _, step := range session.Steps {
		if step.Status == StepStatusSuccess || step.Status == StepStatusCorrected {
			successCount++
			totalResults += len(step.Result)
		}
	}

	sb += fmt.Sprintf("Completed %d/%d analysis steps, finding %d results.\n", successCount, len(session.Steps), totalResults)

	if totalResults > 0 {
		sb += "\nKey findings:\n"
		// Show first few hydrated nodes
		count := 0
		for _, step := range session.Steps {
			for _, h := range step.Hydrated {
				if count >= 5 {
					break
				}
				sb += fmt.Sprintf("- %s (%s)\n", h.ID, h.Kind)
				count++
			}
		}
	}

	return sb
}
