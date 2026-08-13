package ingest

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/logger"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

// detectSmells runs smell detection queries and writes results to Analytical Store.
// Smell templates are loaded from the TemplateStore (backed by policies/smells/*.mg).
// This makes smell detection fully Datalog-driven - adding new smells requires only .mg files.
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
	} else if s, ok := result["Concept"].(string); ok {
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
