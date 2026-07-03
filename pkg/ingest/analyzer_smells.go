package ingest

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/config"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/logger"
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

// computeHealthScores executes scoring.mg rules against the Analytical Store
// and writes pre-computed health scores and debt facts.
func (a *Analyzer) computeHealthScores(ctx context.Context, projectID string) error {
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	healthDebtQuery := `health_debt_with_hub(File, Total)`
	results, err := mebpkg.Query(ctx, analyticalStore, healthDebtQuery)
	if err != nil {
		return fmt.Errorf("health_debt query failed: %w", err)
	}

	debtCount := 0
	for _, r := range results {
		file, ok := r["File"].(string)
		if !ok || file == "" {
			continue
		}
		total, ok := r["Total"].(float64)
		if !ok {
			continue
		}
		fact := meb.Fact{
			Subject:   file,
			Predicate: config.PredicateHasHealthDebt,
			Object:    fmt.Sprintf("%.0f", total),
		}
		if err := analyticalStore.AddFact(fact); err != nil {
			logger.Warn("Failed to add health_debt fact", "file", file, "error", err)
		} else {
			debtCount++
		}
	}

	healthScoreQuery := `health_score(File, Score)`
	scoreResults, err := mebpkg.Query(ctx, analyticalStore, healthScoreQuery)
	if err != nil {
		return fmt.Errorf("health_score query failed: %w", err)
	}

	scoreCount := 0
	for _, r := range scoreResults {
		file, ok := r["File"].(string)
		if !ok || file == "" {
			continue
		}
		score, ok := r["Score"].(float64)
		if !ok {
			continue
		}
		fact := meb.Fact{
			Subject:   file,
			Predicate: config.PredicateHasHealthScore,
			Object:    fmt.Sprintf("%.0f", score),
		}
		if err := analyticalStore.AddFact(fact); err != nil {
			logger.Warn("Failed to add health_score fact", "file", file, "error", err)
		} else {
			scoreCount++
		}
	}

	logger.Info("Health scores computed", "debt_facts", debtCount, "score_facts", scoreCount)
	return nil
}
