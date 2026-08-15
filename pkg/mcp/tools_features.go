package mcp

import (
	"context"
	"strings"

	"github.com/duynguyendang/gca/pkg/compliance"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/mark3labs/mcp-go/mcp"
)

// --- F2: health-over-time trends ---

func (s *Server) handleGetTrends(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	metric, _ := args["metric"].(string)
	if metric == "" {
		metric = "health"
	}
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)

	s.mu.Lock()
	defer s.mu.Unlock()

	resp, err := s.trends.ListTrends(ctx, project, metric, from, to)
	if err != nil {
		return errorResult("failed to load trends: %v", err), nil
	}
	return jsonResult(resp), nil
}

// --- F3: PR blast-radius report ---

func (s *Server) handleGetImpactReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	diff, err := requireString(args, "diff")
	if err != nil {
		return errorResult("%v", err), nil
	}
	baseCommit, _ := args["base_commit"].(string)
	headCommit, _ := args["head_commit"].(string)
	failIfNewSmells := optionalInt(args, "fail_if_new_smells", 0)

	s.mu.Lock()
	defer s.mu.Unlock()

	report, err := s.impact.Generate(ctx, project, diff)
	if err != nil {
		return errorResult("failed to generate impact report: %v", err), nil
	}
	report.BaseCommit = baseCommit
	report.HeadCommit = headCommit
	if failIfNewSmells > 0 {
		report.NewSmellThreshold = failIfNewSmells
		report.Blocked = report.SmellsNewCount() > failIfNewSmells
	}
	return jsonResult(report), nil
}

// --- F4: vulnerabilities + SBOM ---

func (s *Server) handleListVulnerabilities(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	severityArg, _ := args["severity"].(string)
	severityFilter := map[string]bool{}
	for _, sv := range strings.Split(severityArg, ",") {
		if sv = strings.TrimSpace(sv); sv != "" {
			severityFilter[strings.ToLower(sv)] = true
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	analytical, err := s.mgr.GetAnalyticalStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	severityByID := map[string]string{}
	for fact := range analytical.ScanContext(ctx, "", compliance.PredicateVulnSeverity, "") {
		if id := fact.Subject; id != "" {
			if sev, ok := fact.Object.(string); ok {
				severityByID[id] = sev
			}
		}
	}
	summaryByID := map[string]string{}
	for fact := range analytical.ScanContext(ctx, "", compliance.PredicateVulnSummary, "") {
		if id := fact.Subject; id != "" {
			if sum, ok := fact.Object.(string); ok {
				summaryByID[id] = sum
			}
		}
	}

	type vuln struct {
		Package      string `json:"package"`
		AdvisoryID   string `json:"advisory_id"`
		Severity     string `json:"severity"`
		Summary      string `json:"summary"`
		SnapshotDate string `json:"snapshot_date"`
	}
	snapDate := ""
	if snap, err := compliance.LoadSnapshot(compliance.DefaultSnapshotPath); err == nil {
		snapDate = snap.SnapshotDate()
	}

	var out []vuln
	for fact := range analytical.ScanContext(ctx, "", compliance.PredicateHasVulnerability, "") {
		if fact.Subject == "" {
			continue
		}
		advID, ok := fact.Object.(string)
		if !ok || advID == "" {
			continue
		}
		sev := severityByID[advID]
		if len(severityFilter) > 0 && !severityFilter[strings.ToLower(sev)] {
			continue
		}
		out = append(out, vuln{
			Package:      fact.Subject,
			AdvisoryID:   advID,
			Severity:     sev,
			Summary:      summaryByID[advID],
			SnapshotDate: snapDate,
		})
	}
	return jsonResult(map[string]any{
		"project_id":      project,
		"vulnerabilities": out,
		"snapshot_date":   snapDate,
		"total":           len(out),
	}), nil
}

func (s *Server) handleGetSBOM(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	format, _ := args["format"].(string)

	s.mu.Lock()
	defer s.mu.Unlock()

	source, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	inv, err := compliance.CollectInventory(ctx, source)
	if err != nil {
		return errorResult("failed to collect inventory: %v", err), nil
	}
	if strings.EqualFold(format, "cyclonedx") {
		components := make([]map[string]any, 0, inv.PackageCount)
		for _, dep := range inv.Dependencies {
			components = append(components, map[string]any{
				"type":    "library",
				"name":    dep.Name,
				"version": dep.Version,
				"purl":    "pkg:generic/" + dep.Name,
			})
		}
		return jsonResult(map[string]any{
			"bomFormat":   "CycloneDX",
			"specVersion": "1.5",
			"version":     1,
			"metadata":    map[string]any{"component": map[string]any{"type": "application", "name": project}},
			"components":  components,
		}), nil
	}
	return jsonResult(map[string]any{
		"project_id":    project,
		"format":        "json",
		"package_count": inv.PackageCount,
		"dependencies":  inv.Dependencies,
	}), nil
}

// --- F5: architecture report ---

func (s *Server) handleGetArchitectureReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	sectionsArg, _ := args["sections"].(string)
	includeAI, _ := args["include_ai"].(bool)

	var sections []string
	for _, part := range strings.Split(sectionsArg, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			sections = append(sections, trimmed)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	md, err := s.reports.GenerateMarkdown(ctx, service.ReportOptions{
		ProjectID: project,
		Sections:  sections,
		IncludeAI: includeAI,
	})
	if err != nil {
		return errorResult("failed to generate architecture report: %v", err), nil
	}
	return mcp.NewToolResultText(md), nil
}
