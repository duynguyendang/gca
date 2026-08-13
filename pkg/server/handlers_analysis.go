package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	apperrors "github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/logger"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
	"github.com/gin-gonic/gin"
)

// HealthSummary represents the health summary response.
type HealthSummary struct {
	ProjectID        string        `json:"project_id"`
	Summary          HealthDetails `json:"summary"`
	TotalSmells      int           `json:"total_smells"`
	TotalHubs        int           `json:"total_hubs"`
	TotalEntrypoints int           `json:"total_entry_points"`
}

// HealthDetails contains categorized health information.
type HealthDetails struct {
	CircularDeps    []SmellEntry `json:"circular_dependencies,omitempty"`
	GodFiles        []SmellEntry `json:"god_files,omitempty"`
	LayerViolations []SmellEntry `json:"layer_violations,omitempty"`
	Hubs            []HubEntry   `json:"hubs,omitempty"`
	Entrypoints     []string     `json:"entry_points,omitempty"`
}

// SmellEntry represents a detected smell.
type SmellEntry struct {
	File   string `json:"file"`
	Smell  string `json:"smell"`
	Detail string `json:"detail,omitempty"`
}

// HubEntry represents a hub file.
type HubEntry struct {
	File  string `json:"file"`
	Score int    `json:"score"`
}

// handleHealthSummary returns a health summary from the Analytical Store.
// Query parameters:
//   - project: project ID to query
//
// Response: JSON health summary with smells, hubs, and entry points.
func (s *Server) handleHealthSummary(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}

	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return
	}

	// Flat smells list for frontend compatibility
	type Smell struct {
		File      string `json:"file"`
		SmellType string `json:"smell_type"`
		Severity  string `json:"severity"`
	}

	summary := HealthDetails{}
	var smells []Smell
	totalSmells := 0
	totalHubs := 0
	totalEntrypoints := 0

	// Fetch from the Analytical partition where the smells actually live.
	analyticalStore, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		logger.Error("handleHealthSummary error", "project", projectID, "error", err)
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to access analytical store", err))
		return
	}

	// Query for smells - read structured facts from analytical store
	type smellResult struct {
		Subject   string
		SmellType string
		Severity  string
		Category  string
	}

	var smellResults []smellResult

	// Query has_smell_type facts
	typeQuery := common.GetNamedQuery("smell_type")
	if typeResults, err := mebpkg.Query(c.Request.Context(), analyticalStore, typeQuery); err == nil {
		for _, r := range typeResults {
			if subject, ok := r["Subject"].(string); ok {
				if smellType, ok := r["Type"].(string); ok {
					smellResults = append(smellResults, smellResult{
						Subject:   subject,
						SmellType: smellType,
					})
				}
			}
		}
	}

	// Query has_smell_severity facts to get severity
	severityQuery := common.GetNamedQuery("smell_severity")
	if sevResults, err := mebpkg.Query(c.Request.Context(), analyticalStore, severityQuery); err == nil {
		severityMap := make(map[string]string)
		for _, r := range sevResults {
			if subject, ok := r["Subject"].(string); ok {
				if severity, ok := r["Severity"].(string); ok {
					severityMap[subject] = severity
				}
			}
		}
		for i := range smellResults {
			if sev, ok := severityMap[smellResults[i].Subject]; ok {
				smellResults[i].Severity = sev
			}
		}
	}

	// Categorize smells and build response
	for _, sr := range smellResults {
		totalSmells++

		var entry SmellEntry
		var smellLabel string

		switch sr.SmellType {
		case "circular_dependency":
			entry = SmellEntry{File: sr.Subject, Smell: "circular_dependency"}
			summary.CircularDeps = append(summary.CircularDeps, entry)
			smellLabel = "Circular Dependency"
		case "god_file":
			entry = SmellEntry{File: sr.Subject, Smell: "god_file"}
			summary.GodFiles = append(summary.GodFiles, entry)
			smellLabel = "God File"
		case "layer_violation":
			entry = SmellEntry{File: sr.Subject, Smell: "layer_violation"}
			summary.LayerViolations = append(summary.LayerViolations, entry)
			smellLabel = "Layer Violation"
		default:
			entry = SmellEntry{File: sr.Subject, Smell: sr.SmellType}
			summary.GodFiles = append(summary.GodFiles, entry)
			smellLabel = sr.SmellType
		}

		severity := sr.Severity
		if severity == "" {
			severity = "Medium"
		}

		smells = append(smells, Smell{
			File:      sr.Subject,
			SmellType: smellLabel,
			Severity:  severity,
		})
	}

	// Query for hub scores
	hubResults, err := mebpkg.Query(c.Request.Context(), analyticalStore, common.GetNamedQuery("hub_score"))
	if err == nil {
		for _, r := range hubResults {
			subject, _ := r["Subject"].(string)
			scoreStr, _ := r["Score"].(string)
			if subject == "" {
				continue
			}
			score := 0
			if s, err := strconv.Atoi(scoreStr); err == nil {
				score = s
			}
			summary.Hubs = append(summary.Hubs, HubEntry{
				File:  subject,
				Score: score,
			})
			totalHubs++
		}
	}

	// Query for entry points
	entryResults, err := mebpkg.Query(c.Request.Context(), analyticalStore, common.GetNamedQuery("entry_point"))
	if err == nil {
		for _, r := range entryResults {
			subject, _ := r["Subject"].(string)
			if subject != "" {
				summary.Entrypoints = append(summary.Entrypoints, subject)
				totalEntrypoints++
			}
		}
	}

	// Calculate overall score (0-100)
	// Start with 100 and deduct for issues found
	overallScore := 100
	// Deduct 5 points per smell, 2 points per hub, 1 point per entry point (capped at 0)
	overallScore -= totalSmells * 5
	overallScore -= totalHubs * 2
	overallScore -= totalEntrypoints
	if overallScore < 0 {
		overallScore = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"overall_score":      overallScore,
		"total_smells":       totalSmells,
		"total_hubs":         totalHubs,
		"total_entry_points": totalEntrypoints,
		"smells":             smells,
	})
}

