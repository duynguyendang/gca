package repl

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

// Run starts the interactive REPL with intelligent feedback loop.
func Run(ctx context.Context, cfg Config, s *meb.MEBStore) {
	fmt.Println("\n--- Interactive Query Mode ---")

	// Initialize REPL
	projectContext, factStrings := initializeREPL(cfg, s)

	// Load prompt templates
	nlPrompt, explainPrompt, plannerPrompt := loadPromptTemplates()

	// Initialize session context
	session := NewSessionContext()

	fmt.Println("Enter datalog queries (e.g. triples(S, \"calls\", O)). Type 'exit' or 'quit' to stop.")
	scanner := bufio.NewScanner(os.Stdin)

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

		// Process commands
		if processCommand(ctx, cfg, s, line, projectContext, plannerPrompt) {
			continue
		}

		// Process query
		processQuery(ctx, cfg, s, line, session, nlPrompt, explainPrompt, factStrings)
	}
	fmt.Println("👋 Bye!")
}

// initializeREPL sets up the REPL environment and displays initial information.
func initializeREPL(cfg Config, s *meb.MEBStore) (*ProjectSummary, []string) {
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

	fmt.Println("\n=== Project Context ===")
	projectContext, err := GenerateProjectSummary(s)
	if err != nil {
		log.Printf("Warning: Failed to generate project context: %v", err)
	} else {
		displayProjectContext(projectContext)
	}

	var factStrings []string
	for _, p := range predsList {
		factStrings = append(factStrings, fmt.Sprintf("%v", p))
	}

	return projectContext, factStrings
}

// displayProjectContext shows project summary information.
func displayProjectContext(projectContext *ProjectSummary) {
	if len(projectContext.Packages) > 0 {
		fmt.Printf("\n📦 Packages (%d):\n", len(projectContext.Packages))
		displayLimit := config.DisplayLimitSmall
		for i, pkg := range projectContext.Packages {
			if i >= displayLimit {
				fmt.Printf("   ... and %d more\n", len(projectContext.Packages)-displayLimit)
				break
			}
			fmt.Printf("   - %s\n", pkg)
		}
	}

	if len(projectContext.TopSymbols) > 0 {
		fmt.Printf("\n🎯 Top Symbols (%d):\n", len(projectContext.TopSymbols))
		displayLimit := config.DisplayLimitMedium
		for i, symbol := range projectContext.TopSymbols {
			if i >= displayLimit {
				fmt.Printf("   ... and %d more\n", len(projectContext.TopSymbols)-displayLimit)
				break
			}
			fmt.Printf("   - %s (%d)\n", symbol.Name, symbol.Count)
		}
	}

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

// loadPromptTemplates loads all required prompt templates.
func loadPromptTemplates() (*prompts.Prompt, *prompts.Prompt, *prompts.Prompt) {
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

	return nlPrompt, explainPrompt, plannerPrompt
}

// processCommand handles special REPL commands (plan, export, search, show).
func processCommand(ctx context.Context, cfg Config, s *meb.MEBStore, line string, projectContext *ProjectSummary, plannerPrompt *prompts.Prompt) bool {
	if strings.HasPrefix(line, "plan ") {
		goal := strings.TrimPrefix(line, "plan ")
		if plannerPrompt == nil {
			fmt.Println("Error: Planner prompt not loaded.")
			return true
		}
		if err := executePlanCommand(ctx, cfg, s, goal, projectContext, plannerPrompt); err != nil {
			fmt.Printf("❌ Plan execution failed: %v\n", err)
		}
		return true
	}

	if strings.HasPrefix(line, "export ") {
		processExportCommand(s, line)
		return true
	}

	if strings.HasPrefix(line, "search ") {
		processSearchCommand(ctx, cfg, line)
		return true
	}

	if strings.HasPrefix(line, "show ") {
		arg := strings.TrimPrefix(line, "show ")
		HandleShow(context.Background(), s, arg)
		return true
	}

	return false
}

// processExportCommand handles the export command.
func processExportCommand(s *meb.MEBStore, line string) {
	argsStr := strings.TrimPrefix(line, "export ")
	var filterTests bool

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
		return
	}

	datalogQuery := strings.TrimSpace(argsStr[:lastSpace])
	filename := strings.TrimSpace(argsStr[lastSpace+1:])

	results, err := gcamdb.Query(context.Background(), s, datalogQuery)
	if err != nil {
		fmt.Printf("Query error: %v\n", err)
		return
	}

	if len(results) == 0 {
		fmt.Println("No results to export.")
		return
	}

	transformer := export.NewD3Transformer(s)
	transformer.ExcludeTestFiles = filterTests

	graph, err := transformer.Transform(context.Background(), datalogQuery, results)
	if err != nil {
		fmt.Printf("Export error: %v\n", err)
		return
	}

	if err := export.SaveD3Graph(graph, filename); err != nil {
		fmt.Printf("Save error: %v\n", err)
		return
	}

	fmt.Printf("✅ Exported %d nodes and %d links to %s\n", len(graph.Nodes), len(graph.Links), filename)
}

