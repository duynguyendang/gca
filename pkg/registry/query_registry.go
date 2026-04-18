package registry

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/duynguyendang/manglekit/core"
)

// QueryDefinition represents a pre-defined query loaded from GenePool
type QueryDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Category    string           `json:"category"`
	Tier        int              `json:"tier"`
	Template    string           `json:"template"`
	Parameters  []QueryParameter `json:"parameters"`
	Examples    []QueryExample   `json:"examples"`
}

// QueryParameter defines a parameter for a query
type QueryParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "file", "symbol", "string", "int"
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

// QueryExample provides usage examples
type QueryExample struct {
	Description string         `json:"description"`
	Input       map[string]any `json:"input"`
	Output      string         `json:"output"` // Expected Datalog output
}

// QueryRegistry manages pre-defined queries from GenePool
type QueryRegistry struct {
	mu         sync.RWMutex
	engine     core.Evaluator
	queries    map[string]*QueryDefinition
	categories map[string][]string
}

// NewQueryRegistry creates a new query registry
func NewQueryRegistry(engine core.Evaluator) *QueryRegistry {
	return &QueryRegistry{
		engine:     engine,
		queries:    make(map[string]*QueryDefinition),
		categories: make(map[string][]string),
	}
}

// LoadQueriesFromGenePool loads query definitions from Datalog policy files or directory
func (r *QueryRegistry) LoadQueriesFromGenePool(ctx context.Context, policyPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, err := os.Stat(policyPath)
	if err != nil {
		return fmt.Errorf("failed to stat policy path: %w", err)
	}

	if info.IsDir() {
		// Load all .dl files from directory at init time
		entries, err := os.ReadDir(policyPath)
		if err != nil {
			return fmt.Errorf("failed to read policy directory: %w", err)
		}

		// Sort for deterministic loading order
		sortEntries(entries)

		loadedFiles := []string{}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dl") {
				continue
			}
			filePath := filepath.Join(policyPath, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", entry.Name(), err)
			}
			if err := r.engine.LoadPolicy(ctx, string(content)); err != nil {
				return fmt.Errorf("failed to load %s: %w", entry.Name(), err)
			}
			loadedFiles = append(loadedFiles, entry.Name())
		}

		// Also load subdirectories recursively
		for _, entry := range entries {
			if entry.IsDir() {
				subPath := filepath.Join(policyPath, entry.Name())
				if err := r.loadDlsFromDir(ctx, subPath, &loadedFiles); err != nil {
					return err
				}
			}
		}

		log.Printf("Loaded %d policy files from %s", len(loadedFiles), policyPath)
	} else {
		// Single file mode (backward compatible)
		content, err := os.ReadFile(policyPath)
		if err != nil {
			return fmt.Errorf("failed to read policy file: %w", err)
		}
		if err := r.engine.LoadPolicy(ctx, string(content)); err != nil {
			return fmt.Errorf("failed to load query policies: %w", err)
		}
	}

	// Extract query metadata and build definitions
	// Query format: query_metadata("name", "description")
	//            query_metadata("name", "category", "value")
	//            query_metadata("name", "tier", "value")

	// First, get all query names and descriptions
	descQuery := `query_metadata(Name, Description)`
	descResults, err := r.engine.Query(ctx, []string{}, descQuery)
	if err != nil {
		return fmt.Errorf("failed to query descriptions: %w", err)
	}

	// Build a map to collect metadata for each query
	type QueryMeta struct {
		Name        string
		Description string
		Category    string
		Tier        int
		Template    string
	}
	metaMap := make(map[string]QueryMeta)

	for _, result := range descResults {
		name := result["Name"]
		metaMap[name] = QueryMeta{
			Name:        name,
			Description: result["Description"],
		}
	}

	// Get categories
	catQuery := `query_metadata(Name, "category", Category)`
	catResults, err := r.engine.Query(ctx, []string{}, catQuery)
	if err == nil {
		for _, result := range catResults {
			name := result["Name"]
			if meta, ok := metaMap[name]; ok {
				meta.Category = result["Category"]
				metaMap[name] = meta
			}
		}
	}

	// Get tiers
	tierQuery := `query_metadata(Name, "tier", Tier)`
	tierResults, err := r.engine.Query(ctx, []string{}, tierQuery)
	if err == nil {
		for _, result := range tierResults {
			name := result["Name"]
			if meta, ok := metaMap[name]; ok {
				if tierInt, err := strconv.Atoi(result["Tier"]); err == nil {
					meta.Tier = tierInt
				}
				metaMap[name] = meta
			}
		}
	}

	// Get templates from metadata
	templateQuery := `query_metadata(Name, "template", Template)`
	templateResults, err := r.engine.Query(ctx, []string{}, templateQuery)
	if err == nil {
		for _, result := range templateResults {
			name := result["Name"]
			if meta, ok := metaMap[name]; ok {
				meta.Template = result["Template"]
				metaMap[name] = meta
			}
		}
	}

	// Build query definitions from collected metadata
	for _, meta := range metaMap {
		// Build template by querying the query structure
		template, err := r.buildQueryTemplate(ctx, meta.Name)
		if err != nil {
			return fmt.Errorf("failed to build template for %s: %w", meta.Name, err)
		}

		def := &QueryDefinition{
			Name:        meta.Name,
			Description: meta.Description,
			Category:    meta.Category,
			Tier:        meta.Tier,
			Template:    template,
			Parameters:  r.extractParameters(template),
			Examples:    r.extractExamples(meta.Name),
		}

		r.queries[meta.Name] = def
		if meta.Category != "" {
			r.categories[meta.Category] = append(r.categories[meta.Category], meta.Name)
		}
	}

	return nil
}