// HealthSummaryV2 is the per-file risk leaderboard format.
type HealthSummaryV2 struct {
	OverallScore        int            `json:"overall_score"`
	TotalSecurityAlerts int            `json:"total_security_alerts"`
	TotalArchDebt       int            `json:"total_arch_debt"`
	Files               []FileHealthV2 `json:"files"`
}

// FileHealthV2 is per-file health data for the risk leaderboard.
type FileHealthV2 struct {
	FileName       string   `json:"file_name"`
	TotalDebtScore int      `json:"total_debt_score"`
	SecurityIssues int      `json:"security_issues"`
	ArchSmells     []string `json:"arch_smells"`
}

// handleHealthSummaryV2 returns the V2 health summary with per-file risk breakdown.
func (s *Server) handleHealthSummaryV2(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return
	}

	analyticalStore, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			handleError(c, apperrors.NewAppError(http.StatusNotFound, "project not found", err))
			return
		}
		logger.Error("handleHealthSummaryV2 error", "project", projectID, "error", err)
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to access analytical store", err))
		return
	}

	// Build file -> smells mapping and pre-computed health debt.
	fileSmells := make(map[string][]string)
	fileHubScore := make(map[string]int)
	fileDebt := make(map[string]int)

	// Pre-computed health debt facts from scoring.mg
	if results, err := mebpkg.Query(c.Request.Context(), analyticalStore, common.GetNamedQuery("health_debt")); err == nil {
		for _, r := range results {
			subject, _ := r["Subject"].(string)
			debtStr, _ := r["Debt"].(string)
			if subject == "" || debtStr == "" {
				continue
			}
			if debt, err := strconv.Atoi(debtStr); err == nil {
				fileDebt[subject] = debt
			}
		}
	}

	// Smells: triples(Subject, "has_smell_type", Type) — canonical smell predicate
	if results, err := mebpkg.Query(c.Request.Context(), analyticalStore, common.GetNamedQuery("smell_type")); err == nil {
		for _, r := range results {
			subject, _ := r["Subject"].(string)
			object, _ := r["Object"].(string)
			if subject == "" || object == "" {
				continue
			}
			fileSmells[subject] = append(fileSmells[subject], object)
		}
	}

	// Hub scores: triples(Subject, "has_hub_score", Score)
	if results, err := mebpkg.Query(c.Request.Context(), analyticalStore, common.GetNamedQuery("hub_score")); err == nil {
		for _, r := range results {
			subject, _ := r["Subject"].(string)
			scoreStr, _ := r["Score"].(string)
			if subject == "" {
				continue
			}
			if score, err := strconv.Atoi(scoreStr); err == nil {
				fileHubScore[subject] = score
			}
		}
	}

	// Smell weights are read from the Analytical Store via smellRegistry.
	// This replaces the old hardcoded smellWeight map — adding a new smell
	// type only requires a change to policies/smells/*.mg, no Go changes.

	var files []FileHealthV2
	totalArchDebt := 0
	totalSecurity := 0

	for file, smells := range fileSmells {
		debt := 0
		var archSmells []string
		secIssues := 0

		for _, smell := range smells {
			if s.smellRegistry.IsSecurity(smell) {
				secIssues++
				totalSecurity++
			} else {
				archSmells = append(archSmells, smell)
			}
		}

		// Use pre-computed debt if available, else sum weights + hub
		if preComputedDebt, ok := fileDebt[file]; ok {
			debt = preComputedDebt
		} else {
			for _, smell := range smells {
				if w, ok := s.smellRegistry.Weight(smell); ok {
					debt += w
				} else {
					debt += s.smellRegistry.DefaultWeight()
				}
			}
			if hub, ok := fileHubScore[file]; ok {
				debt += hub
			}
		}

		files = append(files, FileHealthV2{
			FileName:       file,
			TotalDebtScore: debt,
			SecurityIssues: secIssues,
			ArchSmells:     archSmells,
		})
		totalArchDebt += debt
	}

	// Add files that have pre-computed debt but no smells detected
	for file, debt := range fileDebt {
		if _, exists := fileSmells[file]; !exists {
			files = append(files, FileHealthV2{
				FileName:       file,
				TotalDebtScore: debt,
				SecurityIssues: 0,
				ArchSmells:     []string{},
			})
			totalArchDebt += debt
		}
	}

	// Overall score: 100 minus total arch debt (capped at 0)
	overallScore := 100 - totalArchDebt/10
	if overallScore < 0 {
		overallScore = 0
	}

	c.JSON(http.StatusOK, HealthSummaryV2{
		OverallScore:        overallScore,
		TotalSecurityAlerts: totalSecurity,
		TotalArchDebt:       totalArchDebt,
		Files:               files,
	})
}