// processSearchCommand handles the search command.
func processSearchCommand(ctx context.Context, cfg Config, line string) {
	query := strings.TrimPrefix(line, "search ")
	if query == "" {
		fmt.Println("Usage: search <query>")
		return
	}

	fmt.Println("🔍 Analyzing query...")

	keywords, err := ExtractKeywords(ctx, cfg.Genkit, query)
	if err != nil {
		fmt.Printf("⚠️ Keyword extraction failed (using raw query): %v\n", err)
		keywords = []string{query}
	} else {
		fmt.Printf("🔑 Keywords: %v\n", keywords)
	}

	searchQuery := strings.Join(keywords, " ")
	if len(keywords) == 0 {
		searchQuery = query
	}

	fmt.Printf("🔎 Searching for: %q\n", searchQuery)
	results := FindNodesBySimilarity(searchQuery, nil)

	if len(results) == 0 {
		fmt.Println("📭 No matching nodes found.")
	} else {
		fmt.Printf("\n✅ Search Results (%d matches):\n", len(results))
		for i, r := range results {
			fmt.Printf("%d. %s\n", i+1, r)
		}
	}
}

// processQuery handles natural language and datalog query processing.
func processQuery(ctx context.Context, cfg Config, s *meb.MEBStore, line string, session *SessionContext, nlPrompt *prompts.Prompt, explainPrompt *prompts.Prompt, factStrings []string) {
	isFollowUp := isFollowUpQuery(line) && session.HasContext()
	isNL := !strings.Contains(line, "(") && strings.Contains(line, " ")

	var nlQuery string
	var datalogQuery string

	if isNL || isFollowUp {
		if nlPrompt == nil {
			fmt.Println("Error: NL prompt not loaded.")
			return
		}
		fmt.Println("💭 Thinking...")

		var prevSuggestions string
		if isFollowUp && session.HasContext() {
			prevSuggestions = session.GetLastSuggestions()
		}

		translated, err := askLLMWithContext(ctx, cfg, nlPrompt, line, factStrings, prevSuggestions)
		if err != nil {
			fmt.Printf("LLM Error: %v\n", err)
			return
		}

		nlQuery = line
		datalogQuery = translated
		fmt.Printf("📝 Translated to: %s\n", datalogQuery)
	} else {
		datalogQuery = line
	}

	results, err := gcamdb.Query(context.Background(), s, datalogQuery)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	displayResults(results)

	summary := SummarizeResults(results)
	session.UpdateContext(nlQuery, datalogQuery, results, summary)

	if nlQuery != "" && explainPrompt != nil {
		fmt.Println("\n🤖 Analyzing results...")
		explanation, err := explainResults(ctx, cfg, session, explainPrompt)
		if err != nil {
			log.Printf("Warning: Failed to generate explanation: %v", err)
		} else {
			fmt.Printf("\n📊 %s\n\n", explanation)

			suggestedQueries := extractSuggestedQueries(explanation)

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

// displayResults formats and displays query results.
func displayResults(results []map[string]any) {
	if len(results) == 0 {
		fmt.Println("📭 [No results]")
		return
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

	modelName := cfg.Model
	if explainPrompt.Config.Model != "" {
		modelName = explainPrompt.Config.Model
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := genkit.Generate(ctx, cfg.Genkit,
		ai.WithModelName(modelName),
		ai.WithPrompt(promptStr),
	)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Text()), nil
}

func parseArg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
		return ""
	}
	return clean(s)
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\"", ""))
}

func askLLM(ctx context.Context, cfg Config, p *prompts.Prompt, question string, facts []string) (string, error) {
	return askLLMWithContext(ctx, cfg, p, question, facts, "")
}

func askLLMWithContext(ctx context.Context, cfg Config, p *prompts.Prompt, question string, facts []string, suggestedQueries string) (string, error) {
	modelName := cfg.Model
	if p.Config.Model != "" {
		modelName = p.Config.Model
	}

	data := map[string]interface{}{
		"Query":            question,
		"Predicates":       formatPredicatesListSection(facts),
		"SuggestedQueries": suggestedQueries,
	}

	promptStr, err := p.Execute(data)
	if err != nil {
		return "", fmt.Errorf("failed to execute prompt template: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := genkit.Generate(ctx, cfg.Genkit,
		ai.WithModelName(modelName),
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

// executePlanCommand handles the "plan <goal>" command by generating and executing a multi-step plan.
func executePlanCommand(ctx context.Context, cfg Config, s *meb.MEBStore, goal string, projectContext *ProjectSummary, plannerPrompt *prompts.Prompt) error {
	fmt.Println("\n🧠 Analyzing codebase and generating execution plan...")

	data := map[string]interface{}{
		"Query":      goal,
		"Packages":   projectContext.Packages,
		"Predicates": projectContext.Predicates,
		"TopSymbols": projectContext.TopSymbols,
	}

	promptStr, err := plannerPrompt.Execute(data)
	if err != nil {
		return fmt.Errorf("failed to execute planner template: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	modelName := cfg.Model
	if plannerPrompt.Config.Model != "" {
		modelName = plannerPrompt.Config.Model
	}

	resp, err := genkit.Generate(ctx, cfg.Genkit,
		ai.WithModelName(modelName),
		ai.WithPrompt(promptStr),
	)
	if err != nil {
		return fmt.Errorf("LLM API error: %w", err)
	}

	planJSON := resp.Text()
	if planJSON == "" {
		return fmt.Errorf("empty response from LLM")
	}

	steps, err := parseJSONPlan(planJSON)
	if err != nil {
		return fmt.Errorf("failed to parse plan: %w", err)
	}

	session := NewExecutionSession(goal, steps)

	DisplayPlan(session)

	confirmed, err := ConfirmExecution()
	if err != nil {
		return fmt.Errorf("failed to get user confirmation: %w", err)
	}

	if !confirmed {
		fmt.Println("\n❌ Plan execution cancelled by user.")
		return nil
	}

	return ExecutePlan(ctx, cfg, s, session, plannerPrompt)
}
