package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

// ExecutePlan executes the plan steps.
func ExecutePlan(ctx context.Context, cfg Config, s *meb.MEBStore, session *ExecutionSession, plannerPrompt *prompts.Prompt) error {
	fmt.Printf("\n🚀 Executing plan: %s\n\n", session.Goal)

	for !session.IsComplete() {
		step := session.NextStep()
		if step == nil {
			break
		}

		fmt.Printf("Step %d: %s\n", step.ID, step.Task)
		fmt.Printf("  Query: %s\n", step.Query)

		expanded := expandVariables(step.Query, session)
		fmt.Printf("  Expanded: %s\n", expanded)

		results, err := gcamdb.Query(ctx, s, expanded)
		if err != nil {
			fmt.Printf("  ❌ Error: %v\n", err)

			corrected, corrErr := reflectAndCorrect(ctx, cfg, step, session, plannerPrompt)
			if corrErr != nil {
				fmt.Printf("  ❌ Correction failed: %v\n", corrErr)
				continue
			}

			fmt.Printf("  🔄 Trying corrected query: %s\n", corrected)
			results, err = gcamdb.Query(ctx, s, corrected)
			if err != nil {
				fmt.Printf("  ❌ Corrected query also failed: %v\n", err)
				continue
			}
		}

		if len(results) == 0 {
			fmt.Println("  📭 No results")

			corrected, corrErr := reflectAndCorrect(ctx, cfg, step, session, plannerPrompt)
			if corrErr != nil {
				fmt.Printf("  ❌ Correction failed: %v\n", corrErr)
				continue
			}

			fmt.Printf("  🔄 Trying corrected query: %s\n", corrected)
			results, err = gcamdb.Query(ctx, s, corrected)
			if err != nil {
				fmt.Printf("  ❌ Corrected query also failed: %v\n", err)
				continue
			}
		}

		stepResults := make([]map[string]string, 0, len(results))
		for _, r := range results {
			strMap := make(map[string]string)
			for k, v := range r {
				strMap[k] = fmt.Sprintf("%v", v)
			}
			stepResults = append(stepResults, strMap)
		}
		session.StoreResults(step.ID, stepResults)

		fmt.Printf("  ✅ %d results\n\n", len(results))
	}

	fmt.Println("\n✅ Plan execution complete!")
	return nil
}

// expandVariables replaces variable references in a query with actual values from previous steps.
func expandVariables(query string, session *ExecutionSession) string {
	expanded := query

	for _, step := range session.Steps {
		if step.ID >= session.CurrentStep {
			break
		}

		results := session.GetResults(step.ID)
		if len(results) == 0 {
			continue
		}

		for _, result := range results {
			for varName, value := range result {
				placeholder := fmt.Sprintf("{{step_%d_%s}}", step.ID, varName)
				expanded = strings.ReplaceAll(expanded, placeholder, value)
			}
		}
	}

	return expanded
}

// reflectAndCorrect asks the LLM to suggest an alternative query when a step fails or returns no results.
func reflectAndCorrect(ctx context.Context, cfg Config, step *PlanStep, session *ExecutionSession, plannerPrompt *prompts.Prompt) (string, error) {
	reflectPromptFile, err := prompts.LoadPrompt("prompts/reflect.prompt")
	if err != nil || reflectPromptFile == nil {
		return "", fmt.Errorf("failed to load reflect.prompt: %w", err)
	}

	promptStr, err := reflectPromptFile.Execute(map[string]interface{}{
		"StepID": step.ID,
		"Task":   step.Task,
		"Query":  step.Query,
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute reflect prompt template: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := genkit.Generate(ctx, cfg.Genkit,
		ai.WithModelName(cfg.Model),
		ai.WithPrompt(promptStr),
	)
	if err != nil {
		return "", err
	}

	if resp.Text() == "" {
		return "", fmt.Errorf("no response from LLM")
	}

	clean := strings.TrimSpace(resp.Text())
	clean = strings.TrimPrefix(clean, "```datalog")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	return strings.TrimSpace(clean), nil
}

// ConfirmExecution prompts the user for [Y/n] confirmation to proceed with plan execution.
func ConfirmExecution() (bool, error) {
	fmt.Print("\n📋 Proceed with execution? [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" || response == "y" || response == "yes" {
		return true, nil
	}

	return false, nil
}

// parseJSONPlan parses an LLM response containing a JSON plan, handling markdown backticks.
func parseJSONPlan(response string) ([]PlanStep, error) {
	clean := strings.TrimSpace(response)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var steps []PlanStep
	if err := json.Unmarshal([]byte(clean), &steps); err != nil {
		return nil, fmt.Errorf("failed to parse JSON plan: %w\nResponse was: %s", err, clean)
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("plan contains no steps")
	}

	return steps, nil
}