type SurpriseFactor struct {
	Type  string  `json:"type"`
	Score float64 `json:"score"`
}

type SurpriseEdge struct {
	Source  string           `json:"source"`
	Target  string           `json:"target"`
	Score   float64          `json:"score"`
	Factors []SurpriseFactor `json:"factors"`
	SrcFile string           `json:"src_file,omitempty"`
	TgtFile string           `json:"tgt_file,omitempty"`
}

type SurpriseResponse struct {
	Edges       []SurpriseEdge `json:"edges"`
	TotalCount  int            `json:"total_count"`
	HighCount   int            `json:"high_count"`
	MediumCount int            `json:"medium_count"`
	LowCount    int            `json:"low_count"`
}

func (s *Server) handleSurpriseAnalysis(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return
	}

	analyticalStore, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to access analytical store", err))
		return
	}

	type surpriseResult struct {
		Subject      string
		Target       string
		SurpriseType string
		Score        string
	}

	var surpriseResults []surpriseResult

	query := common.GetNamedQuery("surprise")
	if results, err := mebpkg.Query(c.Request.Context(), analyticalStore, query); err == nil {
		for _, r := range results {
			if subject, ok := r["Subject"].(string); ok {
				if target, ok := r["Target"].(string); ok {
					if stype, ok := r["Type"].(string); ok {
						surpriseResults = append(surpriseResults, surpriseResult{
							Subject:      subject,
							Target:       target,
							SurpriseType: stype,
						})
					}
				}
			}
		}
	}

	// Also query for surprise score facts (composite scores)
	scoreQuery := common.GetNamedQuery("surprise_score")
	scoreMap := make(map[string]float64)
	if scoreResults, err := mebpkg.Query(c.Request.Context(), analyticalStore, scoreQuery); err == nil {
		for _, r := range scoreResults {
			if subject, ok := r["Subject"].(string); ok {
				if scoreStr, ok := r["ScoreStr"].(string); ok {
					var score float64
					fmt.Sscanf(scoreStr, "%f", &score)
					scoreMap[subject] = score
				}
			}
		}
	}

	// Aggregate by edge (Subject->Target)
	edgeMap := make(map[string]*SurpriseEdge)
	for _, sr := range surpriseResults {
		key := sr.Subject + "->" + sr.Target
		if _, exists := edgeMap[key]; !exists {
			edgeMap[key] = &SurpriseEdge{
				Source:  sr.Subject,
				Target:  sr.Target,
				Factors: []SurpriseFactor{},
			}
		}
		factorScore := 0.0
		switch sr.SurpriseType {
		case "surprise_cross_community":
			factorScore = 0.30
		case "surprise_cross_language":
			factorScore = 0.20
		case "surprise_peripheral_hub":
			factorScore = 0.20
		case "surprise_cross_test_boundary":
			factorScore = 0.25
		default:
			factorScore = 0.10
		}
		edgeMap[key].Factors = append(edgeMap[key].Factors, SurpriseFactor{
			Type:  sr.SurpriseType,
			Score: factorScore,
		})
	}

	var edges []SurpriseEdge
	for _, e := range edgeMap {
		var totalScore float64
		for _, f := range e.Factors {
			totalScore += f.Score
		}
		if totalScore > 1.0 {
			totalScore = 1.0
		}
		e.Score = totalScore
		edges = append(edges, *e)
	}

	// Sort by score descending
	sort.Slice(edges, func(i, j int) bool {
		return edges[j].Score < edges[i].Score
	})

	highCount, mediumCount, lowCount := 0, 0, 0
	for _, e := range edges {
		if e.Score >= 0.5 {
			highCount++
		} else if e.Score >= 0.2 {
			mediumCount++
		} else {
			lowCount++
		}
	}

	c.JSON(http.StatusOK, SurpriseResponse{
		Edges:       edges,
		TotalCount:  len(edges),
		HighCount:   highCount,
		MediumCount: mediumCount,
		LowCount:    lowCount,
	})
}

