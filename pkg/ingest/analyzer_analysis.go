package ingest

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// clearAnalyticalData removes old smells, hub scores, entry points, and centrality data
// from the analytical store to prevent stale data from previous ingests.
// Uses a single scan pass to collect all subjects, then batch-deletes per subject.
func (a *Analyzer) clearAnalyticalData(ctx context.Context, projectID string) error {
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	predicatesToClear := map[string]bool{
		"has_smell":          true,
		"has_smell_type":     true,
		"has_smell_category": true,
		"has_smell_severity": true,
		"has_hub_score":      true,
		"is_entry_point":     true,
		"has_centrality":     true,
		"has_in_degree":      true,
		"has_out_degree":     true,
		"belongs_to_cluster": true,
		"has_surprise":       true,
		"has_knowledge_gap":  true,
		"has_category":       true,
		"has_severity":       true,
		"has_health_score":   true,
		"has_health_debt":    true,
	}

	subjectsToDelete := make(map[string]bool)
	predCounts := make(map[string]int)

	for fact, err := range analyticalStore.ScanContext(ctx, "", "", "") {
		if err != nil {
			logger.Warn("Error scanning facts", "error", err)
			continue
		}
		if predicatesToClear[fact.Predicate] {
			subjectsToDelete[fact.Subject] = true
			predCounts[fact.Predicate]++
		}
	}

	for pred, count := range predCounts {
		logger.Info("Found subjects to clear", "predicate", pred, "count", count, "project", projectID)
	}

	clearedCount := 0
	for subject := range subjectsToDelete {
		if err := analyticalStore.DeleteFactsBySubject(subject); err != nil {
			logger.Warn("Failed to delete facts for subject", "subject", subject, "error", err)
		} else {
			clearedCount++
		}
	}

	if clearedCount > 0 {
		logger.Info("Cleared analytical data", "subjects", clearedCount, "project", projectID)
	}

	return nil
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

	// computeCentrality MUST run before executeRulesFromTemplates.
	// Templates like surprise.mg and knowledge_gaps.mg query facts written by
	// computeCentrality (has_in_degree, has_out_degree, belongs_to_cluster).
	if err := a.computeCentrality(ctx, projectID); err != nil {
		logger.Warn("Centrality computation failed", "error", err)
	}

	// Dead-code detection is a Go-side pass (the template engine cannot express
	// "no incoming calls" negation). Emits has_smell_type=dead_code.
	if err := a.detectDeadCode(ctx, projectID); err != nil {
		logger.Warn("Dead-code detection failed", "error", err)
	}

	// Duplicate detection groups functions by body hash and flags
	// files containing identical function bodies.
	if err := a.detectDuplicates(ctx, projectID); err != nil {
		logger.Warn("Duplicate detection failed", "error", err)
	}

	// Write okf_age_days facts so the stale smell policy (stale.mg) can fire.
	sourceStore, srcErr := a.storeManager.GetSourceStore(projectID)
	if srcErr == nil {
		if err := a.writeOKFAgeDays(ctx, sourceStore); err != nil {
			logger.Warn("OKF age days computation failed", "error", err)
		}
	}

	if err := a.executeRulesFromTemplates(ctx, projectID); err != nil {
		logger.Warn("Template rule execution failed", "error", err)
	}

	if err := a.computeHealthScores(ctx, projectID); err != nil {
		logger.Warn("Health score computation failed", "error", err)
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
