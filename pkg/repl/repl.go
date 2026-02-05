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
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// Run starts the interactive REPL with intelligent feedback loop.
func Run(ctx context.Context, cfg Config, s *meb.MEBStore) {
	fmt.Println("\n--- Interactive Query Mode ---")

	// Recalculate stats to ensure we have fresh counts
	if !cfg.ReadOnly {
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
				fmt.Printf("   - %s (%d)\n", symbol.Name, symbol.Count)
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
	nlPromptPath := "prompts/datalog.prompt"
	nlPrompt, err := prompts.LoadPrompt(nlPromptPath)
	if err != nil {
		log.Printf("Warning: Failed to load prompt from %s: %v. NL features may not work.", nlPromptPath, err)
	}

	explainPromptPath := "prompts/explain_results.prompt"
	explainPrompt, err := prompts.LoadPrompt(explainPromptPath)
	if err != nil {
		log.Printf("Warning: Failed to load explain prompt from %s: %v. Explanation features may not work.", explainPromptPath, err)
	}

	plannerPromptPath := "prompts/planner.prompt"
	plannerPrompt, err := prompts.LoadPrompt(plannerPromptPath)
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
			if err := executePlanCommand(ctx, cfg, s, goal, projectContext, plannerPrompt); err != nil {
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

		// Handle search command
		if strings.HasPrefix(line, "search ") {
			query := strings.TrimPrefix(line, "search ")
			if query == "" {
				fmt.Println("Usage: search <query>")
				continue
			}

			fmt.Println("🔍 Analyzing query...")

			// 1. Extract keywords
			keywords, err := ExtractKeywords(context.Background(), query)
			if err != nil {
				fmt.Printf("⚠️ Keyword extraction failed (using raw query): %v\n", err)
				keywords = []string{query}
			} else {
				fmt.Printf("🔑 Keywords: %v\n", keywords)
			}

			// 2. Gather candidates from MEBStore
			// We scan all symbols in the dictionary
			var candidates []string
			err = s.IterateSymbols(func(sym string) bool {
				candidates = append(candidates, sym)
				return true
			})
			if err != nil {
				fmt.Printf("❌ Failed to scan symbols: %v\n", err)
				continue
			}

			// 3. Fuzzy Match
			// tailoredQuery combines keywords for better fuzzy matching context if needed,
			// but our search logic handles the raw query + tokenization.
			// Passing the raw user query usually works best with our logic,
			// but we can also append keywords if the query is very vague.
			// Let's pass the raw query.

			// Note: If we really want to leverage the "Technical Keywords" extracted by Gemini,
			// we should probably pass THOSE to the updated FindNodesBySimilarity or join them.
			// The current FindNodesBySimilarity takes a single query string.
			// Let's try joining keywords as the query, or better, stick to user query
			// and treat keywords as "boosters"??
			// Actually, let's trust the user's raw query for the Levenshtein part (typos),
			// and maybe the keywords help?
			//
			// If I use `keywords` joined by space, it might clean up "find the function" -> "function".
			// Let's try using the extracted keywords joined as the query string,
			// as Gemini is likely smarter at picking the important parts.
			searchQuery := strings.Join(keywords, " ")
			if len(keywords) == 0 {
				searchQuery = query
			}

			fmt.Printf("🔎 Searching for: %q\n", searchQuery)
			results := FindNodesBySimilarity(searchQuery, candidates)

			if len(results) == 0 {
				fmt.Println("📭 No matching nodes found.")
			} else {
				fmt.Printf("\n✅ Search Results (%d matches):\n", len(results))
				for i, r := range results {
					fmt.Printf("%d. %s\n", i+1, r)
				}
			}
			continue
		}

		// Handle show command
		if strings.HasPrefix(line, "show ") {
			arg := strings.TrimPrefix(line, "show ")
			HandleShow(context.Background(), s, arg)
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

			translated, err := askGeminiWithContext(ctx, cfg, nlPrompt, line, factStrings, prevSuggestions)
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
			explanation, err := explainResults(ctx, cfg, session, explainPrompt)
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
func explainResults(ctx context.Context, cfg Config, session *SessionContext, explainPrompt *prompts.Prompt) (string, error) {
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
	// Call Gemini with the explain prompt
	apiKey := cfg.GeminiAPIKey
	if apiKey == "" {
		return "", fmt.Errorf("Gemini API key not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	// Use model from config, which defaults to env or fallback in DefaultConfig()
	modelName := cfg.Model
	if explainPrompt.Config.Model != "" {
		modelName = explainPrompt.Config.Model
	}
	model := client.GenerativeModel(modelName)
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

func askGemini(ctx context.Context, cfg Config, p *prompts.Prompt, question string, facts []string) (string, error) {
	return askGeminiWithContext(ctx, cfg, p, question, facts, "")
}

func askGeminiWithContext(ctx context.Context, cfg Config, p *prompts.Prompt, question string, facts []string, suggestedQueries string) (string, error) {

	apiKey := cfg.GeminiAPIKey
	if apiKey == "" {
		return "", fmt.Errorf("Gemini API key not configured")
	}

	// Increased timeout to 30 seconds to handle complex queries
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	// Use model from config (which accounts for env var)
	// If prompt config overrides it, use that?
	// Usually prompt config model is bare or specific. Let's prefer global config if prompt doesn't specify?
	// ACTUALLY, repl.Config.Model is the source of truth for the session.
	modelName := cfg.Model
	if p.Config.Model != "" {
		modelName = p.Config.Model
	}

	model := client.GenerativeModel(modelName)
	model.SetTemperature(p.Config.Temperature)

	// Prepare data for template
	data := map[string]interface{}{
		"Query":            question,
		"Predicates":       formatPredicatesListSection(facts),
		"SuggestedQueries": suggestedQueries,
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
func executePlanCommand(ctx context.Context, cfg Config, s *meb.MEBStore, goal string, projectContext *ProjectSummary, plannerPrompt *prompts.Prompt) error {
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
	apiKey := cfg.GeminiAPIKey
	if apiKey == "" {
		return fmt.Errorf("Gemini API key not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	// Use model from config
	modelName := cfg.Model
	if plannerPrompt.Config.Model != "" {
		modelName = plannerPrompt.Config.Model
	}
	model := client.GenerativeModel(modelName)
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
	return ExecutePlan(ctx, cfg, s, session, plannerPrompt)
}
