package registry

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/ingest"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	externmeb "github.com/duynguyendang/meb"
)

// TemplateStore manages query templates in the Analytical Store.
// It parses .mg policy files and stores their templates and metadata as triples.
type TemplateStore struct {
	storeManager *manager.StoreManager
	// in-memory cache so LoadPolicyFiles doesn't need to open BadgerDB
	templates map[string]*QueryTemplate
}

// QueryTemplate is an alias for ingest.TemplateStoreQuery for API compatibility.
type QueryTemplate = ingest.TemplateStoreQuery

// NewTemplateStore creates a new TemplateStore.
func NewTemplateStore(storeManager *manager.StoreManager) *TemplateStore {
	return &TemplateStore{
		storeManager: storeManager,
		templates:    make(map[string]*QueryTemplate),
	}
}

// LoadPolicyFiles parses init.mg manifest and caches templates in memory.
// Does not open BadgerDB — avoids lock contention with analyzer.
func (ts *TemplateStore) LoadPolicyFiles(ctx context.Context, initPath string) error {
	manifest, err := LoadManifest(initPath)
	if err != nil {
		return fmt.Errorf("failed to load init.mg: %w", err)
	}

	var templatesLoaded int
	for _, filePath := range manifest.FilePaths {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		templates, err := ts.parseTemplateFile(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", filePath, err)
		}

		for _, tmpl := range templates {
			ts.templates[tmpl.ID] = tmpl
		}
		templatesLoaded += len(templates)
	}

	log.Printf("Loaded %d templates from init.mg manifest", templatesLoaded)
	return nil
}

// parseTemplateFile extracts query templates from .mg file content.
func (ts *TemplateStore) parseTemplateFile(content string) ([]*QueryTemplate, error) {
	var templates []*QueryTemplate

	// Extract query_metadata entries
	// Format: query_metadata("name", "key", "value") or query_metadata("name", "key", `backtick_value`)
	quotePattern := `query_metadata\s*\(\s*"([^"]+)"\s*,\s*"([^"]+)"\s*,\s*"([^"]*)"\s*\)\s*\.`
	backtickPattern := `query_metadata\s*\(\s*"([^"]+)"\s*,\s*"([^"]+)"\s*,\s*` + "`" + `([^` + "`" + `]*)` + "`" + `\s*\)\s*\.`

	// Group metadata by query name
	metaMap := make(map[string]map[string]string)

	// Match quoted strings
	for _, match := range regexp.MustCompile(quotePattern).FindAllStringSubmatch(content, -1) {
		name := match[1]
		key := match[2]
		value := match[3]
		if _, ok := metaMap[name]; !ok {
			metaMap[name] = make(map[string]string)
		}
		metaMap[name][key] = value
	}

	// Match backtick strings
	for _, match := range regexp.MustCompile(backtickPattern).FindAllStringSubmatch(content, -1) {
		name := match[1]
		key := match[2]
		value := match[3]
		if _, ok := metaMap[name]; !ok {
			metaMap[name] = make(map[string]string)
		}
		metaMap[name][key] = value
	}

	// Extract templates from "template" metadata as fallback for queries without body
	for name, meta := range metaMap {
		if tmplStr, ok := meta["template"]; ok && tmplStr != "" {
			tmpl := &QueryTemplate{
				ID:          name,
				Body:        tmplStr,
				Predicate:   meta["Predicate"],
				SmellType:   meta["smell_type"],
				Category:    meta["category"],
				Severity:    meta["severity"],
				Description: meta["description"],
			}
			templates = append(templates, tmpl)
		}
	}

	// Extract query templates
	// Format: query("name", A, B) :- triples(...).
	queryPattern := regexp.MustCompile(`query\s*\(\s*"([^"]+)"\s*,([^:]+)\)\s*:-`)
	queryMatches := queryPattern.FindAllStringSubmatchIndex(content, -1)

	// Collect names that already have a template from metadata
	hasMetadataTemplate := make(map[string]bool)
	for _, t := range templates {
		hasMetadataTemplate[t.ID] = true
	}

	for _, loc := range queryMatches {
		name := content[loc[2]:loc[3]]
		vars := strings.TrimSpace(content[loc[4]:loc[5]])

		meta, ok := metaMap[name]
		if !ok {
			meta = make(map[string]string)
		}

		// Extract the rule body: the text after ":-" up to the terminating "."
		// (a period at end of line or EOF). Rule bodies commonly span multiple
		// lines, so a simple `:-:-(.+)` regex (which cannot cross newlines)
		// would silently drop them.
		body := findRuleBody(content[loc[1]:])

		// Skip if body is empty (means template metadata already provided the template)
		if body == "" {
			continue
		}

		// Skip if we already have a template for this name from metadata
		if hasMetadataTemplate[name] {
			continue
		}

		tmpl := &QueryTemplate{
			ID:          name,
			Body:        body,
			Predicate:   meta["Predicate"],
			SmellType:   meta["smell_type"],
			Category:    meta["category"],
			Severity:    meta["severity"],
			Description: meta["description"],
			Parameters:  extractParameters(vars),
		}

		templates = append(templates, tmpl)
	}

	return templates, nil
}