// loadDlsFromDir recursively loads all .dl files from a directory
func (r *QueryRegistry) loadDlsFromDir(ctx context.Context, dirPath string, loadedFiles *[]string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	// Sort for deterministic loading order
	sortEntries(entries)

	for _, entry := range entries {
		if entry.IsDir() {
			subPath := filepath.Join(dirPath, entry.Name())
			if err := r.loadDlsFromDir(ctx, subPath, loadedFiles); err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".dl") {
			filePath := filepath.Join(dirPath, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", entry.Name(), err)
			}
			if err := r.engine.LoadPolicy(ctx, string(content)); err != nil {
				return fmt.Errorf("failed to load %s: %w", entry.Name(), err)
			}
			*loadedFiles = append(*loadedFiles, entry.Name())
		}
	}
	return nil
}

// sortEntries sorts directory entries by name for deterministic loading
func sortEntries(entries []os.DirEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
}

// buildQueryTemplate builds the Datalog template for a query by looking up metadata
func (r *QueryRegistry) buildQueryTemplate(ctx context.Context, queryName string) (string, error) {
	// If engine is nil (e.g., in tests), return error
	if r.engine == nil {
		return "", fmt.Errorf("engine not initialized")
	}
	// Look up template from metadata instead of switch-case
	templateQuery := fmt.Sprintf(`query_metadata("%s", "template", T)`, queryName)
	results, err := r.engine.Query(ctx, []string{}, templateQuery)
	if err == nil && len(results) > 0 {
		if template, ok := results[0]["T"]; ok && template != "" {
			return template, nil
		}
	}
	return fmt.Sprintf("%% Query: %s - Template not yet implemented", queryName), nil
}

// GetQuery retrieves a query definition by name
func (r *QueryRegistry) GetQuery(name string) (*QueryDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.queries[name]
	if !ok {
		return nil, fmt.Errorf("query '%s' not found", name)
	}

	return def, nil
}

// ListQueries returns all available queries
func (r *QueryRegistry) ListQueries() []*QueryDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queries := make([]*QueryDefinition, 0, len(r.queries))
	for _, def := range r.queries {
		queries = append(queries, def)
	}
	return queries
}

// ListQueriesByCategory returns queries filtered by category
func (r *QueryRegistry) ListQueriesByCategory(category string) []*QueryDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queries := make([]*QueryDefinition, 0)
	for _, name := range r.categories[category] {
		if def, ok := r.queries[name]; ok {
			queries = append(queries, def)
		}
	}
	return queries
}

// ExecuteQuery executes a pre-defined query with parameters
func (r *QueryRegistry) ExecuteQuery(ctx context.Context, name string, params map[string]any) (string, error) {
	def, err := r.GetQuery(name)
	if err != nil {
		return "", err
	}

	// Validate required parameters
	for _, param := range def.Parameters {
		if param.Required {
			if _, ok := params[param.Name]; !ok {
				return "", fmt.Errorf("missing required parameter: %s", param.Name)
			}
		}
	}

	// Build the Datalog query from template with secure substitution
	query := r.buildQueryFromTemplateSecure(def.Template, params)

	// Check policy before execution
	envelope := core.NewEnvelope(map[string]any{
		"query_name": name,
		"query":      query,
		"params":     params,
	})
	envelope.SetMeta("action", "execute_predefined_query")
	envelope.SetMeta("action_type", "query_execution")

	decision, err := r.engine.AssessPlan(ctx, envelope)
	if err != nil {
		return "", err
	}

	if decision.Outcome != core.OutcomeProceed {
		reason := "access denied"
		if len(decision.Reasons) > 0 {
			reason = decision.Reasons[0]
		}
		return "", fmt.Errorf("query execution denied: %s", reason)
	}

	return query, nil
}

// buildQueryFromTemplate substitutes parameters into the query template
func (r *QueryRegistry) buildQueryFromTemplate(template string, params map[string]any) string {
	query := template
	for key, value := range params {
		placeholder := fmt.Sprintf("{%s}", key)
		replacement := fmt.Sprintf("%v", value)
		query = strings.ReplaceAll(query, placeholder, replacement)
	}
	return query
}

