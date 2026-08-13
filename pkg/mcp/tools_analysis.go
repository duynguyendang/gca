package mcp

import (
	"context"
	"sort"
	"strconv"

	"github.com/duynguyendang/meb"
	"github.com/mark3labs/mcp-go/mcp"
)

// --- Complexity tool ---

type complexityEntry struct {
	Symbol     string `json:"symbol"`
	Complexity int    `json:"complexity"`
	File       string `json:"file"`
}

func (s *Server) handleListHighComplexity(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	threshold := optionalInt(args, "threshold", 15)
	limit := optionalInt(args, "limit", defaultScanLimit)

	s.mu.Lock()
	defer s.mu.Unlock()

	sourceStore, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	var entries []complexityEntry
	for fact := range sourceStore.ScanContext(ctx, "", "has_complexity", "") {
		sym := fact.Subject
		if sym == "" {
			continue
		}
		comp, ok := fact.Object.(int)
		if !ok {
			if s, ok := fact.Object.(string); ok {
				if v, err := strconv.Atoi(s); err == nil {
					comp = v
				}
			}
		}
		if comp < threshold {
			continue
		}
		file := findDefiningFile(ctx, sourceStore, sym)
		entries = append(entries, complexityEntry{
			Symbol:     sym,
			Complexity: comp,
			File:       file,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Complexity > entries[j].Complexity
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}

	return jsonResult(entries), nil
}

// --- Duplicate groups tool ---

type duplicateGroup struct {
	Hash     string   `json:"hash"`
	Symbols  []string `json:"symbols"`
	Files    []string `json:"files"`
	Severity string   `json:"severity"`
}

func (s *Server) handleListDuplicateGroups(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	limit := optionalInt(args, "limit", defaultScanLimit)

	s.mu.Lock()
	defer s.mu.Unlock()

	sourceStore, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	hashToSymbols := make(map[string][]string)
	for fact := range sourceStore.ScanContext(ctx, "", "has_body_hash", "") {
		hash, ok := fact.Object.(string)
		if !ok || hash == "" || fact.Subject == "" {
			continue
		}
		hashToSymbols[hash] = append(hashToSymbols[hash], fact.Subject)
	}

	var groups []duplicateGroup
	for hash, syms := range hashToSymbols {
		if len(syms) < 2 {
			continue
		}
		filesSet := make(map[string]bool)
		for _, sym := range syms {
			file := findDefiningFile(ctx, sourceStore, sym)
			if file != "" {
				filesSet[file] = true
			}
		}
		var files []string
		for f := range filesSet {
			files = append(files, f)
		}
		severity := "low"
		if len(syms) >= 3 {
			severity = "medium"
		}
		groups = append(groups, duplicateGroup{
			Hash:     hash,
			Symbols:  syms,
			Files:    files,
			Severity: severity,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].Symbols) > len(groups[j].Symbols)
	})
	if len(groups) > limit {
		groups = groups[:limit]
	}

	return jsonResult(groups), nil
}

// --- Health overview tool (richer than get_health_summary) ---

type projectHealth struct {
	Project          string `json:"project"`
	OverallScore     int    `json:"overall_score"`
	TotalFacts       int    `json:"total_facts"`
	DeadCodeCount    int    `json:"dead_code_count"`
	HighComplexCount int    `json:"high_complexity_count"`
	DuplicateGroups  int    `json:"duplicate_groups"`
	HubFiles         int    `json:"hub_files"`
	EntryPoints      int    `json:"entry_points"`
}

func (s *Server) handleProjectHealthOverview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sourceStore, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}
	analyticalStore, err := s.mgr.GetAnalyticalStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	h := projectHealth{Project: project}

	// Dead code count
	for fact := range analyticalStore.ScanContext(ctx, "", "has_smell_type", "dead_code") {
		if fact.Subject != "" {
			h.DeadCodeCount++
		}
	}

	// High complexity count
	for fact := range sourceStore.ScanContext(ctx, "", "has_complexity", "") {
		if fact.Subject == "" {
			continue
		}
		comp, _ := toInt(fact.Object)
		if comp >= 15 {
			h.HighComplexCount++
		}
	}

	// Duplicate groups
	hashes := make(map[string]int)
	for fact := range sourceStore.ScanContext(ctx, "", "has_body_hash", "") {
		hash, ok := fact.Object.(string)
		if ok && hash != "" && fact.Subject != "" {
			hashes[hash]++
		}
	}
	for _, count := range hashes {
		if count >= 2 {
			h.DuplicateGroups++
		}
	}

	// Hub files
	for fact := range analyticalStore.ScanContext(ctx, "", "has_hub_score", "") {
		if fact.Subject != "" {
			h.HubFiles++
		}
	}

	// Entry points
	for fact := range analyticalStore.ScanContext(ctx, "", "is_entry_point", "") {
		if fact.Subject != "" {
			h.EntryPoints++
		}
	}

	h.TotalFacts = int(sourceStore.Count())

	// Score from health_score
	_ = queryHealthScore(ctx, analyticalStore, &h)

	return jsonResult(h), nil
}

func queryHealthScore(ctx context.Context, store *meb.MEBStore, h *projectHealth) error {
	// Try to get a health score from the first file with a score.
	for fact := range store.ScanContext(ctx, "", "has_health_score", "") {
		if fact.Subject == "" {
			continue
		}
		score, ok := toInt(fact.Object)
		if ok {
			h.OverallScore = score
			return nil
		}
	}
	// Fallback: compute a simple score from debt.
	totalDebt := 0
	for fact := range store.ScanContext(ctx, "", "has_health_debt", "") {
		debt, _ := toInt(fact.Object)
		totalDebt += debt
	}
	h.OverallScore = 100 - totalDebt/10
	if h.OverallScore < 0 {
		h.OverallScore = 0
	}
	return nil
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i, true
		}
	}
	return 0, false
}

// findDefiningFile scans defines to find which file contains a symbol.
func findDefiningFile(ctx context.Context, store *meb.MEBStore, sym string) string {
	for fact := range store.ScanContext(ctx, "", "defines", sym) {
		if fact.Subject != "" {
			return fact.Subject
		}
	}
	return ""
}
