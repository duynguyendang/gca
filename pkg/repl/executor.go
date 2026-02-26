package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ExecutePlan executes the plan steps.
func ExecutePlan(ctx context.Context, cfg Config, s *meb.MEBStore, session *ExecutionSession, plannerPrompt *prompts.Prompt) error {
	fmt.Printf("\n🚀 Executing plan: %s\n\n", session.Goal)

	for !session.IsComplete() {
		step := session.NextStep()
		if step == nil {
			break
		}

		fmt.Printf("⏳ Step %d/%d: %s\n", step.ID, len(session.Steps), step.Task)

		// Process variable injection
		query := processVariableInjection(step.Query, session)
		if query != step.Query {
			fmt.Printf("   📝 Expanded query: %s\n", query)
		} else {
			fmt.Printf("   📝 Query: %s\n", query)
		}

		// Execute with timeout
		results, err := executeWithTimeout(ctx, s, query, 30*time.Second)
		if err != nil {
			fmt.Printf("   ❌ Error: %v\n\n", err)

			// Attempt self-correction
			fmt.Println("   🤔 Attempting self-correction...")
			correctedQuery, corrErr := reflectAndCorrect(ctx, cfg, step, session, plannerPrompt)
			if corrErr != nil {
				fmt.Printf("   ❌ Self-correction failed: %v\n\n", corrErr)
				continue
			}

			fmt.Printf("   💡 Trying alternative: %s\n", correctedQuery)
			results, err = executeWithTimeout(ctx, s, correctedQuery, 30*time.Second)
			if err != nil {
				fmt.Printf("   ❌ Alternative also failed: %v\n\n", err)
				continue
			}
		}

		// Check for zero results and attempt reflection
		if len(results) == 0 {
			fmt.Println("   📭 No results found.")
			fmt.Println("   🤔 Attempting self-correction...")

			correctedQuery, corrErr := reflectAndCorrect(ctx, cfg, step, session, plannerPrompt)
			if corrErr != nil {
				fmt.Printf("   ⚠️  Self-correction failed: %v\n\n", corrErr)
				// Store empty results and continue
				session.StoreResults(step.ID, results)
				continue
			}

			fmt.Printf("   💡 Trying alternative: %s\n", correctedQuery)
			results, err = executeWithTimeout(ctx, s, correctedQuery, 30*time.Second)
			if err != nil {
				fmt.Printf("   ❌ Alternative failed: %v\n\n", err)
				session.StoreResults(step.ID, []map[string]string{})
				continue
			}
		}

		// Store results
		session.StoreResults(step.ID, results)

		// Display summary
		fmt.Printf("   ✅ Found %d results\n", len(results))
		if len(results) > 0 && len(results) <= 5 {
			for _, r := range results {
				fmt.Printf("      - %v\n", r)
			}
		} else if len(results) > 5 {
			for i := 0; i < 3; i++ {
				fmt.Printf("      - %v\n", results[i])
			}
			fmt.Printf("      ... and %d more\n", len(results)-3)
		}
		fmt.Println()
	}

	// Final summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📊 Execution Summary")
	fmt.Println(strings.Repeat("=", 60))
	totalResults := 0
	for stepID, results := range session.Results {
		totalResults += len(results)
		fmt.Printf("Step %d: %d results\n", stepID, len(results))
	}
	fmt.Printf("\nTotal results across all steps: %d\n", totalResults)
	fmt.Println(strings.Repeat("=", 60) + "\n")

	return nil
}

// executeWithTimeout executes a Datalog query with a timeout.
func executeWithTimeout(ctx context.Context, s *meb.MEBStore, query string, timeout time.Duration) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultsChan := make(chan []map[string]any, 1)
	errChan := make(chan error, 1)

	go func() {
		results, err := s.Query(ctx, query)
		if err != nil {
			errChan <- err
			return
		}
		resultsChan <- results
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("query timeout after %v", timeout)
	case err := <-errChan:
		return nil, err
	case results := <-resultsChan:
		// Convert []map[string]any to []map[string]string
		stringResults := make([]map[string]string, len(results))
		for i, result := range results {
			stringResult := make(map[string]string)
			for k, v := range result {
				stringResult[k] = fmt.Sprintf("%v", v)
			}
			stringResults[i] = stringResult
		}
		return stringResults, nil
	}
}

// processVariableInjection replaces placeholders like "$1.A" with actual values from previous steps.
func processVariableInjection(query string, session *ExecutionSession) string {
	// Regex to match $<stepID>.<VarName> patterns
	re := regexp.MustCompile(`\$(\d+)\.([A-Z])`)

	expanded := re.ReplaceAllStringFunc(query, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		stepID := 0
		fmt.Sscanf(parts[1], "%d", &stepID)
		varName := parts[2]

		// Get bindings from previous step
		bindings := session.GetAllVariableBindings(stepID, varName)
		if len(bindings) == 0 {
			// No bindings found, keep original placeholder
			return match
		}

		// For simplicity, use the first binding
		// In a more advanced implementation, we could expand to multiple queries
		// or use an IN clause pattern
		return fmt.Sprintf("\"%s\"", bindings[0])
	})

	return expanded
}

// reflectAndCorrect asks Gemini to suggest an alternative query when a step fails or returns no results.
// reflectAndCorrect asks Gemini to suggest an alternative query when a step fails or returns no results.
func reflectAndCorrect(ctx context.Context, cfg Config, step *PlanStep, session *ExecutionSession, plannerPrompt *prompts.Prompt) (string, error) {
	apiKey := cfg.GeminiAPIKey
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel(cfg.Model)
	model.SetTemperature(0.2)

	// Create reflection prompt
	reflectPrompt := fmt.Sprintf(`You are the GCA Lead Architect debugging a failed query step.

Step %d: %s
Query: %s
Result: NO RESULTS or ERROR

The query returned no results or failed. Suggest a revised Datalog query that might work better.

Consider:
1. Are the predicate names correct?
2. Are variables used consistently?
3. Is the regex pattern valid?
4. Could the query be too restrictive?

Return ONLY the revised Datalog query, nothing else. No explanations, no markdown.`,
		step.ID, step.Task, step.Query)

	resp, err := model.GenerateContent(ctx, genai.Text(reflectPrompt))
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("no response from Gemini")
	}

	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			clean := strings.TrimSpace(string(txt))
			// Remove markdown code blocks if present
			clean = strings.TrimPrefix(clean, "```datalog")
			clean = strings.TrimPrefix(clean, "```")
			clean = strings.TrimSuffix(clean, "```")
			return strings.TrimSpace(clean), nil
		}
	}

	return "", fmt.Errorf("unexpected response format")
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

	// Default to Yes if user just presses Enter
	if response == "" || response == "y" || response == "yes" {
		return true, nil
	}

	return false, nil
}

// parseJSONPlan parses a Gemini response containing a JSON plan, handling markdown backticks.
func parseJSONPlan(response string) ([]PlanStep, error) {
	// Strip markdown backticks if present
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
