package ingest

import (
	"context"
	"fmt"

	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// Local clustering types to avoid import cycle with service package.
type clusterNode struct {
	ID        string
	Weight    float64
	Neighbors map[int]float64
}
type clusterLink struct {
	Source string
	Target string
}
type clusterResult struct {
	Clusters    map[int][]string
	NodeCluster map[string]int
}

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

	if err := a.computeCentrality(ctx, projectID); err != nil {
		logger.Warn("Centrality computation failed", "error", err)
	}

	if err := a.executeRulesFromTemplates(ctx, projectID); err != nil {
		logger.Warn("Template rule execution failed", "error", err)
	}

	if err := a.computeHealthScores(ctx, projectID); err != nil {
		logger.Warn("Health score computation failed", "error", err)
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
	} else if s, ok := result["Source"].(string); ok {
		subject = s
	} else if s, ok := result["Target"].(string); ok {
		subject = s
	} else if s, ok := result["Symbol"].(string); ok {
		subject = s
	} else if s, ok := result["Cluster"].(string); ok {
		subject = s
	} else if s, ok := result["Concept"].(string); ok {
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
