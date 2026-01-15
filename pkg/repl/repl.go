package repl

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// Run starts the interactive REPL with intelligent feedback loop.
func Run(s *meb.MEBStore, readOnly bool) {
	fmt.Println("\n--- Interactive Query Mode ---")

	// Recalculate stats to ensure we have fresh counts
	if !readOnly {
		if _, err := s.RecalculateStats(); err != nil {
			log.Printf("Stats recalc error: %v", err)
		}
	}
	fmt.Printf("Total Facts: %d\n", s.Count())
	predsList := s.ListPredicates()
	fmt.Printf("Total Predicates: %d\n", len(predsList))
	for _, p := range predsList {
		fmt.Printf(" - %s\n", p)
	}

	// Generate and display project context
	fmt.Println("\n=== Project Context ===")
	projectContext, err := GenerateProjectSummary(s)
	if err != nil {
		log.Printf("Warning: Failed to generate project context: %v", err)
	} else {
		// Display packages
		if len(projectContext.Packages) > 0 {
			fmt.Printf("\n📦 Packages (%d):\n", len(projectContext.Packages))
			displayLimit := 10
			for i, pkg := range projectContext.Packages {
				if i >= displayLimit {
					fmt.Printf("   ... and %d more\n", len(projectContext.Packages)-displayLimit)
					break
				}
				fmt.Printf("   - %s\n", pkg)
			}
		}

		// Display top symbols
		if len(projectContext.TopSymbols) > 0 {
			fmt.Printf("\n🎯 Top Symbols (%d):\n", len(projectContext.TopSymbols))
			displayLimit := 15
			for i, symbol := range projectContext.TopSymbols {
				if i >= displayLimit {
					fmt.Printf("   ... and %d more\n", len(projectContext.TopSymbols)-displayLimit)
					break
				}
				fmt.Printf("   - %s\n", symbol)
			}
		}

		// Display stats
		if len(projectContext.Stats) > 0 {
			fmt.Printf("\n📊 Statistics:\n")
			if count, ok := projectContext.Stats["total_facts"]; ok {
				fmt.Printf("   - Total Facts: %d\n", count)
			}
			if count, ok := projectContext.Stats["unique_predicates"]; ok {
				fmt.Printf("   - Unique Predicates: %d\n", count)
			}
			if count, ok := projectContext.Stats["unique_packages"]; ok {
				fmt.Printf("   - Unique Packages: %d\n", count)
			}
			if count, ok := projectContext.Stats["facts_per_package"]; ok {
				fmt.Printf("   - Facts per Package: %d\n", count)
			}
		}
		fmt.Println()
	}

	fmt.Println("Enter datalog queries (e.g. triples(S, \"calls\", O)). Type 'exit' or 'quit' to stop.")
	scanner := bufio.NewScanner(os.Stdin)

	// Load the prompt templates at startup
	nlPromptPath := "prompts/nl_to_datalog.prompt"
	nlPrompt, err := LoadPrompt(nlPromptPath)
	if err != nil {
		log.Printf("Warning: Failed to load prompt from %s: %v. NL features may not work.", nlPromptPath, err)
	}

	explainPromptPath := "prompts/explain_results.prompt"
	explainPrompt, err := LoadPrompt(explainPromptPath)
	if err != nil {
		log.Printf("Warning: Failed to load explain prompt from %s: %v. Explanation features may not work.", explainPromptPath, err)
	}

	plannerPromptPath := "prompts/planner.prompt"
	plannerPrompt, err := LoadPrompt(plannerPromptPath)
	if err != nil {
		log.Printf("Warning: Failed to load planner prompt from %s: %v. Plan features may not work.", plannerPromptPath, err)
	}

	// Initialize session context
	session := NewSessionContext()

	// Convert predicates to strings for context
	var factStrings []string
	for _, p := range predsList {
		factStrings = append(factStrings, fmt.Sprintf("%v", p))
	}

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "exit" || line == "quit" {
			break
		}
		if line == "" {
			continue
		}

		// Handle plan command
		if strings.HasPrefix(line, "plan ") {
			goal := strings.TrimPrefix(line, "plan ")
			if plannerPrompt == nil {
				fmt.Println("Error: Planner prompt not loaded.")
				continue
			}
			if err := executePlanCommand(context.Background(), s, goal, projectContext, plannerPrompt); err != nil {
				fmt.Printf("❌ Plan execution failed: %v\n", err)
			}
			continue
		}

		// Handle export command
		if strings.HasPrefix(line, "export ") {
			argsStr := strings.TrimPrefix(line, "export ")
			var filterTests bool

			// Parse flags (Naive parser: assumes flags come first)
			for strings.HasPrefix(strings.TrimSpace(argsStr), "--") {
				argsStr = strings.TrimSpace(argsStr)
				idx := strings.Index(argsStr, " ")
				if idx == -1 {
					break
				}
				flag := argsStr[:idx]
				if flag == "--filter-tests" {
					filterTests = true
				}
				argsStr = strings.TrimSpace(argsStr[idx+1:])
			}

			lastSpace := strings.LastIndex(argsStr, " ")
			if lastSpace == -1 {
				fmt.Println("Usage: export [--filter-tests] <query> <filename>")
				continue
			}

			datalogQuery := strings.TrimSpace(argsStr[:lastSpace])
			filename := strings.TrimSpace(argsStr[lastSpace+1:])

			// Execute query
			results, err := s.Query(context.Background(), datalogQuery)
			if err != nil {
				fmt.Printf("Query error: %v\n", err)
				continue
			}

			if len(results) == 0 {
				fmt.Println("No results to export.")
				continue
			}

			// Export using D3Transformer with options
			transformer := export.NewD3Transformer(s)
			transformer.ExcludeTestFiles = filterTests

			graph, err := transformer.Transform(context.Background(), datalogQuery, results)
			if err != nil {
				fmt.Printf("Export error: %v\n", err)
				continue
			}

			if err := export.SaveD3Graph(graph, filename); err != nil {
				fmt.Printf("Save error: %v\n", err)
				continue
			}

			fmt.Printf("✅ Exported %d nodes and %d links to %s\n", len(graph.Nodes), len(graph.Links), filename)
			continue
		}

		// Detect if this is a follow-up query
		isFollowUp := isFollowUpQuery(line) && session.HasContext()

		// Detect if this is natural language or direct Datalog
		isNL := !strings.Contains(line, "(") && strings.Contains(line, " ")

		var nlQuery string
		var datalogQuery string

		if isNL || isFollowUp {
			if nlPrompt == nil {
				fmt.Println("Error: NL prompt not loaded.")
				continue
			}
			fmt.Println("💭 Thinking...")

			// Prepare template data with optional suggested queries context
			var prevSuggestions string
			if isFollowUp && session.HasContext() {
				prevSuggestions = session.GetLastSuggestions()
			}

			translated, err := askGeminiWithContext(context.Background(), nlPrompt, line, factStrings, prevSuggestions)
			if err != nil {
				fmt.Printf("Gemini Error: %v\n", err)
				continue
			}

			nlQuery = line
			datalogQuery = translated
			fmt.Printf("📝 Translated to: %s\n", datalogQuery)
		} else {
			// Direct Datalog query
			datalogQuery = line
		}

		// Execute the query
		results, err := s.Query(context.Background(), datalogQuery)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// Display results
		if len(results) == 0 {
			fmt.Println("📭 [No results]")
			continue
		}

		fmt.Printf("\n✅ Found %d results:\n", len(results))
		displayLimit := 10
		for i, r := range results {
			if i >= displayLimit {
				fmt.Printf("... and %d more\n", len(results)-displayLimit)
				break
			}
			fmt.Printf("- %v\n", r)
		}

		// Summarize results
		summary := SummarizeResults(results)
		session.UpdateContext(nlQuery, datalogQuery, results, summary)

		// Explain results if we have a natural language query and the explain prompt
		if nlQuery != "" && explainPrompt != nil {
			fmt.Println("\n🤖 Analyzing results...")
			explanation, err := explainResults(context.Background(), session, explainPrompt)
			if err != nil {
				log.Printf("Warning: Failed to generate explanation: %v", err)
			} else {
				fmt.Printf("\n📊 %s\n\n", explanation)

				// Extract suggested queries from the explanation
				suggestedQueries := extractSuggestedQueries(explanation)

				// Add to conversation history
				session.AddTurn(ConversationTurn{
					UserInput:        line,
					NLQuery:          nlQuery,
					DatalogQuery:     datalogQuery,
					ResultCount:      len(results),
					Explanation:      explanation,
					SuggestedQueries: suggestedQueries,
				})
			}
		}
	}
	fmt.Println("👋 Bye!")
}