type KnowledgeGapItem struct {
	Symbol   string `json:"symbol"`
	GapType  string `json:"gap_type"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Degree   int    `json:"degree,omitempty"`
}

type KnowledgeGapsResponse struct {
	IsolatedNodes      []KnowledgeGapItem `json:"isolated_nodes"`
	UntestedHotspots   []KnowledgeGapItem `json:"untested_hotspots"`
	ThinCommunities    []KnowledgeGapItem `json:"thin_communities"`
	SingleFileClusters []KnowledgeGapItem `json:"single_file_clusters"`
	TotalCount         int                `json:"total_count"`
}

func (s *Server) handleKnowledgeGaps(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing project parameter"})
		return
	}

	analyticalStore, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to access analytical store", err))
		return
	}

	resp, err := s.computeKnowledgeGaps(c.Request.Context(), analyticalStore)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to compute knowledge gaps", err))
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) computeKnowledgeGaps(ctx context.Context, analyticalStore *meb.MEBStore) (*KnowledgeGapsResponse, error) {
	resp := &KnowledgeGapsResponse{
		IsolatedNodes:      []KnowledgeGapItem{},
		UntestedHotspots:   []KnowledgeGapItem{},
		ThinCommunities:    []KnowledgeGapItem{},
		SingleFileClusters: []KnowledgeGapItem{},
	}

	// Query degree facts
	inDegMap := make(map[string]int)
	outDegMap := make(map[string]int)
	if inResults, err := mebpkg.Query(ctx, analyticalStore, common.GetNamedQuery("in_degree_short")); err == nil {
		for _, r := range inResults {
			if s, ok := r["S"].(string); ok {
				if d, ok := r["D"].(string); ok {
					var deg int
					fmt.Sscanf(d, "%d", &deg)
					inDegMap[s] = deg
				}
			}
		}
	}
	if outResults, err := mebpkg.Query(ctx, analyticalStore, common.GetNamedQuery("out_degree_short")); err == nil {
		for _, r := range outResults {
			if s, ok := r["S"].(string); ok {
				if d, ok := r["D"].(string); ok {
					var deg int
					fmt.Sscanf(d, "%d", &deg)
					outDegMap[s] = deg
				}
			}
		}
	}

	// Query cluster facts
	clusterMap := make(map[string]string)
	if clusterResults, err := mebpkg.Query(ctx, analyticalStore, common.GetNamedQuery("cluster_short")); err == nil {
		for _, r := range clusterResults {
			if s, ok := r["S"].(string); ok {
				if c, ok := r["C"].(string); ok {
					clusterMap[s] = c
				}
			}
		}
	}

	// Isolated nodes: degree <= 1
	allSymbols := make(map[string]bool)
	for s := range inDegMap {
		allSymbols[s] = true
	}
	for s := range outDegMap {
		allSymbols[s] = true
	}
	for sym := range allSymbols {
		in := inDegMap[sym]
		out := outDegMap[sym]
		if in+out <= 1 {
			severity := "low"
			if in+out == 0 {
				severity = "medium"
			}
			resp.IsolatedNodes = append(resp.IsolatedNodes, KnowledgeGapItem{
				Symbol:   sym,
				GapType:  "isolated",
				Severity: severity,
				Detail:   fmt.Sprintf("Degree: %d (in=%d, out=%d)", in+out, in, out),
				Degree:   in + out,
			})
		}
	}

	// Untested hotspots: degree >= 5 and not a test symbol
	if testResults, err := mebpkg.Query(ctx, analyticalStore, common.GetNamedQuery("test_symbol")); err == nil {
		testSymbols := make(map[string]bool)
		for _, r := range testResults {
			if s, ok := r["S"].(string); ok {
				testSymbols[s] = true
			}
		}
		for sym := range allSymbols {
			if testSymbols[sym] {
				continue
			}
			in := inDegMap[sym]
			out := outDegMap[sym]
			if in+out >= 5 {
				resp.UntestedHotspots = append(resp.UntestedHotspots, KnowledgeGapItem{
					Symbol:   sym,
					GapType:  "untested_hotspot",
					Severity: "high",
					Detail:   fmt.Sprintf("Degree: %d (in=%d, out=%d) - no test coverage", in+out, in, out),
					Degree:   in + out,
				})
			}
		}
	}

	// Count cluster sizes
	clusterSizes := make(map[string]int)
	for _, c := range clusterMap {
		clusterSizes[c]++
	}

	// Thin communities: cluster size < 3
	thinClusters := make(map[string]bool)
	for c, size := range clusterSizes {
		if size > 0 && size < 3 {
			thinClusters[c] = true
		}
	}
	for sym, c := range clusterMap {
		if thinClusters[c] {
			resp.ThinCommunities = append(resp.ThinCommunities, KnowledgeGapItem{
				Symbol:   sym,
				GapType:  "thin_community",
				Severity: "low",
				Detail:   fmt.Sprintf("Cluster %s has only %d member(s)", c, clusterSizes[c]),
			})
		}
	}

	// Single-file clusters: all members in same file
	fileMap := make(map[string]string)
	if fileResults, err := mebpkg.Query(ctx, analyticalStore, common.GetNamedQuery("in_file")); err == nil {
		for _, r := range fileResults {
			if s, ok := r["S"].(string); ok {
				if f, ok := r["F"].(string); ok {
					fileMap[s] = f
				}
			}
		}
	}
	clusterFileGroups := make(map[string]map[string]bool)
	for sym, c := range clusterMap {
		f := fileMap[sym]
		if f == "" {
			continue
		}
		key := c + "|" + f
		if clusterFileGroups[key] == nil {
			clusterFileGroups[key] = make(map[string]bool)
		}
		clusterFileGroups[key][sym] = true
	}
	for key, members := range clusterFileGroups {
		if len(members) >= 3 {
			parts := strings.Split(key, "|")
			clusterID := parts[0]
			filePath := parts[1]
			for sym := range members {
				resp.SingleFileClusters = append(resp.SingleFileClusters, KnowledgeGapItem{
					Symbol:   sym,
					GapType:  "single_file_community",
					Severity: "medium",
					Detail:   fmt.Sprintf("Cluster %s has %d symbols all in %s", clusterID, len(members), filePath),
				})
			}
		}
	}

	resp.TotalCount = len(resp.IsolatedNodes) + len(resp.UntestedHotspots) + len(resp.ThinCommunities) + len(resp.SingleFileClusters)
	return resp, nil
}
