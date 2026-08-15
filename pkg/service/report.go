package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/gca/pkg/okf"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/duynguyendang/meb"
)

// NarrativeService generates optional AI prose for report sections (F5).
type NarrativeService interface {
	HandleAsk(ctx context.Context, req ai.AskRequest) (*ai.AskResponse, error)
}

// ReportStoreManager is the store surface ReportService needs: partitioned
// stores for the source graph, analytical facts, and OKF concepts.
type ReportStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
	ListProjects() ([]manager.ProjectMetadata, error)
}

// ReportService assembles regenerable markdown architecture reports (F5) from
// the Analytical Store, graph queries, and the OKF export path.
type ReportService struct {
	manager  ReportStoreManager
	narrator NarrativeService
}

// NewReportService creates a ReportService over a project store manager.
// narrator may be nil to disable AI narratives.
func NewReportService(manager ReportStoreManager, narrator NarrativeService) *ReportService {
	return &ReportService{manager: manager, narrator: narrator}
}

// ReportSection lists the supported report sections.
var ReportSectionNames = []string{"overview", "entry_points", "hubs", "smells", "clusters", "call_flows", "okf"}

// ReportOptions controls report generation.
type ReportOptions struct {
	ProjectID string
	Sections  []string // empty = all sections
	IncludeAI bool
}

// GenerateMarkdown renders a project's architecture report as markdown.
func (s *ReportService) GenerateMarkdown(ctx context.Context, opts ReportOptions) (string, error) {
	if opts.ProjectID == "" {
		return "", fmt.Errorf("project_id is required")
	}

	store, err := s.manager.GetStore(opts.ProjectID)
	if err != nil {
		return "", fmt.Errorf("project not found: %s", opts.ProjectID)
	}

	analytical, err := s.manager.GetAnalyticalStore(opts.ProjectID)
	if err != nil {
		return "", fmt.Errorf("analytical store: %w", err)
	}

	wanted := map[string]bool{}
	for _, name := range opts.Sections {
		wanted[name] = true
	}

	var sb strings.Builder
	sb.WriteString("# Architecture Report\n\n")
	sb.WriteString(fmt.Sprintf("- **Project**: `%s`\n", opts.ProjectID))
	sb.WriteString(fmt.Sprintf("- **Generated at**: %s\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Latest commit**: `%s`\n", s.latestCommit(ctx, opts.ProjectID)))
	sb.WriteString("\n---\n\n")

	sectionBuilder := func(name string) bool {
		if len(wanted) == 0 || wanted[name] {
			return true
		}
		return false
	}

	if sectionBuilder("overview") {
		sb.WriteString(s.overviewSection(ctx, analytical, store))
	}
	if sectionBuilder("entry_points") {
		sb.WriteString(s.entryPointsSection(ctx, analytical, store))
	}
	if sectionBuilder("hubs") {
		sb.WriteString(s.hubsSection(ctx, analytical, store))
	}
	if sectionBuilder("smells") {
		sb.WriteString(s.smellsSection(ctx, analytical))
	}
	if sectionBuilder("clusters") {
		sb.WriteString(s.clustersSection(ctx, analytical))
	}
	if sectionBuilder("call_flows") {
		sb.WriteString(s.callFlowsSection(ctx, analytical, store))
	}
	if sectionBuilder("okf") {
		sb.WriteString(s.okfSection(ctx, opts.ProjectID))
	}

	if opts.IncludeAI {
		if narration, err := s.narrative(ctx, opts.ProjectID, sb.String()); err == nil && narration != "" {
			sb.WriteString("\n## AI Narrative\n\n")
			sb.WriteString(narration)
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// overviewSection summarizes health, smells, deps, and KPI trend.
func (s *ReportService) overviewSection(ctx context.Context, analytical, store *meb.MEBStore) string {
	var sb strings.Builder
	sb.WriteString("## Overview\n\n")

	totalFiles := 0
	for range store.ScanContext(ctx, "", config.PredicateDefines, "") {
		totalFiles++
	}

	smellCount := 0
	byType := make(map[string]int)
	for fact := range analytical.ScanContext(ctx, "", "has_smell_type", "") {
		if fact.Subject == "" {
			continue
		}
		smellCount++
		if st, ok := fact.Object.(string); ok && st != "" {
			byType[st]++
		}
	}

	depCount := 0
	for range store.ScanContext(ctx, "", config.PredicateImports, "") {
		depCount++
	}

	latest := s.latestKPISnapshot(ctx, analytical)
	sb.WriteString(fmt.Sprintf("| Metric | Value |\n|---|---|\n"))
	sb.WriteString(fmt.Sprintf("| Files | %d |\n", totalFiles))
	sb.WriteString(fmt.Sprintf("| Dependencies | %d |\n", depCount))
	sb.WriteString(fmt.Sprintf("| Total smells | %d |\n", smellCount))
	if latest != nil {
		sb.WriteString(fmt.Sprintf("| Health score | %d/100 |\n", latest.HealthScore))
		sb.WriteString(fmt.Sprintf("| Health debt | %d |\n", latest.HealthDebt))
		sb.WriteString(fmt.Sprintf("| Top smell | %s (%d) |\n", latest.TopSmell, latest.SmellCount))
	}
	sb.WriteString("\n")
	return sb.String()
}

// entryPointsSection lists entry points with degrees.
func (s *ReportService) entryPointsSection(ctx context.Context, analytical, store *meb.MEBStore) string {
	var sb strings.Builder
	sb.WriteString("## Entry Points\n\n")

	type entry struct {
		name string
		in   int
		out  int
	}
	entries := map[string]int{}
	for fact := range analytical.ScanContext(ctx, "", "is_entry_point", "") {
		if fact.Subject != "" {
			entries[fact.Subject] = 0
		}
	}
	// Compute out-degree from the source store calls graph.
	outDeg := map[string]int{}
	for fact := range store.ScanContext(ctx, "", config.PredicateCalls, "") {
		if fact.Subject != "" {
			outDeg[fact.Subject]++
		}
	}

	var names []string
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)

	if len(names) == 0 {
		sb.WriteString("_No entry points detected._\n\n")
		return sb.String()
	}
	sb.WriteString("| Symbol | Out-degree |\n|---|---|\n")
	for _, n := range names {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", n, outDeg[n]))
	}
	sb.WriteString("\n")
	return sb.String()
}

// hubsSection lists hub files with scores.
func (s *ReportService) hubsSection(ctx context.Context, analytical, store *meb.MEBStore) string {
	var sb strings.Builder
	sb.WriteString("## Hubs\n\n")

	type hub struct {
		name  string
		score float64
	}
	var hubs []hub
	for fact := range analytical.ScanContext(ctx, "", "has_hub_score", "") {
		if fact.Subject == "" {
			continue
		}
		if score, err := strconv.ParseFloat(fmt.Sprintf("%v", fact.Object), 64); err == nil {
			hubs = append(hubs, hub{name: fact.Subject, score: score})
		}
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].score > hubs[j].score })
	if len(hubs) > 20 {
		hubs = hubs[:20]
	}

	if len(hubs) == 0 {
		sb.WriteString("_No hub files detected._\n\n")
		return sb.String()
	}
	sb.WriteString("| File | Hub score |\n|---|---|\n")
	for _, h := range hubs {
		sb.WriteString(fmt.Sprintf("| %s | %.2f |\n", h.name, h.score))
	}
	sb.WriteString("\n")
	return sb.String()
}

