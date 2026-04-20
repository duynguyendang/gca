package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ingest"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	externmeb "github.com/duynguyendang/meb"
)

// TemplateStore manages query templates in the Analytical Store.
// It parses .dl policy files and stores their templates and metadata as triples.
type TemplateStore struct {
	storeManager *manager.StoreManager
}

// QueryTemplate is an alias for ingest.TemplateStoreQuery for API compatibility.
type QueryTemplate = ingest.TemplateStoreQuery

// NewTemplateStore creates a new TemplateStore.
func NewTemplateStore(storeManager *manager.StoreManager) *TemplateStore {
	return &TemplateStore{
		storeManager: storeManager,
	}
}

// LoadPolicyFiles parses all .dl files in the given directory
// and stores the templates as triples in the Analytical Store.
func (ts *TemplateStore) LoadPolicyFiles(ctx context.Context, dir string) error {
	// Get Analytical Store
	store, err := ts.storeManager.GetAnalyticalStore("")
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	// Sort for deterministic loading
	sortEntries(entries)

	var templatesLoaded int

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dl") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", entry.Name(), err)
		}

		templates, err := ts.parseTemplateFile(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", entry.Name(), err)
		}

		for _, tmpl := range templates {
			if err := ts.storeTemplate(ctx, store, tmpl); err != nil {
				return fmt.Errorf("failed to store template %s: %w", tmpl.ID, err)
			}
			templatesLoaded++
		}
	}

	return nil
}

// parseTemplateFile extracts query templates from .dl file content.
func (ts *TemplateStore) parseTemplateFile(content string) ([]*QueryTemplate, error) {
	var templates []*QueryTemplate

	// Extract query_metadata entries
	// Format: query_metadata("name", "key", "value").
	metaPattern := regexp.MustCompile(`query_metadata\s*\(\s*"([^"]+)"\s*,\s*"([^"]+)"\s*,\s*"([^"]*)"\s*\)\s*\.`)
	metaMatches := metaPattern.FindAllStringSubmatch(content, -1)

	// Group metadata by query name
	metaMap := make(map[string]map[string]string)
	for _, match := range metaMatches {
		name := match[1]
		key := match[2]
		value := match[3]
		if _, ok := metaMap[name]; !ok {
			metaMap[name] = make(map[string]string)
		}
		metaMap[name][key] = value
	}

	// Extract query templates
	// Format: query("name", A, B) :- triples(...).
	queryPattern := regexp.MustCompile(`query\s*\(\s*"([^"]+)"\s*,([^:]+)\)\s*:-`)
	queryMatches := queryPattern.FindAllStringSubmatch(content, -1)

	for _, match := range queryMatches {
		name := match[1]
		vars := strings.TrimSpace(match[2])

		meta, ok := metaMap[name]
		if !ok {
			meta = make(map[string]string)
		}

		// Extract template body by finding the rule body after :-
		rulePattern := regexp.MustCompile(fmt.Sprintf(`query\s*\(\s*"%s"\s*,%s\)\s*:-(.+)`, regexp.QuoteMeta(name), regexp.QuoteMeta(vars)))
		ruleMatch := rulePattern.FindStringSubmatch(content)
		var body string
		if len(ruleMatch) > 1 {
			body = strings.TrimSpace(ruleMatch[1])
			// Clean up the body - remove trailing period if present
			body = strings.TrimSuffix(body, ".")
		}

		tmpl := &QueryTemplate{
			ID:          name,
			Body:        body,
			Category:    meta["category"],
			Severity:    meta["severity"],
			Description: meta["description"],
			Parameters:  extractParameters(vars),
		}

		templates = append(templates, tmpl)
	}

	return templates, nil
}

// extractParameters extracts parameter names from query variable list.
func extractParameters(vars string) []ingest.TemplateParam {
	var params []ingest.TemplateParam
	// Vars are comma-separated, e.g., "A, B, File"
	parts := strings.Split(vars, ",")
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			params = append(params, ingest.TemplateParam{
				Name: name,
				Type: "string",
			})
		}
	}
	return params
}

// storeTemplate stores a query template as triples in the Analytical Store.
func (ts *TemplateStore) storeTemplate(ctx context.Context, store *externmeb.MEBStore, tmpl *QueryTemplate) error {
	// Store template body
	bodyFact := externmeb.Fact{
		Subject:   tmpl.ID,
		Predicate: "query_template",
		Object:    tmpl.Body,
	}
	if err := store.AddFact(bodyFact); err != nil {
		return fmt.Errorf("failed to add template body: %w", err)
	}

	// Store category
	if tmpl.Category != "" {
		catFact := externmeb.Fact{
			Subject:   tmpl.ID,
			Predicate: "category",
			Object:    tmpl.Category,
		}
		if err := store.AddFact(catFact); err != nil {
			return fmt.Errorf("failed to add category: %w", err)
		}
	}

	// Store severity
	if tmpl.Severity != "" {
		sevFact := externmeb.Fact{
			Subject:   tmpl.ID,
			Predicate: "severity",
			Object:    tmpl.Severity,
		}
		if err := store.AddFact(sevFact); err != nil {
			return fmt.Errorf("failed to add severity: %w", err)
		}
	}

	// Store description
	if tmpl.Description != "" {
		descFact := externmeb.Fact{
			Subject:   tmpl.ID,
			Predicate: "description",
			Object:    tmpl.Description,
		}
		if err := store.AddFact(descFact); err != nil {
			return fmt.Errorf("failed to add description: %w", err)
		}
	}

	return nil
}

