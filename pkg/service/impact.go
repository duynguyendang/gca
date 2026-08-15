package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ephemeral"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/duynguyendang/meb"
)

// ImpactStoreManager exposes the partitioned stores the impact report reads.
type ImpactStoreManager interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// ImpactReportService produces CI-friendly PR blast-radius reports (F3).
type ImpactReportService struct {
	ephemeral *ephemeral.EphemeralStore
	manager   ImpactStoreManager
	narrator  NarrativeService
}

// NewImpactReportService creates an ImpactReportService.
// narrator may be nil to disable the AI narrative.
func NewImpactReportService(es *ephemeral.EphemeralStore, manager ImpactStoreManager, narrator NarrativeService) *ImpactReportService {
	return &ImpactReportService{ephemeral: es, manager: manager, narrator: narrator}
}

// ImpactReport is the blast-radius report JSON shape.
type ImpactReport struct {
	SessionID             string         `json:"session_id"`
	ProjectID             string         `json:"project_id"`
	BaseCommit            string         `json:"base_commit,omitempty"`
	HeadCommit            string         `json:"head_commit,omitempty"`
	TouchedFiles          []string       `json:"touched_files"`
	TouchedFileCount      int            `json:"touched_file_count"`
	TouchedSymbols        []string       `json:"touched_symbols"`
	HubFilesHit           []string       `json:"hub_files_hit"`
	EntryPointsAffected   []string       `json:"entry_points_affected"`
	SmellsPreExisting     map[string]int `json:"smells_pre_existing"`
	SmellsNew             map[string]int `json:"smells_new"`
	ReachableCallersCount int            `json:"reachable_callers_count"`
	ReachableSymbols      []string       `json:"reachable_symbols"`
	Narrative             string         `json:"narrative,omitempty"`
	NewSmellThreshold     int            `json:"new_smell_threshold,omitempty"`
	Blocked               bool           `json:"blocked"`
}

// SmellsNewCount returns the total number of newly introduced smells.
func (r *ImpactReport) SmellsNewCount() int {
	total := 0
	for _, n := range r.SmellsNew {
		total += n
	}
	return total
}

