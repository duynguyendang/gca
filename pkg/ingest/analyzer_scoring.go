package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

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

// KPISnapshot is one point in a project's health-over-time series (F2). It is
// stored as a single fact keyed by a stable snapshot ID so trends can be read
// back without recomputing anything.
type KPISnapshot struct {
	ID              string    `json:"id"`
	CommitSHA       string    `json:"commit,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
	HealthScore     int       `json:"health_score"`
	HealthDebt      int       `json:"health_debt"`
	SmellCount      int       `json:"smell_count"`
	TopSmell        string    `json:"top_smell,omitempty"`
	DeadCodeCount   int       `json:"dead_code_count"`
	ComplexityCount int       `json:"complexity_count"`
	DuplicateCount  int       `json:"duplicate_count"`
}

// recordKPISnapshot persists one compact KPI record keyed by commit SHA (or a
// synthetic sequence ID when no git data is present) and prunes to the newest
// KPISnapshotRetention snapshots. It must run AFTER computeHealthScores so the
// health facts exist in the analytical store.
func (a *Analyzer) recordKPISnapshot(ctx context.Context, projectID string) error {
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	snap, err := a.collectKPISnapshot(ctx, projectID, analyticalStore)
	if err != nil {
		return err
	}

	// Persist as a single JSON object fact under PredicateKPISnapshot. Using the
	// JSON body keeps the fact surface tiny (F2 risk: storage growth).
	body, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("failed to marshal KPI snapshot: %w", err)
	}
	fact := meb.Fact{
		Subject:   snap.ID,
		Predicate: config.PredicateKPISnapshot,
		Object:    string(body),
	}
	if err := analyticalStore.AddFact(fact); err != nil {
		return fmt.Errorf("failed to add KPI snapshot: %w", err)
	}

	pruned := a.pruneKPISnapshots(ctx, analyticalStore)
	logger.Info("KPI snapshot recorded", "project", projectID, "id", snap.ID, "score", snap.HealthScore, "debt", snap.HealthDebt, "pruned", pruned)
	return nil
}

// collectKPISnapshot computes the aggregate KPI values from the analytical store.
func (a *Analyzer) collectKPISnapshot(ctx context.Context, projectID string, analyticalStore *meb.MEBStore) (*KPISnapshot, error) {
	// Health totals: every has_health_score / has_health_debt fact, summed.
	totalScore, totalDebt, scoredFiles := 0, 0, 0
	for fact := range analyticalStore.ScanContext(ctx, "", config.PredicateHasHealthScore, "") {
		if fact.Subject == "" {
			continue
		}
		if v, err := strconv.Atoi(fmt.Sprintf("%v", fact.Object)); err == nil {
			totalScore += v
			scoredFiles++
		}
	}
	for fact := range analyticalStore.ScanContext(ctx, "", config.PredicateHasHealthDebt, "") {
		if fact.Subject == "" {
			continue
		}
		if v, err := strconv.Atoi(fmt.Sprintf("%v", fact.Object)); err == nil {
			totalDebt += v
		}
	}

	// Smell tallies from has_smell_type facts.
	smellCount := 0
	byType := make(map[string]int)
	for fact := range analyticalStore.ScanContext(ctx, "", "has_smell_type", "") {
		if fact.Subject == "" {
			continue
		}
		smellCount++
		if st, ok := fact.Object.(string); ok && st != "" {
			byType[st]++
		}
	}

	topSmell := ""
	topCount := 0
	for st, n := range byType {
		if n > topCount {
			topSmell, topCount = st, n
		}
	}

	// Average health score (design: overall_score in get_health_summary is
	// 100 - debt/10; we store the per-file mean here).
	avgScore := 0
	if scoredFiles > 0 {
		avgScore = totalScore / scoredFiles
	}

	// Snapshot ID: commit SHA when present, else synthetic sequence.
	commit := ""
	if sourceStore, err := a.storeManager.GetSourceStore(projectID); err == nil {
		commit = GetLastCommitSHA(sourceStore)
	}
	snap := &KPISnapshot{
		CommitSHA:       commit,
		Timestamp:       time.Now().UTC(),
		HealthScore:     avgScore,
		HealthDebt:      totalDebt,
		SmellCount:      smellCount,
		TopSmell:        topSmell,
		DeadCodeCount:   byType["dead_code"],
		ComplexityCount: byType["high_complexity"],
		DuplicateCount:  byType["duplicate_code"],
	}
	snap.ID = snapCommitID(projectID, commit)
	return snap, nil
}

// snapCommitID returns a stable snapshot ID. When a commit SHA is available it
// is used directly (deduplicates re-runs on the same commit); otherwise a
// synthetic monotonically increasing ID keeps series points distinct.
func snapCommitID(projectID, commit string) string {
	if commit != "" {
		return "kpi:" + projectID + ":" + commit
	}
	return "kpi:" + projectID + ":t" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// pruneKPISnapshots removes all but the newest KPISnapshotRetention snapshots
// for a project. Returns the number pruned.
func (a *Analyzer) pruneKPISnapshots(ctx context.Context, analyticalStore *meb.MEBStore) int {
	type snapMeta struct {
		id string
		ts time.Time
	}
	var all []snapMeta
	for fact := range analyticalStore.ScanContext(ctx, "", config.PredicateKPISnapshot, "") {
		if fact.Subject == "" {
			continue
		}
		var s KPISnapshot
		if obj, ok := fact.Object.(string); ok {
			if err := json.Unmarshal([]byte(obj), &s); err == nil {
				all = append(all, snapMeta{id: fact.Subject, ts: s.Timestamp})
				continue
			}
		}
		all = append(all, snapMeta{id: fact.Subject, ts: time.Time{}})
	}

	if len(all) <= config.KPISnapshotRetention {
		return 0
	}

	// Keep the newest N by timestamp; stable tie-break by id.
	sort.Slice(all, func(i, j int) bool {
		if !all[i].ts.Equal(all[j].ts) {
			return all[i].ts.After(all[j].ts)
		}
		return all[i].id > all[j].id
	})

	pruned := 0
	for _, meta := range all[config.KPISnapshotRetention:] {
		if err := analyticalStore.DeleteFactsBySubject(meta.id); err != nil {
			logger.Warn("Failed to prune KPI snapshot", "id", meta.id, "error", err)
		} else {
			pruned++
		}
	}
	return pruned
}
