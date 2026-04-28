package ingest

import (
	"context"
	"fmt"

	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

const (
	CurrentAnalyticsVersion   = "2.0"
	AnalyticsVersionPredicate = "analytics_version"
)

// Analyzer performs post-ingestion analysis on the Source Store
// and writes results (smells, centrality) to the Analytical Store.
type Analyzer struct {
	storeManager  StoreManagerInterface
	templateStore TemplateStoreInterface
}

// StoreManagerInterface defines the interface for store operations.
type StoreManagerInterface interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// TemplateStoreInterface defines the interface for template operations.
type TemplateStoreInterface interface {
	GetTemplate(ctx context.Context, projectID, templateID string) (*TemplateStoreQuery, error)
	ListTemplates(ctx context.Context, projectID, category string) ([]*TemplateStoreQuery, error)
}

// TemplateStoreQuery represents a query template from the TemplateStore.
type TemplateStoreQuery struct {
	ID          string
	Body        string
	Predicate   string
	Category    string
	Severity    string
	SmellType   string
	Description string
	Parameters  []TemplateParam
}

// TemplateParam represents a parameter in a query template.
type TemplateParam struct {
	Name        string
	Type        string
	Description string
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(sm StoreManagerInterface, ts TemplateStoreInterface) *Analyzer {
	return &Analyzer{
		storeManager:  sm,
		templateStore: ts,
	}
}

// RunStaticAnalysis executes the full static analysis pipeline:
// 1. Clear old analytical data
// 2. Compute centrality and entry points using Datalog rules
// 3. Execute templates from TemplateStore
func (a *Analyzer) RunStaticAnalysis(ctx context.Context, projectID string) error {
	logger.Info("Starting static analysis", "project", projectID)

	if err := a.clearAnalyticalData(ctx, projectID); err != nil {
		logger.Warn("Failed to clear old analytical data", "error", err)
	}

	if err := a.executeRulesFromTemplates(ctx, projectID); err != nil {
		logger.Warn("Template rule execution failed", "error", err)
	}

	logger.Info("Static analysis completed", "project", projectID)
	return nil
}

// executeRulesFromTemplates loads and executes all templates from TemplateStore.
// Each template defines: query body, result predicate, and metadata.
// This is the generic rule execution engine - works for any template type.
func (a *Analyzer) executeRulesFromTemplates(ctx context.Context, projectID string) error {
	if a.templateStore == nil {
		return fmt.Errorf("template store not available")
	}

	templates, err := a.templateStore.ListTemplates(ctx, projectID, "")
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	if len(templates) == 0 {
		logger.Info("No templates found in TemplateStore")
		return nil
	}

	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	totalResults := 0

	for _, tmpl := range templates {
		if tmpl.Body == "" {
			continue
		}

		results, err := mebpkg.Query(ctx, sourceStore, tmpl.Body)
		if err != nil {
			logger.Warn("Template query failed", "template", tmpl.ID, "error", err)
			continue
		}

		logger.Debug("Template returned results", "template", tmpl.ID, "count", len(results))

		for _, r := range results {
			if err := a.emitFactFromTemplate(analyticalStore, r, tmpl); err != nil {
				logger.Warn("Failed to emit fact for template", "template", tmpl.ID, "error", err)
			} else {
				totalResults++
			}
		}
	}

	logger.Info("Template rule execution complete", "facts", totalResults, "templates", len(templates))
	return nil
}

// emitFactFromTemplate emits a structured fact to the analytical store.
// The fact predicate is derived from template metadata (e.g., "has_smell_type", "is_entry_point").
// Metadata fields (category, severity, etc.) are stored as separate predicates.
func (a *Analyzer) emitFactFromTemplate(store *meb.MEBStore, result map[string]any, tmpl *TemplateStoreQuery) error {
	var subject string

	if s, ok := result["File"].(string); ok {
		subject = s
	} else if s, ok := result["A"].(string); ok {
		subject = s
	} else if s, ok := result["Subject"].(string); ok {
		subject = s
	}

	if subject == "" {
		return fmt.Errorf("no subject found in result")
	}

	predicate := tmpl.Predicate
	if predicate == "" {
		predicate = "has_" + tmpl.ID
	}

	smellType := tmpl.SmellType
	if smellType == "" {
		smellType = tmpl.ID
	}

	facts := []meb.Fact{
		{Subject: subject, Predicate: predicate, Object: smellType},
		{Subject: subject, Predicate: "has_category", Object: tmpl.Category},
		{Subject: subject, Predicate: "has_severity", Object: tmpl.Severity},
	}

	for _, fact := range facts {
		if err := store.AddFact(fact); err != nil {
			return err
		}
	}

	return nil
}

// extractString extracts a string value from a map.

// clearAnalyticalData removes old smells, hub scores, entry points, and centrality data
// from the analytical store to prevent stale data from previous ingests.
// Uses batch collection then single-pass deletion per subject to minimize DB operations.
func (a *Analyzer) clearAnalyticalData(ctx context.Context, projectID string) error {
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	predicatesToClear := []string{
		"has_smell",
		"has_hub_score",
		"is_entry_point",
		"has_centrality",
	}

	for _, pred := range predicatesToClear {
		logger.Info("Clearing facts", "predicate", pred, "project", projectID)

		subjectsToDelete := make(map[string]bool)
		for fact, err := range analyticalStore.ScanContext(ctx, "", pred, "") {
			if err != nil {
				logger.Warn("Error scanning facts", "predicate", pred, "error", err)
				continue
			}
			subjectsToDelete[fact.Subject] = true
		}

		logger.Info("Found subjects to clear", "count", len(subjectsToDelete), "predicate", pred)

		clearedCount := 0
		for subject := range subjectsToDelete {
			if err := analyticalStore.DeleteFactsBySubject(subject); err != nil {
				logger.Warn("Failed to delete facts for subject", "subject", subject, "error", err)
			} else {
				clearedCount++
			}
		}

		if clearedCount > 0 {
			logger.Info("Cleared facts", "count", clearedCount, "predicate", pred, "project", projectID)
		}
	}

	return nil
}

// executeTemplateQueries executes smell detection templates from the TemplateStore.
func (a *Analyzer) executeTemplateQueries(ctx context.Context, projectID string) error {
	if a.templateStore == nil {
		return nil
	}

	// Get all smell templates
	templates, err := a.templateStore.ListTemplates(ctx, projectID, "smell")
	if err != nil {
		return fmt.Errorf("failed to list smell templates: %w", err)
	}

	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	for _, tmpl := range templates {
		if tmpl.Body == "" {
			continue
		}

		// Execute template query against source store
		results, err := mebpkg.Query(ctx, sourceStore, tmpl.Body)
		if err != nil {
			logger.Warn("Template query failed", "template", tmpl.ID, "error", err)
			continue
		}

		// Write results to analytical store
		for _, r := range results {
			subject := extractString(r, "Subject")
			if subject == "" {
				continue
			}

			fact := meb.Fact{
				Subject:   subject,
				Predicate: "has_smell",
				Object:    tmpl.ID + ":" + tmpl.Category,
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				logger.Warn("Failed to add smell fact", "error", err)
			}
		}
	}

	return nil
}

// extractString extracts a string value from a map.
func extractString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getAnalyticsVersion retrieves the stored analytics version from the analytical store
func (a *Analyzer) getAnalyticsVersion(analyticalStore *meb.MEBStore) string {
	ctx := context.Background()
	for item, err := range analyticalStore.ScanContext(ctx, "", AnalyticsVersionPredicate, "") {
		if err != nil {
			continue
		}
		if objStr, ok := item.Object.(string); ok {
			return objStr
		}
	}
	return ""
}

// setAnalyticsVersion stores the analytics version in the analytical store
func (a *Analyzer) setAnalyticsVersion(analyticalStore *meb.MEBStore) error {
	fact := meb.Fact{
		Subject:   "analytics",
		Predicate: AnalyticsVersionPredicate,
		Object:    CurrentAnalyticsVersion,
	}
	return analyticalStore.AddFact(fact)
}

// RunPostIngestAnalysis runs all post-ingestion analysis for a project.
// This should be called after the main ingestion is complete.
// It runs asynchronously to avoid blocking the ingestion process.
// Uses idempotent fact addition - duplicate facts won't be added if they already exist.
// Checks analytics version to skip redundant computations.
// All analysis is template-driven via TemplateStore - no hardcoded rules.
func (a *Analyzer) RunPostIngestAnalysis(ctx context.Context, projectID string) error {
	logger.Info("Starting post-ingest analysis", "project", projectID)

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	storedVersion := a.getAnalyticsVersion(analyticalStore)
	if storedVersion == CurrentAnalyticsVersion {
		logger.Info("Analytics already up to date, skipping", "version", CurrentAnalyticsVersion)
		return nil
	}

	if err := a.clearAnalyticalData(ctx, projectID); err != nil {
		logger.Warn("Failed to clear old analytical data", "error", err)
	}

	if err := a.executeRulesFromTemplates(ctx, projectID); err != nil {
		logger.Warn("Template rule execution failed", "error", err)
	}

	if err := a.setAnalyticsVersion(analyticalStore); err != nil {
		logger.Warn("Failed to set analytics version", "error", err)
	}

	logger.Info("Post-ingest analysis complete", "project", projectID)
	return nil
}

// RunPostIngestAnalysisAsync runs analysis asynchronously.
func (a *Analyzer) RunPostIngestAnalysisAsync(ctx context.Context, projectID string) {
	go func() {
		if err := a.RunPostIngestAnalysis(ctx, projectID); err != nil {
			logger.Error("Async post-ingest analysis failed", "error", err)
		}
	}()
}

// computeCentrality calculates in/out degree for files and functions,
// then writes centrality facts to the Analytical Store.
func (a *Analyzer) computeCentrality(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	// Compute hub scores - files that are called by many others
	hubQuery := `triples(File, "calls", _), not contains(File, ":")`
	hubResults, err := mebpkg.Query(ctx, sourceStore, hubQuery)
	if err != nil {
		return fmt.Errorf("hub query failed: %w", err)
	}

	// Count callers per file
	callerCounts := make(map[string]int)
	for _, r := range hubResults {
		if file, ok := r["File"].(string); ok {
			callerCounts[file]++
		}
	}

	// Write hub score facts for files with multiple callers
	for file, count := range callerCounts {
		if count > 5 { // Threshold for hub classification
			fact := meb.Fact{
				Subject:   file,
				Predicate: "has_hub_score",
				Object:    fmt.Sprintf("%d", count),
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				logger.Warn("Failed to add hub score", "file", file, "error", err)
			}
		}
	}

	// Compute entry points - files that define main, init, or have many callees
	entryQuery := `triples(File, "defines", Symbol), or(contains(Symbol, "main"), contains(Symbol, "init"))`
	entryResults, err := mebpkg.Query(ctx, sourceStore, entryQuery)
	if err != nil {
		logger.Warn("Entry point query failed", "error", err)
	} else {
		for _, r := range entryResults {
			if file, ok := r["File"].(string); ok {
				fact := meb.Fact{
					Subject:   file,
					Predicate: "is_entry_point",
					Object:    "true",
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					logger.Warn("Failed to add entry point", "file", file, "error", err)
				}
			}
		}
	}

	// Compute centrality for symbols (functions/methods)
	symbolCentralityQuery := `triples(File, "defines", Symbol), triples(Symbol, "calls", Target)`
	symbolResults, err := mebpkg.Query(ctx, sourceStore, symbolCentralityQuery)
	if err != nil {
		logger.Warn("Symbol centrality query failed", "error", err)
	} else {
		// Count calls per symbol
		symbolCalls := make(map[string]int)
		for _, r := range symbolResults {
			if symbol, ok := r["Symbol"].(string); ok {
				symbolCalls[symbol]++
			}
		}

		for symbol, count := range symbolCalls {
			if count > 10 { // High connectivity threshold
				fact := meb.Fact{
					Subject:   symbol,
					Predicate: "has_centrality",
					Object:    fmt.Sprintf("%d", count),
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					logger.Warn("Failed to add centrality", "symbol", symbol, "error", err)
				}
			}
		}
	}

	logger.Info("Centrality analysis complete", "hub_files", len(callerCounts), "entry_points", len(entryResults))

	return nil
}

// detectSmells runs smell detection queries and writes results to Analytical Store.
// Smell templates are loaded from the TemplateStore (backed by policies/smells/*.dl).
// This makes smell detection fully Datalog-driven - adding new smells requires only .dl files.
func (a *Analyzer) detectSmells(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	if a.templateStore == nil {
		return fmt.Errorf("template store not available for smell detection")
	}

	templates, err := a.templateStore.ListTemplates(ctx, projectID, "smell")
	if err != nil {
		return fmt.Errorf("failed to list smell templates: %w", err)
	}

	smellResults := 0

	for _, tmpl := range templates {
		if tmpl.Body == "" {
			logger.Warn("Template has empty body", "template", tmpl.ID)
			continue
		}

		results, err := mebpkg.Query(ctx, sourceStore, tmpl.Body)
		if err != nil {
			logger.Warn("Smell query failed", "template", tmpl.ID, "error", err)
			continue
		}

		for _, r := range results {
			if err := a.emitStructuredFact(analyticalStore, r, tmpl.ID, tmpl.Category, tmpl.Severity); err != nil {
				logger.Warn("Failed to emit structured fact", "error", err)
			} else {
				smellResults++
			}
		}
	}

	logger.Info("Smell detection complete", "smells", smellResults)
	return nil
}

// emitStructuredFact emits a structured fact triple to the analytical store.
// The predicate is derived from the template ID (e.g., "smell_circular_direct").
// Value fields are stored as separate predicates.
func (a *Analyzer) emitStructuredFact(store *meb.MEBStore, result map[string]any, templateID, category, severity string) error {
	var subject string

	if s, ok := result["File"].(string); ok {
		subject = s
	} else if s, ok := result["A"].(string); ok {
		subject = s
	} else if s, ok := result["Subject"].(string); ok {
		subject = s
	}

	if subject == "" {
		return fmt.Errorf("no subject found in result")
	}

	facts := []meb.Fact{
		{Subject: subject, Predicate: "has_smell_type", Object: templateID},
		{Subject: subject, Predicate: "has_smell_category", Object: category},
		{Subject: subject, Predicate: "has_smell_severity", Object: severity},
	}

	for _, fact := range facts {
		if err := store.AddFact(fact); err != nil {
			return err
		}
	}

	return nil
}