// sanitizeDatalogValue prevents Datalog injection by escaping special characters
func sanitizeDatalogValue(input string) string {
	// Escape backslash first, then quotes
	escaped := strings.ReplaceAll(input, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return escaped
}

// buildQueryFromTemplateSecure substitutes parameters with sanitization
func (r *QueryRegistry) buildQueryFromTemplateSecure(template string, params map[string]any) string {
	query := template
	for key, value := range params {
		placeholder := fmt.Sprintf("{%s}", key)
		safeValue := sanitizeDatalogValue(fmt.Sprintf("%v", value))
		query = strings.ReplaceAll(query, placeholder, safeValue)
	}
	return query
}

// extractParameters extracts parameters from a query template using regex
func (r *QueryRegistry) extractParameters(template string) []QueryParameter {
	return r.extractParametersDynamic(template)
}

// extractParametersDynamic extracts {param} placeholders from template using regex
func (r *QueryRegistry) extractParametersDynamic(template string) []QueryParameter {
	re := regexp.MustCompile(`\{(\w+)\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	uniqueParams := make(map[string]bool)
	params := []QueryParameter{}

	for _, match := range matches {
		paramName := match[1]
		if !uniqueParams[paramName] {
			params = append(params, QueryParameter{
				Name:     paramName,
				Required: true,
				Type:     "string",
			})
			uniqueParams[paramName] = true
		}
	}
	return params
}

// extractExamples extracts usage examples for a query
func (r *QueryRegistry) extractExamples(queryName string) []QueryExample {
	// This would load examples from a separate file or database
	// For now, return empty examples
	return []QueryExample{}
}

// ValidateQuery checks if a query execution request complies with policies
func (r *QueryRegistry) ValidateQuery(ctx context.Context, queryName string, params map[string]any) error {
	// Check if query exists
	def, err := r.GetQuery(queryName)
	if err != nil {
		return err
	}

	// Check parameter requirements
	for _, param := range def.Parameters {
		if param.Required {
			if _, ok := params[param.Name]; !ok {
				return fmt.Errorf("missing required parameter '%s' for query '%s'", param.Name, queryName)
			}
		}
	}

	// Check tier-based access control
	// (This would integrate with session management)
	return nil
}

// IsFactSticky checks if a fact should be stored in the Global (Attention Sink) partition
// by evaluating the is_sticky Datalog rule via the policy engine AND name-based pattern detection.
// Returns true if the fact is "sticky" (important enough for permanent storage).
func (r *QueryRegistry) IsFactSticky(ctx context.Context, pred, subj string) bool {
	// Check Datalog rules first
	if r.engine != nil {
		query := fmt.Sprintf("is_sticky('%s', '%s')", pred, subj)
		results, err := r.engine.Query(ctx, []string{}, query)
		if err == nil && len(results) > 0 {
			return true
		}
	}

	// Fall back to name-based pattern detection for defines predicates
	if pred == "defines" {
		return isAttentionWorthyName(subj)
	}

	return false
}

// isAttentionWorthyName returns true if the symbol name matches attention-worthy patterns.
// These are name-based heuristics for detecting important code elements.
func isAttentionWorthyName(name string) bool {
	lower := strings.ToLower(name)

	// Entry point patterns
	if strings.Contains(lower, "main") || strings.Contains(lower, "init") || strings.Contains(lower, "start") {
		return true
	}

	// Event/Callback patterns
	if strings.Contains(lower, "handler") || strings.HasPrefix(lower, "on_") || strings.HasSuffix(lower, "handler") || strings.HasSuffix(lower, "callback") {
		return true
	}

	// Lifecycle patterns
	if strings.Contains(lower, "setup") || strings.Contains(lower, "teardown") || strings.Contains(lower, "bootstrap") || strings.Contains(lower, "cleanup") {
		return true
	}

	// Test patterns
	if strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "Test") || strings.HasPrefix(name, "Test") || strings.HasSuffix(name, "_bench") || strings.HasSuffix(name, "Benchmark") {
		return true
	}

	// Security/Auth patterns
	if strings.Contains(lower, "auth") || strings.Contains(lower, "login") || strings.Contains(lower, "logout") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") || strings.Contains(lower, "token") || strings.Contains(lower, "session") || strings.Contains(lower, "sanitize") || strings.Contains(lower, "authorize") {
		return true
	}

	// Validation patterns
	if strings.Contains(lower, "validate") || strings.Contains(lower, "verify") || strings.Contains(lower, "check") {
		return true
	}

	// CRUD patterns (prefix-based)
	if strings.HasPrefix(name, "Create") || strings.HasPrefix(name, "Delete") || strings.HasPrefix(name, "Update") || strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List") || strings.HasPrefix(name, "Put") {
		return true
	}

	// Error/Recovery patterns
	if strings.Contains(lower, "error") || strings.Contains(lower, "retry") || strings.Contains(lower, "fallback") || strings.Contains(lower, "recovery") || strings.Contains(lower, "recover") {
		return true
	}

	return false
}
