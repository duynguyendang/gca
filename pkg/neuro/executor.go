package neuro

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/registry"
	"github.com/duynguyendang/gca/pkg/service/ai"
)

// IntentToTemplate maps intents to relevant query templates.
var IntentToTemplate = map[ai.Intent][]string{
	ai.IntentFind: {
		"smell_circular_direct",
		"smell_hub_anomaly",
		"smell_layer_violation",
	},
	ai.IntentWhoCalls:   {"query_who_calls"},
	ai.IntentWhatCalls:  {"query_what_calls"},
	ai.IntentHowReaches: {"query_reachability"},
	ai.IntentSecurity: {
		"security_agent",
		"secrets_scan",
	},
	ai.IntentRefactor: {
		"smell_circular_direct",
		"smell_hub_anomaly",
		"smell_god_file",
	},
	ai.IntentPerformance: {
		"performance_hotspot",
	},
}

// NeuroSymbolicExecutor orchestrates the OODA loop using diagnostic context
// and template store for grounded AI responses.
type NeuroSymbolicExecutor struct {
	storeManager   *manager.StoreManager
	templateStore  *registry.TemplateStore
	contextBuilder *ContextBuilder
}

// NewNeuroSymbolicExecutor creates a new NeuroSymbolicExecutor.
func NewNeuroSymbolicExecutor(storeManager *manager.StoreManager, templateStore *registry.TemplateStore) *NeuroSymbolicExecutor {
	return &NeuroSymbolicExecutor{
		storeManager:   storeManager,
		templateStore:  templateStore,
		contextBuilder: NewContextBuilder(storeManager),
	}
}

// ExecuteResult represents the result of a neuro-symbolic execution.
type ExecuteResult struct {
	DiagnosticContext string              // The diagnostic context used
	Intent            ai.Intent           // Classified intent
	TemplateID        string              // Selected template ID
	Query             string              // Parameterized query
	Results           []map[string]string // Query execution results
	Answer            string              // Final synthesized answer
}

// Execute implements the Neuro-Symbolic pattern:
// 1. Build diagnostic context from Analytical Store (O(1) read)
// 2. Classify user intent
// 3. Select and parameterize template
// 4. Execute query against Source Store
// 5. Return results for LLM synthesis
func (nse *NeuroSymbolicExecutor) Execute(ctx context.Context, projectID, userQuery string) (*ExecuteResult, error) {
	result := &ExecuteResult{}

	// Step 1: Build diagnostic context from Analytical Store
	diagnosticCtx, err := nse.contextBuilder.BuildDiagnosticContext(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to build diagnostic context: %w", err)
	}
	result.DiagnosticContext = diagnosticCtx

	// Step 2: Classify intent
	intentResult := ai.ClassifyIntent(userQuery)
	result.Intent = intentResult.Intent

	// Step 3: Select template based on intent
	templateID, err := nse.selectTemplate(intentResult.Intent, userQuery)
	if err != nil {
		// If no template found, fall back to general query
		templateID = ""
	}
	result.TemplateID = templateID

	// Step 4: Extract parameters and parameterize query
	if templateID != "" {
		params := nse.extractParams(userQuery, intentResult)
		paramQuery, err := nse.templateStore.Parameterize(templateID, params)
		if err != nil {
			return nil, fmt.Errorf("failed to parameterize template: %w", err)
		}
		result.Query = paramQuery
	} else {
		// Fall back to a general query based on the user's query
		result.Query = nse.buildFallbackQuery(userQuery)
	}

	// Step 5: Execute query against Source Store
	sourceStore, err := nse.storeManager.GetSourceStore(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source store: %w", err)
	}

	queryResults, err := mebpkg.Query(ctx, sourceStore, result.Query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	// Convert results to string map
	result.Results = make([]map[string]string, len(queryResults))
	for i, r := range queryResults {
		strRow := make(map[string]string)
		for k, v := range r {
			if v != nil {
				strRow[k] = fmt.Sprintf("%v", v)
			}
		}
		result.Results[i] = strRow
	}

	return result, nil
}

// selectTemplate selects the most appropriate template for the given intent.
func (nse *NeuroSymbolicExecutor) selectTemplate(intent ai.Intent, query string) (string, error) {
	templateIDs, ok := IntentToTemplate[intent]
	if !ok || len(templateIDs) == 0 {
		return "", fmt.Errorf("no template found for intent: %s", intent)
	}

	// Try to find templates that match the query keywords
	queryLower := strings.ToLower(query)
	for _, templateID := range templateIDs {
		if strings.Contains(queryLower, strings.ReplaceAll(templateID, "_", " ")) {
			return templateID, nil
		}
	}

	// Return the first template as default
	return templateIDs[0], nil
}

// extractParams extracts parameters from the user query and intent result.
func (nse *NeuroSymbolicExecutor) extractParams(query string, intentResult ai.IntentResult) map[string]string {
	params := make(map[string]string)

	// Extract target from intent result
	if intentResult.Target != "" {
		params["Target"] = intentResult.Target
	}

	// Extract file path if mentioned
	filePatterns := []string{
		`in\s+([\w./]+)`,
		`file\s+([\w./]+)`,
		`module\s+([\w./]+)`,
		`package\s+([\w./]+)`,
	}
	for _, pattern := range filePatterns {
		if match := findSubstringMatch(query, pattern); len(match) > 1 && match[1] != "" {
			params["File"] = match[1]
			break
		}
	}

	// Set default values for common parameters
	if _, ok := params["File"]; !ok {
		params["File"] = "_"
	}
	if _, ok := params["Depth"]; !ok {
		params["Depth"] = "3"
	}

	return params
}

// buildFallbackQuery builds a simple query based on keywords in the user query.
func (nse *NeuroSymbolicExecutor) buildFallbackQuery(query string) string {
	queryLower := strings.ToLower(query)

	if strings.Contains(queryLower, "circular") || strings.Contains(queryLower, "cycle") {
		return `triples(A, "calls", B), triples(B, "calls", A), A != B`
	}
	if strings.Contains(queryLower, "hub") || strings.Contains(queryLower, "god") {
		return `triples(File, "calls", _), not contains(File, ":")`
	}
	if strings.Contains(queryLower, "import") || strings.Contains(queryLower, "dependency") {
		return `triples(File, "imports", Target)`
	}
	if strings.Contains(queryLower, "define") || strings.Contains(queryLower, "function") {
		return `triples(File, "defines", Symbol)`
	}

	// Default: return all calls
	return `triples(A, "calls", B)`
}

// findSubstringMatch finds a substring match using a simple pattern.
func findSubstringMatch(s, pattern string) []string {
	idx := strings.Index(s, pattern)
	if idx == -1 {
		return nil
	}
	start := idx
	end := idx + len(pattern)
	return []string{s[start:end], s[start:end]}
}

// EnrichPromptWithContext enriches a prompt with the diagnostic context.
func (nse *NeuroSymbolicExecutor) EnrichPromptWithContext(ctx context.Context, projectID, userQuery string) (string, error) {
	diagnosticCtx, err := nse.contextBuilder.BuildDiagnosticContext(ctx, projectID)
	if err != nil {
		// Return original prompt if context building fails
		return userQuery, nil
	}

	// Build enriched prompt
	var sb strings.Builder
	sb.WriteString(diagnosticCtx)
	sb.WriteString("\nUser Query: ")
	sb.WriteString(userQuery)
	sb.WriteString("\n")

	return sb.String(), nil
}
