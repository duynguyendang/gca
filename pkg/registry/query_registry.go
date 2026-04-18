package registry

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

// buildQueryTemplate builds the Datalog template for a query by examining its structure
func (r *QueryRegistry) buildQueryTemplate(ctx context.Context, queryName string) (string, error) {
	// For now, return a simple template based on the query name
	// The actual Datalog would be extracted from the policy file in a full implementation
	// This template will be used for parameter substitution
	switch queryName {
	case "find_defines":
		return `triples({FileID}, "defines", Symbol)`, nil
	case "find_imports":
		return `triples({FileID}, "imports", Target)`, nil
	case "find_outbound_calls":
		return `triples({FileID}, "defines", Symbol), triples(Symbol, "calls", Target)`, nil
	case "find_inbound_calls":
		return `triples(Caller, "calls", Symbol), triples({FileID}, "defines", Symbol)`, nil
	case "smell_circular_direct":
		return `triples(A, "calls", B), triples(B, "calls", A), A != B`, nil
	case "smell_imports":
		return `triples(File, "imports", Pkg)`, nil
	case "smell_defines":
		return `triples(File, "defines", Symbol)`, nil
	case "smell_hub":
		return `triples(File, "calls", _), triples(Caller, "calls", File), File != Caller`, nil
	case "smell_layer_violation":
		return `triples(File, "imports", Target), triples(File, "has_tag", LayerTag), triples(Target, "has_tag", "backend"), LayerTag != "backend"`, nil
	default:
		return fmt.Sprintf(`%% Query: %s - Template not yet implemented`, queryName), nil
	}
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

	// Build the Datalog query from template
	query := r.buildQueryFromTemplate(def.Template, params)

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

// extractParameters extracts parameters from a query template
func (r *QueryRegistry) extractParameters(template string) []QueryParameter {
	// Parse template for {parameter} placeholders
	// This is a simplified implementation
	params := []QueryParameter{}

	// Look for common patterns
	if strings.Contains(template, "{FileID}") {
		params = append(params, QueryParameter{
			Name:     "FileID",
			Type:     "file",
			Required: true,
		})
	}
	if strings.Contains(template, "{Symbol}") {
		params = append(params, QueryParameter{
			Name:     "Symbol",
			Type:     "symbol",
			Required: true,
		})
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
