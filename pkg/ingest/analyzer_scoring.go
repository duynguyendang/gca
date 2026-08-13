package ingest

import (
	"context"
	"fmt"
	"strconv"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// hubDebtPenalty is the extra debt applied to hub files, mirroring scoring.mg's
// `Total #= Weight + (hub_high(File) ? 5 : 0)`.
const hubDebtPenalty = 5

// computeHealthScores computes health debt and score per file in Go and writes
// has_health_debt / has_health_score facts to the Analytical Store.
//
// Rationale: the scoring.mg rules use derived predicates (file_has_smell,
// smell_weight, hub_high) plus `not <derived>` atoms, which mebpkg.Query cannot
// evaluate (derived predicates are not stored facts — docs/designs/contract.md §5).
// This Go pass implements the same semantics from the policy smell_weight facts:
//
//	debt(File)  = Σ smell_weight(smell) for smells on File  (+ hubDebtPenalty if hub)
//	score(File) = max(0, 100 - debt(File))
//
// Files with no smells and no hub score get score 100 and no debt fact
// (mirroring scoring.mg's health_score(File, 100) rule).
func (a *Analyzer) computeHealthScores(ctx context.Context, projectID string) error {
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	weights := common.LoadSmellWeights()

	// Per-file smell types (has_smell_type is the canonical smell predicate).
	fileSmells := make(map[string][]string)
	for fact := range analyticalStore.ScanContext(ctx, "", "has_smell_type", "") {
		if fact.Subject == "" {
			continue // scanner sentinel
		}
		if st, ok := fact.Object.(string); ok && st != "" {
			fileSmells[fact.Subject] = append(fileSmells[fact.Subject], st)
		}
	}

	// Hub files: the writer (computeCentrality) only emits has_hub_score past
	// HubClassificationThreshold, so presence alone means "hub".
	hubFiles := make(map[string]bool)
	for fact := range analyticalStore.ScanContext(ctx, "", "has_hub_score", "") {
		if fact.Subject != "" {
			hubFiles[fact.Subject] = true
		}
	}

	// Enumerate every file via source-store defines subjects.
	files := make(map[string]bool)
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateDefines, "") {
		if fact.Subject != "" {
			files[fact.Subject] = true
		}
	}

	debtFacts, scoreFacts := 0, 0
	for file := range files {
		debt := 0
		for _, st := range fileSmells[file] {
			// Unknown smell types contribute 0 (scoring.mg: not smell_weight -> 0).
			if w, ok := weights[st]; ok {
				debt += w
			}
		}
		if hubFiles[file] {
			debt += hubDebtPenalty
		}

		if debt > 0 {
			fact := meb.Fact{Subject: file, Predicate: config.PredicateHasHealthDebt, Object: strconv.Itoa(debt)}
			if err := analyticalStore.AddFact(fact); err != nil {
				logger.Warn("Failed to add health_debt fact", "file", file, "error", err)
			} else {
				debtFacts++
			}
		}

		score := 100 - debt
		if score < 0 {
			score = 0
		}
		scoreFact := meb.Fact{Subject: file, Predicate: config.PredicateHasHealthScore, Object: strconv.Itoa(score)}
		if err := analyticalStore.AddFact(scoreFact); err != nil {
			logger.Warn("Failed to add health_score fact", "file", file, "error", err)
		} else {
			scoreFacts++
		}
	}

	logger.Info("Health scores computed", "debt_facts", debtFacts, "score_facts", scoreFacts, "files", len(files))
	return nil
}