// isFollowUpQuery detects if a query is a follow-up to a previous query.
func isFollowUpQuery(query string) bool {
	lower := strings.ToLower(query)
	followUpKeywords := []string{
		"why", "how come", "what about",
		"show me", "filter by", "only the", "just the",
		"exclude", "without", "except",
		"narrow down", "refine", "also",
	}

	for _, keyword := range followUpKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// explainResults generates a natural language explanation of query results.
func explainResults(ctx context.Context, session *SessionContext, explainPrompt *Prompt) (string, error) {
	if session.ResultSummary == nil {
		return "", fmt.Errorf("no result summary available")
	}

	// Prepare data for the explain template
	data := map[string]interface{}{
		"nl_query":            session.LastNLQuery,
		"datalog":             session.LastDatalog,
		"total_count":         session.ResultSummary.TotalCount,
		"is_truncated":        session.ResultSummary.IsTruncated,
		"sample_results":      session.ResultSummary.SampleResults,
		"frequent_predicates": session.ResultSummary.FrequentPredicates,
		"frequent_subjects":   session.ResultSummary.FrequentSubjects,
	}

	promptStr, err := explainPrompt.Execute(data)
	if err != nil {
		return "", fmt.Errorf("failed to execute explain template: %w", err)
	}

	// Call Gemini with the explain prompt
	apiKey := os.Getenv("GEMINI_API_KEY")
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

	model := client.GenerativeModel(explainPrompt.Config.Model)
	if explainPrompt.Config.Model == "" {
		model = client.GenerativeModel("gemini-3-flash-preview")
	}
	model.SetTemperature(explainPrompt.Config.Temperature)

	resp, err := model.GenerateContent(ctx, genai.Text(promptStr))
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("no response from Gemini")
	}

	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			return strings.TrimSpace(string(txt)), nil
		}
	}
	return "", fmt.Errorf("unexpected response format")
}