// Generate computes a blast-radius report for a unified diff.
// It parses the diff into a fresh ephemeral session, aggregates facts across
// the ephemeral/source/analytical stores, and expires the session when done.
func (s *ImpactReportService) Generate(ctx context.Context, projectID, diff string) (*ImpactReport, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if diff == "" {
		return nil, fmt.Errorf("diff is required")
	}

	session, _, err := s.ephemeral.ParseDiffAndCreateSession(projectID, diff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse diff: %w", err)
	}
	defer func() {
		_ = session.Close()
		s.ephemeral.DeleteSession(session.ID)
	}()

	analytical, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		return nil, fmt.Errorf("analytical store: %w", err)
	}
	source, err := s.manager.GetSourceStore(projectID)
	if err != nil {
		return nil, fmt.Errorf("source store: %w", err)
	}

	// 1. Touched files from diff facts.
	touched := map[string]bool{}
	for _, pred := range []string{ephemeral.DiffAdded, ephemeral.DiffModified, ephemeral.DiffRemoved} {
		for fact := range session.Facts.ScanContext(ctx, "", pred, "") {
			if fact.Subject != "" {
				touched[fact.Subject] = true
			}
		}
	}

	// Symbols defined in touched files.
	touchedSymbols := map[string]bool{}
	for file := range touched {
		for fact := range source.ScanContext(ctx, file, config.PredicateDefines, "") {
			if sym, ok := fact.Object.(string); ok && sym != "" {
				touchedSymbols[sym] = true
			}
		}
	}

	// 2. Hub files hit: touched files with a hub score.
	hubs := map[string]bool{}
	for fact := range analytical.ScanContext(ctx, "", "has_hub_score", "") {
		if fact.Subject == "" || !touched[fact.Subject] {
			continue
		}
		if score, err := strconv.ParseFloat(fmt.Sprintf("%v", fact.Object), 64); err == nil && score > 0 {
			hubs[fact.Subject] = true
		}
	}

	// 3. Entry points affected: entry-point symbols among touched symbols.
	entrySet := map[string]bool{}
	for fact := range analytical.ScanContext(ctx, "", "is_entry_point", "") {
		if fact.Subject != "" {
			entrySet[fact.Subject] = true
		}
	}
	entryPoints := map[string]bool{}
	for sym := range touchedSymbols {
		if entrySet[sym] {
			entryPoints[sym] = true
		}
	}

	// 4. Smells: pre-existing on touched files vs. newly introduced in diff.
	smellsPre := map[string]int{}
	smellsNew := map[string]int{}
	for fact := range analytical.ScanContext(ctx, "", "has_smell_type", "") {
		if fact.Subject == "" {
			continue
		}
		smellType, ok := fact.Object.(string)
		if !ok || smellType == "" {
			continue
		}
		if touched[fact.Subject] {
			smellsPre[smellType]++
		}
	}
	// New smells: a diff_added line whose symbol carries a smell fact. We
	// approximate by counting smell facts whose subject is a touched symbol
	// (introduced in the diff) rather than a pre-existing file-level smell.
	for sym := range touchedSymbols {
		for fact := range analytical.ScanContext(ctx, sym, "has_smell_type", "") {
			if smellType, ok := fact.Object.(string); ok && smellType != "" {
				smellsNew[smellType]++
			}
		}
	}

	// 5. Call-graph reachability: bounded BFS from touched symbols upward
	// (who-calls), reporting lower-bound callers.
	callers := map[string]bool{}
	queue := make([]string, 0, len(touchedSymbols))
	for sym := range touchedSymbols {
		callers[sym] = true
		queue = append(queue, sym)
	}
	depth := 0
	for len(queue) > 0 && depth < 4 {
		next := queue[0]
		queue = queue[1:]
		for fact := range source.ScanContext(ctx, "", config.PredicateCalls, next) {
			caller := fact.Subject
			if caller == "" || callers[caller] {
				continue
			}
			callers[caller] = true
			if len(callers) >= 200 {
				break
			}
			queue = append(queue, caller)
		}
		depth++
	}

	report := &ImpactReport{
		SessionID:             session.ID,
		ProjectID:             projectID,
		TouchedFiles:          sortedKeys(touched),
		TouchedFileCount:      len(touched),
		TouchedSymbols:        sortedKeys(touchedSymbols),
		HubFilesHit:           sortedKeys(hubs),
		EntryPointsAffected:   sortedKeys(entryPoints),
		SmellsPreExisting:     smellsPre,
		SmellsNew:             smellsNew,
		ReachableCallersCount: len(callers) - len(touchedSymbols),
		ReachableSymbols:      sortedKeys(callers),
	}

	// Optional AI narrative.
	if s.narrator != nil {
		if narration, err := s.narrate(ctx, projectID, report); err == nil {
			report.Narrative = narration
		} else {
			logger.Warn("Impact narrative failed", "error", err)
		}
	}

	return report, nil
}

// narrate produces an optional AI summary of the impact report.
func (s *ImpactReportService) narrate(ctx context.Context, projectID string, report *ImpactReport) (string, error) {
	summary := fmt.Sprintf(
		"PR touches %d files (%d symbols) in project %s. "+
			"Hubs hit: %v. Entry points affected: %v. Pre-existing smells: %v. New smells: %v. "+
			"Estimated reachable callers: %d.",
		report.TouchedFileCount, len(report.TouchedSymbols), projectID,
		report.HubFilesHit, report.EntryPointsAffected,
		report.SmellsPreExisting, report.SmellsNew, report.ReachableCallersCount,
	)
	resp, err := s.narrator.HandleAsk(ctx, ai.AskRequest{
		ProjectID: projectID,
		Query:     "Summarize the blast radius of this pull request and flag the highest-risk changes.",
		Context:   summary,
	})
	if err != nil {
		return "", err
	}
	return resp.Answer, nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
