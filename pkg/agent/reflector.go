package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// NarrativeResult is the structured output for the UI.
type NarrativeResult struct {
	Summary  string         `json:"summary"`
	Evidence []EvidenceItem `json:"evidence"`
	Steps    []PlanStep     `json:"steps"`
}

type EvidenceItem struct {
	StepIndex int    `json:"step_index"`
	Query     string `json:"query"`
	Result    string `json:"result"`
	Count     int    `json:"count"`
}

// Reflector handles reflection on failures and narrative synthesis.
type Reflector struct {
	model ModelAdapter
}

// NewReflector creates a reflector backed by the given model.
func NewReflector(model ModelAdapter) *Reflector {
	return &Reflector{model: model}
}

// ReflectAndCorrect examines a failed step and proposes a corrected query.
func (r *Reflector) ReflectAndCorrect(ctx context.Context, session *ExecutionSession, stepIndex int) (string, error) {
	step := session.GetStep(stepIndex)
	if step == nil {
		return "", fmt.Errorf("step %d not found", stepIndex)
	}

	prompt := fmt.Sprintf(`The following Datalog query returned no results or failed. Suggest a corrected query.

Original Query: %s
Error: %s
User Intent: %s

Available predicates: defines, calls, imports, has_doc, in_package, has_role, has_tag, kind

Rules:
1. Return ONLY the corrected Datalog query, nothing else.
2. If the query references a result from a previous step, use a variable like ?ref instead of a literal.
3. The corrected query should be broader or fix the syntax issue.
4. Keep it to a single triple pattern if possible.`, step.Query, step.Error, session.Query)

	response, err := r.model.GenerateContent(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("reflection model call failed: %w", err)
	}

	// Extract just the query from the response
	corrected := strings.TrimSpace(response)
	// Remove markdown code fences if present
	corrected = strings.TrimPrefix(corrected, "```prolog")
	corrected = strings.TrimPrefix(corrected, "```datalog")
	corrected = strings.TrimPrefix(corrected, "```")
	corrected = strings.TrimSuffix(corrected, "```")
	corrected = strings.TrimSpace(corrected)

	log.Printf("[Agent/Reflector] Corrected step %d: %q -> %q", stepIndex, step.Query, corrected)
	return corrected, nil
}

// SynthesizeNarrative generates a final narrative from completed session steps.
func (r *Reflector) SynthesizeNarrative(ctx context.Context, session *ExecutionSession) (string, error) {
	var evidence strings.Builder
	for _, step := range session.Steps {
		if step.Status != StepStatusSuccess && step.Status != StepStatusCorrected {
			continue
		}
		evidence.WriteString(fmt.Sprintf("### Step %d: %s\n", step.Index, step.Task))
		evidence.WriteString(fmt.Sprintf("Query: `%s`\n", step.Query))
		evidence.WriteString(fmt.Sprintf("Results: %d rows\n", len(step.Result)))
		if len(step.Hydrated) > 0 {
			evidence.WriteString("Key findings:\n")
			for _, h := range step.Hydrated {
				evidence.WriteString(fmt.Sprintf("- %s (%s)\n", h.ID, h.Kind))
			}
		}
		evidence.WriteString("\n")
	}

	prompt := fmt.Sprintf(`Based on the following analysis steps, provide a concise narrative answering the user's question.

User Question: %s

Analysis Evidence:
%s

Instructions:
1. Answer the user's question directly.
2. Reference specific symbols/files found.
3. Keep the narrative to 2-4 paragraphs.
4. Use markdown formatting.`, session.Query, evidence.String())

	response, err := r.model.GenerateContent(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("narrative synthesis failed: %w", err)
	}

	return response, nil
}

// BuildEvidenceItems creates EvidenceItem list from session steps for the frontend.
func BuildEvidenceItems(session *ExecutionSession) []EvidenceItem {
	var items []EvidenceItem
	for _, step := range session.Steps {
		if step.Status == StepStatusPending {
			continue
		}
		resultSummary := fmt.Sprintf("%d results", len(step.Result))
		if step.Status == StepStatusFailed {
			resultSummary = fmt.Sprintf("Failed: %s", step.Error)
		}
		items = append(items, EvidenceItem{
			StepIndex: step.Index,
			Query:     step.Query,
			Result:    resultSummary,
			Count:     len(step.Result),
		})
	}
	return items
}