// findRuleBody extracts a rule body from the text following ":-" up to the
// terminating "." — the first period at end-of-line (or EOF). The body may span
// multiple lines; the final body line ends with ".".
func findRuleBody(s string) string {
	lines := strings.Split(s, "\n")
	var parts []string
	for _, line := range lines {
		parts = append(parts, line)
		if strings.HasSuffix(strings.TrimRight(line, " \t"), ".") {
			body := strings.Join(parts, "\n")
			return strings.TrimSpace(strings.TrimSuffix(body, "."))
		}
	}
	// No terminating period found: take the whole remainder.
	return strings.TrimSpace(strings.Join(parts, "\n"))
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

	// Store predicate
	if tmpl.Predicate != "" {
		predFact := externmeb.Fact{
			Subject:   tmpl.ID,
			Predicate: "Predicate",
			Object:    tmpl.Predicate,
		}
		if err := store.AddFact(predFact); err != nil {
			return fmt.Errorf("failed to add predicate: %w", err)
		}
	}

	// Store smell_type
	if tmpl.SmellType != "" {
		smellTypeFact := externmeb.Fact{
			Subject:   tmpl.ID,
			Predicate: "smell_type",
			Object:    tmpl.SmellType,
		}
		if err := store.AddFact(smellTypeFact); err != nil {
			return fmt.Errorf("failed to add smell_type: %w", err)
		}
	}

	// Store parameters as "name|type" triples
	for _, param := range tmpl.Parameters {
		paramFact := externmeb.Fact{
			Subject:   tmpl.ID,
			Predicate: "param",
			Object:    fmt.Sprintf("%s|%s", param.Name, param.Type),
		}
		if err := store.AddFact(paramFact); err != nil {
			return fmt.Errorf("failed to add param: %w", err)
		}
	}

	return nil
}

// GetTemplate returns a query template by ID.
// Uses in-memory cache if available, falls back to Analytical Store.
func (ts *TemplateStore) GetTemplate(ctx context.Context, projectID, templateID string) (*QueryTemplate, error) {
	if tmpl, ok := ts.templates[templateID]; ok {
		return tmpl, nil
	}

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

	// Query for parameters
	paramQuery := fmt.Sprintf(`triples("%s", "param", Param)`, templateID)
	if paramResults, err := queryStore(ctx, store, paramQuery); err == nil && len(paramResults) > 0 {
		for _, pr := range paramResults {
			if paramStr := pr["Param"]; paramStr != "" {
				parts := strings.SplitN(paramStr, "|", 2)
				if len(parts) == 2 {
					tmpl.Parameters = append(tmpl.Parameters, ingest.TemplateParam{
						Name: parts[0],
						Type: parts[1],
					})
				}
			}
		}
	}

	return tmpl, nil
}

// ListTemplates returns all templates matching a category.
// Uses in-memory cache if available, falls back to Analytical Store.
func (ts *TemplateStore) ListTemplates(ctx context.Context, projectID, category string) ([]*QueryTemplate, error) {
	if len(ts.templates) > 0 {
		var templates []*QueryTemplate
		for _, tmpl := range ts.templates {
			if category == "" || tmpl.Category == category {
				templates = append(templates, tmpl)
			}
		}
		return templates, nil
	}

	store, err := ts.storeManager.GetAnalyticalStore("")
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
	seen := make(map[string]int)

	for _, r := range results {
		id := r["ID"]

		// Keep entry with non-empty body if we already saw this id
		if existingIdx, ok := seen[id]; ok {
			if r["Body"] != "" && templates[existingIdx].Body == "" {
				// Update existing template with non-empty body
				templates[existingIdx].Body = r["Body"]
			}
			continue
		}
		seen[id] = len(templates)

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
		predQuery := fmt.Sprintf(`triples("%s", "Predicate", Pred)`, id)
		if predResults, err := queryStore(ctx, store, predQuery); err == nil && len(predResults) > 0 {
			tmpl.Predicate = predResults[0]["Pred"]
		}
		smellTypeQuery := fmt.Sprintf(`triples("%s", "smell_type", SmellType)`, id)
		if smellTypeResults, err := queryStore(ctx, store, smellTypeQuery); err == nil && len(smellTypeResults) > 0 {
			tmpl.SmellType = smellTypeResults[0]["SmellType"]
		}

		// Get parameters
		paramQuery := fmt.Sprintf(`triples("%s", "param", Param)`, id)
		if paramResults, err := queryStore(ctx, store, paramQuery); err == nil && len(paramResults) > 0 {
			for _, pr := range paramResults {
				if paramStr := pr["Param"]; paramStr != "" {
					parts := strings.SplitN(paramStr, "|", 2)
					if len(parts) == 2 {
						tmpl.Parameters = append(tmpl.Parameters, ingest.TemplateParam{
							Name: parts[0],
							Type: parts[1],
						})
					}
				}
			}
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