func parseArg(s string) string {
	s = strings.TrimSpace(s)
	// If it starts with uppercase, it's a variable -> empty string for Scan
	if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
		return ""
	}
	// If it's quoted, strip quotes
	return clean(s)
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\"", ""))
}

func askGemini(ctx context.Context, p *Prompt, question string, facts []string) (string, error) {
	return askGeminiWithContext(ctx, p, question, facts, "")
}

func askGeminiWithContext(ctx context.Context, p *Prompt, question string, facts []string, suggestedQueries string) (string, error) {

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}

	// Increased timeout to 30 seconds to handle complex queries
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel(p.Config.Model)
	if model == nil {
		// Fallback if config model is empty ?? or just let it default?
		// GenerativeModel returns a value, so we just use what we have.
		// If p.Config.Model is empty, it might fail. Let's assume the prompt file is valid.
		if p.Config.Model == "" {
			model = client.GenerativeModel("gemini-3-flash-preview")
		}
	}
	model.SetTemperature(p.Config.Temperature)

	// Prepare data for template
	data := map[string]interface{}{
		"query":             question,
		"context_facts":     facts,
		"suggested_queries": suggestedQueries,
	}

	promptStr, err := p.Execute(data)
	if err != nil {
		return "", fmt.Errorf("failed to execute prompt template: %w", err)
	}

	resp, err := model.GenerateContent(ctx, genai.Text(promptStr))

	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("no response from Gemini")
	}

	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			clean := strings.TrimSpace(string(txt))
			// Remove markdown code blocks if Gemini adds them
			clean = strings.TrimPrefix(clean, "```datalog")
			clean = strings.TrimPrefix(clean, "```")
			clean = strings.TrimSuffix(clean, "```")
			return strings.TrimSpace(clean), nil
		}
	}
	return "", fmt.Errorf("unexpected response format")
}

// executePlanCommand handles the "plan <goal>" command by generating and executing a multi-step plan.
func executePlanCommand(ctx context.Context, s *meb.MEBStore, goal string, projectContext *ProjectSummary, plannerPrompt *Prompt) error {
	fmt.Println("\n🧠 Analyzing codebase and generating execution plan...")

	// Prepare template data with project context
	data := map[string]interface{}{
		"Query":      goal,
		"Packages":   projectContext.Packages,
		"Predicates": projectContext.Predicates,
		"TopSymbols": projectContext.TopSymbols,
	}

	// Execute template to generate prompt
	promptStr, err := plannerPrompt.Execute(data)
	if err != nil {
		return fmt.Errorf("failed to execute planner template: %w", err)
	}

	// Call Gemini to generate plan
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(plannerPrompt.Config.Model)
	if plannerPrompt.Config.Model == "" {
		model = client.GenerativeModel("gemini-3-flash-preview")
	}
	model.SetTemperature(plannerPrompt.Config.Temperature)

	resp, err := model.GenerateContent(ctx, genai.Text(promptStr))
	if err != nil {
		return fmt.Errorf("Gemini API error: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return fmt.Errorf("no response from Gemini")
	}

	var planJSON string
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			planJSON = string(txt)
			break
		}
	}

	if planJSON == "" {
		return fmt.Errorf("empty response from Gemini")
	}

	// Parse JSON plan
	steps, err := parseJSONPlan(planJSON)
	if err != nil {
		return fmt.Errorf("failed to parse plan: %w", err)
	}

	// Create execution session
	session := NewExecutionSession(goal, steps)

	// Display plan to user
	DisplayPlan(session)

	// Get user confirmation
	confirmed, err := ConfirmExecution()
	if err != nil {
		return fmt.Errorf("failed to get user confirmation: %w", err)
	}

	if !confirmed {
		fmt.Println("\n❌ Plan execution cancelled by user.")
		return nil
	}

	// Execute the plan
	return ExecutePlan(ctx, s, session, plannerPrompt)
}