// GetTemplate returns a query template by ID from the Analytical Store.
func (ts *TemplateStore) GetTemplate(ctx context.Context, projectID, templateID string) (*QueryTemplate, error) {
	store, err := ts.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return nil, err
	}

	// Query for template body
	bodyQuery := fmt.Sprintf(`triples("%s", "query_template", Body)`, templateID)
	results, err := queryStore(ctx, store, bodyQuery)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}

	tmpl := &QueryTemplate{
		ID:   templateID,
		Body: results[0]["Body"],
	}

	// Query for category
	catQuery := fmt.Sprintf(`triples("%s", "category", Category)`, templateID)
	if catResults, err := queryStore(ctx, store, catQuery); err == nil && len(catResults) > 0 {
		tmpl.Category = catResults[0]["Category"]
	}

	// Query for severity
	sevQuery := fmt.Sprintf(`triples("%s", "severity", Severity)`, templateID)
	if sevResults, err := queryStore(ctx, store, sevQuery); err == nil && len(sevResults) > 0 {
		tmpl.Severity = sevResults[0]["Severity"]
	}

	// Query for description
	descQuery := fmt.Sprintf(`triples("%s", "description", Desc)`, templateID)
	if descResults, err := queryStore(ctx, store, descQuery); err == nil && len(descResults) > 0 {
		tmpl.Description = descResults[0]["Desc"]
	}

	return tmpl, nil
}

// ListTemplates returns all templates matching a category.
func (ts *TemplateStore) ListTemplates(ctx context.Context, projectID, category string) ([]*QueryTemplate, error) {
	store, err := ts.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return nil, err
	}

	var query string
	if category != "" {
		query = fmt.Sprintf(`triples(ID, "category", "%s"), triples(ID, "query_template", Body)`, category)
	} else {
		query = `triples(ID, "query_template", Body)`
	}

	results, err := queryStore(ctx, store, query)
	if err != nil {
		return nil, err
	}

	var templates []*QueryTemplate
	seen := make(map[string]bool)

	for _, r := range results {
		id := r["ID"]
		if seen[id] {
			continue
		}
		seen[id] = true

		tmpl := &QueryTemplate{
			ID:   id,
			Body: r["Body"],
		}

		// Get additional metadata
		catQuery := fmt.Sprintf(`triples("%s", "category", Cat)`, id)
		if catResults, err := queryStore(ctx, store, catQuery); err == nil && len(catResults) > 0 {
			tmpl.Category = catResults[0]["Cat"]
		}
		sevQuery := fmt.Sprintf(`triples("%s", "severity", Sev)`, id)
		if sevResults, err := queryStore(ctx, store, sevQuery); err == nil && len(sevResults) > 0 {
			tmpl.Severity = sevResults[0]["Sev"]
		}
		descQuery := fmt.Sprintf(`triples("%s", "description", Desc)`, id)
		if descResults, err := queryStore(ctx, store, descQuery); err == nil && len(descResults) > 0 {
			tmpl.Description = descResults[0]["Desc"]
		}

		templates = append(templates, tmpl)
	}

	return templates, nil
}

// Parameterize takes a template ID and parameter map, returns a concrete Datalog query.
func (ts *TemplateStore) Parameterize(templateID string, params map[string]string) (string, error) {
	tmpl, err := ts.GetTemplate(context.Background(), "", templateID)
	if err != nil {
		return "", err
	}

	query := tmpl.Body
	for name, value := range params {
		placeholder := fmt.Sprintf("{{%s}}", name)
		query = strings.ReplaceAll(query, placeholder, value)
	}

	return query, nil
}

// queryStore executes a query against the MEB store.
func queryStore(ctx context.Context, store *externmeb.MEBStore, q string) ([]map[string]string, error) {
	// Import the meb package query function
	results, err := mebpkg.Query(ctx, store, q)
	if err != nil {
		return nil, err
	}

	// Convert []map[string]any to []map[string]string
	strResults := make([]map[string]string, len(results))
	for i, r := range results {
		strRow := make(map[string]string)
		for k, v := range r {
			if v != nil {
				strRow[k] = fmt.Sprintf("%v", v)
			}
		}
		strResults[i] = strRow
	}

	return strResults, nil
}