// smellsSection groups smells by type.
func (s *ReportService) smellsSection(ctx context.Context, analytical *meb.MEBStore) string {
	var sb strings.Builder
	sb.WriteString("## Smells\n\n")

	byType := map[string][]string{}
	for fact := range analytical.ScanContext(ctx, "", "has_smell_type", "") {
		if fact.Subject == "" {
			continue
		}
		st, ok := fact.Object.(string)
		if !ok || st == "" {
			continue
		}
		byType[st] = append(byType[st], fact.Subject)
	}

	if len(byType) == 0 {
		sb.WriteString("_No architectural smells detected._\n\n")
		return sb.String()
	}

	var types []string
	for st := range byType {
		types = append(types, st)
	}
	sort.Strings(types)

	for _, st := range types {
		subjects := byType[st]
		sort.Strings(subjects)
		display := subjects
		if len(display) > 10 {
			display = display[:10]
		}
		sb.WriteString(fmt.Sprintf("### %s (%d)\n\n", st, len(subjects)))
		for _, subj := range display {
			sb.WriteString(fmt.Sprintf("- %s\n", subj))
		}
		if len(subjects) > 10 {
			sb.WriteString(fmt.Sprintf("- _... and %d more_\n", len(subjects)-10))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// clustersSection lists community/cluster assignments.
func (s *ReportService) clustersSection(ctx context.Context, analytical *meb.MEBStore) string {
	var sb strings.Builder
	sb.WriteString("## Clusters\n\n")

	byCluster := map[string][]string{}
	for fact := range analytical.ScanContext(ctx, "", "belongs_to_cluster", "") {
		if fact.Subject == "" {
			continue
		}
		cl, ok := fact.Object.(string)
		if !ok || cl == "" {
			continue
		}
		byCluster[cl] = append(byCluster[cl], fact.Subject)
	}

	if len(byCluster) == 0 {
		sb.WriteString("_No cluster assignments._\n\n")
		return sb.String()
	}

	var clusters []string
	for c := range byCluster {
		clusters = append(clusters, c)
	}
	sort.Strings(clusters)

	sb.WriteString("| Cluster | Members |\n|---|---|\n")
	for _, c := range clusters {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", c, len(byCluster[c])))
	}
	sb.WriteString("\n")
	return sb.String()
}

// callFlowsSection lists top entry-point downstream reachability (bounded).
func (s *ReportService) callFlowsSection(ctx context.Context, analytical, store *meb.MEBStore) string {
	var sb strings.Builder
	sb.WriteString("## Call Flows\n\n")

	// Top entry points by out-degree.
	outDeg := map[string]int{}
	for fact := range store.ScanContext(ctx, "", config.PredicateCalls, "") {
		if fact.Subject != "" {
			outDeg[fact.Subject]++
		}
	}
	entrySet := map[string]bool{}
	for fact := range analytical.ScanContext(ctx, "", "is_entry_point", "") {
		if fact.Subject != "" {
			entrySet[fact.Subject] = true
		}
	}

	var top []string
	for n := range entrySet {
		top = append(top, n)
	}
	sort.Slice(top, func(i, j int) bool {
		if outDeg[top[i]] != outDeg[top[j]] {
			return outDeg[top[i]] > outDeg[top[j]]
		}
		return top[i] < top[j]
	})
	if len(top) > 5 {
		top = top[:5]
	}

	if len(top) == 0 {
		sb.WriteString("_No entry points to trace._\n\n")
		return sb.String()
	}

	// BFS down to bounded depth.
	for _, entry := range top {
		sb.WriteString(fmt.Sprintf("### %s\n\n", entry))
		seen := map[string]bool{entry: true}
		queue := []string{entry}
		depth := 0
		for len(queue) > 0 && depth < 4 {
			next := queue[0]
			queue = queue[1:]
			for fact := range store.ScanContext(ctx, next, config.PredicateCalls, "") {
				callee, ok := fact.Object.(string)
				if !ok || callee == "" || seen[callee] {
					continue
				}
				seen[callee] = true
				sb.WriteString(fmt.Sprintf("- %s\n", callee))
				if len(seen) >= 20 {
					break
				}
				queue = append(queue, callee)
			}
			depth++
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// okfSection lists linked OKF concepts.
func (s *ReportService) okfSection(ctx context.Context, projectID string) string {
	var sb strings.Builder
	sb.WriteString("## OKF Concepts\n\n")

	source, err := s.manager.GetSourceStore(projectID)
	if err != nil {
		sb.WriteString("_OKF source store unavailable._\n\n")
		return sb.String()
	}

	concepts := map[string][]string{}
	for fact := range source.ScanContext(ctx, "", okf.PredBridgesTo.String(), "") {
		if fact.Subject == "" {
			continue
		}
		obj, ok := fact.Object.(string)
		if !ok {
			continue
		}
		concepts[fact.Subject] = append(concepts[fact.Subject], obj)
	}

	if len(concepts) == 0 {
		sb.WriteString("_No OKF concepts linked._\n\n")
		return sb.String()
	}

	var names []string
	for c := range concepts {
		names = append(names, c)
	}
	sort.Strings(names)
	sb.WriteString("| Concept | Linked symbols |\n|---|---|\n")
	for _, c := range names {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", c, len(concepts[c])))
	}
	sb.WriteString("\n")
	return sb.String()
}

// latestKPISnapshot returns the newest KPI snapshot for the overview.
func (s *ReportService) latestKPISnapshot(ctx context.Context, analytical *meb.MEBStore) *ingest.KPISnapshot {
	var latest *ingest.KPISnapshot
	for fact := range analytical.ScanContext(ctx, "", config.PredicateKPISnapshot, "") {
		if fact.Subject == "" {
			continue
		}
		obj, ok := fact.Object.(string)
		if !ok {
			continue
		}
		var snap ingest.KPISnapshot
		if err := json.Unmarshal([]byte(obj), &snap); err != nil {
			continue
		}
		if latest == nil || snap.Timestamp.After(latest.Timestamp) {
			cp := snap
			latest = &cp
		}
	}
	return latest
}

// latestCommit reads the last commit SHA from the source store.
func (s *ReportService) latestCommit(ctx context.Context, projectID string) string {
	source, err := s.manager.GetSourceStore(projectID)
	if err != nil {
		return ""
	}
	return ingest.GetLastCommitSHA(source)
}

// narrative generates one optional AI narrative for the report.
func (s *ReportService) narrative(ctx context.Context, projectID, report string) (string, error) {
	if s.narrator == nil {
		return "", nil
	}
	req := ai.AskRequest{
		ProjectID: projectID,
		Query:     "Provide an executive summary of this architecture report, highlighting the most critical architectural risks and strengths.",
		Context:   report,
	}
	resp, err := s.narrator.HandleAsk(ctx, req)
	if err != nil {
		logger.Warn("Report AI narrative failed", "error", err)
		return "", err
	}
	return resp.Answer, nil
}
