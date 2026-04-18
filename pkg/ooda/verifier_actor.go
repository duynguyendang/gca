package ooda

import (
	"context"
	"fmt"
	"strings"
)

type Policy struct {
	MaxPromptLength     int
	MaxContextItems     int
	AllowedTasks        map[GCATask]bool
	BlockPatterns       []string
	RequireSymbolOnTask map[GCATask]bool
}

var DefaultPolicy = Policy{
	MaxPromptLength: 8000,
	MaxContextItems: 50,
	AllowedTasks: map[GCATask]bool{
		TaskInsight:             true,
		TaskChat:                true,
		TaskPrune:               true,
		TaskSummary:             true,
		TaskNarrative:           true,
		TaskResolveSymbol:       true,
		TaskPathEndpoints:       true,
		TaskDatalog:             true,
		TaskPathNarrative:       true,
		TaskSmartSearchAnalysis: true,
		TaskMultiFileSummary:    true,
		TaskRefactor:            true,
		TaskTestGeneration:      true,
		TaskSecurityAudit:       true,
		TaskPerformance:         true,
	},
	BlockPatterns: []string{
		"delete",
		"drop",
		"remove all",
		"truncate",
	},
	RequireSymbolOnTask: map[GCATask]bool{
		TaskInsight: true,
	},
}

type PolicyVerifier struct {
	policy Policy
}

func NewPolicyVerifier(policy Policy) *PolicyVerifier {
	if policy.MaxPromptLength == 0 {
		policy.MaxPromptLength = DefaultPolicy.MaxPromptLength
	}
	if policy.MaxContextItems == 0 {
		policy.MaxContextItems = DefaultPolicy.MaxContextItems
	}
	if policy.AllowedTasks == nil {
		policy.AllowedTasks = DefaultPolicy.AllowedTasks
	}
	if policy.BlockPatterns == nil {
		policy.BlockPatterns = DefaultPolicy.BlockPatterns
	}
	if policy.RequireSymbolOnTask == nil {
		policy.RequireSymbolOnTask = DefaultPolicy.RequireSymbolOnTask
	}

	return &PolicyVerifier{
		policy: policy,
	}
}

func (v *PolicyVerifier) Verify(ctx context.Context, frame *GCAFrame) error {
	frame.Phase = PhaseVerify

	if !v.policy.AllowedTasks[frame.Task] {
		frame.Status = VerifyStatusFailed
		frame.Proof = &AuditResult{
			Pass:          false,
			ViolationTier: "TIER_1",
			ConflictPath:  "policy:allowed_tasks",
			ProofTree:     fmt.Sprintf("Task %s is not allowed", frame.Task),
		}
		return fmt.Errorf("task %s is not allowed by policy", frame.Task)
	}

	if requireSymbol := v.policy.RequireSymbolOnTask[frame.Task]; requireSymbol {
		if frame.SymbolID == "" {
			frame.Status = VerifyStatusFailed
			frame.Proof = &AuditResult{
				Pass:          false,
				ViolationTier: "TIER_1",
				ConflictPath:  "policy:require_symbol",
				ProofTree:     fmt.Sprintf("Task %s requires a symbol ID", frame.Task),
			}
			return fmt.Errorf("task %s requires a symbol ID", frame.Task)
		}
	}

	if len(frame.Prompt) > v.policy.MaxPromptLength {
		frame.Status = VerifyStatusWarning
		frame.Proof = &AuditResult{
			Pass:          true,
			ViolationTier: "TIER_1",
			ProofTree:     fmt.Sprintf("Prompt length %d exceeds limit %d", len(frame.Prompt), v.policy.MaxPromptLength),
		}
	}

	if len(frame.Context) > v.policy.MaxContextItems {
		frame.Status = VerifyStatusWarning
		frame.Proof = &AuditResult{
			Pass:          true,
			ViolationTier: "TIER_1",
			ProofTree:     fmt.Sprintf("Context items %d exceeds limit %d", len(frame.Context), v.policy.MaxContextItems),
		}
	}

	inputLower := strings.ToLower(frame.Input)
	for _, pattern := range v.policy.BlockPatterns {
		if strings.Contains(inputLower, pattern) {
			frame.Status = VerifyStatusFailed
			frame.Proof = &AuditResult{
				Pass:          false,
				ViolationTier: "TIER_0",
				ConflictPath:  "policy:block_patterns",
				ProofTree:     fmt.Sprintf("Input contains blocked pattern: %s", pattern),
			}
			return fmt.Errorf("input contains blocked pattern: %s", pattern)
		}
	}

	frame.Status = VerifyStatusPassed
	frame.Proof = &AuditResult{
		Pass:          true,
		ViolationTier: "",
	}

	frame.Context = append(frame.Context, Atom{
		Predicate: "policy_verified",
		Subject:   frame.ID.String(),
		Object:    "passed",
		Weight:    1.0,
	})

	return nil
}

type GeminiActor struct {
	model Model
}

type Model interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

func NewGeminiActor(model Model) *GeminiActor {
	return &GeminiActor{model: model}
}

func (a *GeminiActor) Act(ctx context.Context, frame *GCAFrame) error {
	frame.Phase = PhaseAct

	if frame.Prompt == "" {
		frame.ExecError = fmt.Errorf("no prompt to execute")
		return frame.ExecError
	}

	response, err := a.model.GenerateContent(ctx, frame.Prompt)
	if err != nil {
		frame.ExecError = err
		frame.Status = VerifyStatusFailed
		return fmt.Errorf("gemini call failed: %w", err)
	}

	frame.Response = response

	frame.Context = append(frame.Context, Atom{
		Predicate: "response_received",
		Subject:   frame.ID.String(),
		Object:    fmt.Sprintf("length:%d", len(response)),
		Weight:    1.0,
	})

	return nil
}
