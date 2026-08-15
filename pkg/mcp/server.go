package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/agent"
	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ephemeral"
	"github.com/duynguyendang/gca/pkg/ingest"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/okf"
	"github.com/duynguyendang/gca/pkg/registry"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/duynguyendang/meb"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Default limits for tool results.
const (
	defaultSearchLimit = 10
	defaultQueryLimit  = 50
	defaultScanLimit   = 50
	defaultSemanticK   = 10
)

// Options configures the MCP server.
type Options struct {
	Manager       *manager.StoreManager
	AIService     *ai.AIService
	SmellRegistry *registry.SmellRegistry
}

// Server is a manager-based MCP server. It multiplexes the Source and
// Analytical stores per project and serializes store access, because both
// partitions share a single *meb.MEBStore whose TopicID is mutable.
type Server struct {
	mgr        *manager.StoreManager
	aiSvc      *ai.AIService
	smellReg   *registry.SmellRegistry
	graph      *service.GraphService
	clustering *service.ClusteringService
	trends     *service.TrendService
	impact     *service.ImpactReportService
	reports    *service.ReportService
	readOnly   bool
	mu         sync.Mutex
}

// New builds a mark3labs MCPServer with all resources and tools registered.
func New(opts Options) *mcpserver.MCPServer {
	smellReg := opts.SmellRegistry
	if smellReg == nil {
		smellReg = registry.NewSmellRegistry(opts.Manager)
	}

	// NewImpactReportService needs an ephemeral store; create a shared one.
	es := ephemeral.NewEphemeralStore(0)

	s := &Server{
		mgr:        opts.Manager,
		aiSvc:      opts.AIService,
		smellReg:   smellReg,
		graph:      service.NewGraphService(opts.Manager),
		clustering: service.NewClusteringService(),
		trends:     service.NewTrendService(opts.Manager),
		reports:    service.NewReportService(opts.Manager, service.NilNarrative(opts.AIService)),
		readOnly:   opts.Manager.ReadOnly(),
	}
	s.impact = service.NewImpactReportService(es, opts.Manager, service.NilNarrative(opts.AIService))

	ms := mcpserver.NewMCPServer(
		"GCA-Backend",
		"0.2.0",
		mcpserver.WithResourceCapabilities(true, true),
		mcpserver.WithLogging(),
	)

	s.registerResources(ms)
	s.registerTools(ms)
	return ms
}

// --- Argument helpers ---

func requireProject(args map[string]any) (string, error) {
	p, ok := args["project"].(string)
	if !ok || p == "" {
		return "", fmt.Errorf("project argument required")
	}
	return p, nil
}

func requireString(args map[string]any, name string) (string, error) {
	v, ok := args[name].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("%s argument required", name)
	}
	return v, nil
}

func optionalInt(args map[string]any, name string, defaultVal int) int {
	if v, ok := args[name].(float64); ok {
		return int(v)
	}
	return defaultVal
}

// --- Result helpers ---

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("json marshal failed: %v", err))
	}
	return mcp.NewToolResultText(string(b))
}

// errorResult builds a JSON error envelope with a code inferred from msg.
// It is the single construction point for all tool error paths so clients
// always receive a consistent {error, code, details} shape (UC15).
func errorResult(format string, args ...any) *mcp.CallToolResult {
	msg := fmt.Sprintf(format, args...)
	return toolError(classifyError(msg), msg)
}

// --- Resource registration ---

func (s *Server) registerResources(ms *mcpserver.MCPServer) {
	ms.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"gca://projects/{project}/summary",
			"Project Graph Summary",
			mcp.WithTemplateDescription("Summary statistics of a project's graph database"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		s.handleGraphSummary,
	)
	ms.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"gca://projects/{project}/files/{+path}",
			"File Content",
			mcp.WithTemplateDescription("Content of a source file in a project"),
			mcp.WithTemplateMIMEType("text/plain"),
		),
		s.handleFileContent,
	)
	ms.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"gca://projects/{project}/smells",
			"Project Smells",
			mcp.WithTemplateDescription("Detected code smells for a project from the Analytical Store"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		s.handleSmellsResource,
	)
	ms.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"gca://projects/{project}/health",
			"Project Health",
			mcp.WithTemplateDescription("Health overview: debt score, smell counts, security issues for a project"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		s.handleHealthResource,
	)
	ms.AddResource(
		mcp.NewResource(
			"gca://schema/conventions",
			"Schema Conventions",
			mcp.WithResourceDescription("Architectural schema and naming conventions for GCA"),
			mcp.WithMIMEType("text/markdown"),
		),
		s.handleSchemaConventions,
	)
}

// --- Tool registration ---

func (s *Server) registerTools(ms *mcpserver.MCPServer) {
	// Project management
	ms.AddTool(
		mcp.NewTool("list_projects",
			mcp.WithDescription("List available projects in the GCA data directory.")),
		s.handleListProjects,
	)

	// Graph query tools (Source Store)
	ms.AddTool(
		mcp.NewTool("search_nodes",
			mcp.WithDescription("Search for nodes (symbols, files) in a project's graph."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("query", mcp.Required(), mcp.Description("The search query string")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10)"))),
		s.handleSearchNodes,
	)
	ms.AddTool(
		mcp.NewTool("get_outgoing_edges",
			mcp.WithDescription("Get outgoing edges (dependencies/calls) from a specific node."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("node_id", mcp.Required(), mcp.Description("The ID of the source node"))),
		s.handleGetOutgoingEdges,
	)
	ms.AddTool(
		mcp.NewTool("get_incoming_edges",
			mcp.WithDescription("Get incoming edges (consumers/callers) to a specific node."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("node_id", mcp.Required(), mcp.Description("The ID of the target node"))),
		s.handleGetIncomingEdges,
	)
	ms.AddTool(
		mcp.NewTool("scan_facts",
			mcp.WithDescription("Scan raw source-store facts (Subject, Predicate, Object). Empty fields act as wildcards. Paginated via limit/cursor."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("subject", mcp.Description("Subject filter")),
			mcp.WithString("predicate", mcp.Description("Predicate filter")),
			mcp.WithString("object", mcp.Description("Object filter")),
			mcp.WithNumber("limit", mcp.Description("Max facts per page (default 50)")),
			mcp.WithString("cursor", mcp.Description("Opaque pagination token from a previous scan_facts response"))),
		s.handleScanFacts,
	)
	ms.AddTool(
		mcp.NewTool("get_node_metadata",
			mcp.WithDescription("Get detailed metadata for a node (kind, package, tags, etc.)."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("node_id", mcp.Required(), mcp.Description("The ID of the node"))),
		s.handleGetNodeMetadata,
	)
	ms.AddTool(
		mcp.NewTool("trace_impact_path",
			mcp.WithDescription("Trace the shortest dependency path between two nodes."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("start_node", mcp.Required(), mcp.Description("Start node ID")),
			mcp.WithString("end_node", mcp.Required(), mcp.Description("End node ID"))),
		s.handleTraceImpactPath,
	)
	ms.AddTool(
		mcp.NewTool("get_clusters",
			mcp.WithDescription("Detect clusters/communities in a project's graph using the Leiden algorithm."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID"))),
		s.handleGetClusters,
	)
	ms.AddTool(
		mcp.NewTool("datalog_query",
			mcp.WithDescription("Execute a raw Datalog query (triples(...) atoms) against a project's Source Store. Paginated via limit/cursor."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("query", mcp.Required(), mcp.Description("Datalog query, e.g. triples(Subject, \"calls\", \"target\")")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 50)")),
			mcp.WithString("cursor", mcp.Description("Opaque pagination token from a previous datalog_query response"))),
		s.handleDatalogQuery,
	)

	// Analysis tools (Analytical Store)
	ms.AddTool(
		mcp.NewTool("get_health_summary",
			mcp.WithDescription("Get the per-file health summary for a project (health debt, smells, security issues)."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID"))),
		s.handleHealthSummary,
	)
	ms.AddTool(
		mcp.NewTool("list_smells",
			mcp.WithDescription("List detected code smells for a project from the Analytical Store. Optional severity/type filters."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("severity", mcp.Description("Filter by severity (low/medium/high)")),
			mcp.WithString("type", mcp.Description("Filter by smell type (e.g. god_file, missing_error_check, okf_orphan)"))),
		s.handleListSmells,
	)
	// Analysis tools (F1+ data)
	ms.AddTool(
		mcp.NewTool("list_high_complexity",
			mcp.WithDescription("List functions/methods with cyclomatic complexity above a threshold. Reads has_complexity facts from the Source Store."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithNumber("threshold", mcp.Description("Minimum complexity (default 15)")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 50)"))),
		s.handleListHighComplexity,
	)
	ms.AddTool(
		mcp.NewTool("list_duplicate_groups",
			mcp.WithDescription("List groups of functions with identical normalized bodies (same has_body_hash). Reads from Source Store."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithNumber("limit", mcp.Description("Max groups (default 50)"))),
		s.handleListDuplicateGroups,
	)
	ms.AddTool(
		mcp.NewTool("project_health_overview",
			mcp.WithDescription("Comprehensive health overview: dead code, high complexity, duplicates, hubs, entry points, and overall score."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID"))),
		s.handleProjectHealthOverview,
	)

	// Feature Wave F2-F5 tools
	ms.AddTool(
		mcp.NewTool("get_trends",
			mcp.WithDescription("Get a health-over-time series for a project. Metrics: health, debt, smell_count, dead_code, complexity, duplicate. Optional RFC3339 from/to filters."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("metric", mcp.Description("Metric (default health): health, debt, smell_count, dead_code, complexity, duplicate")),
			mcp.WithString("from", mcp.Description("RFC3339 start time filter")),
			mcp.WithString("to", mcp.Description("RFC3339 end time filter"))),
		s.handleGetTrends,
	)
	ms.AddTool(
		mcp.NewTool("get_impact_report",
			mcp.WithDescription("Produce a PR blast-radius report for a unified diff: touched files, hubs hit, entry points affected, smells, reachable callers."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("diff", mcp.Required(), mcp.Description("Unified git diff")),
			mcp.WithString("base_commit", mcp.Description("Optional base commit SHA")),
			mcp.WithString("head_commit", mcp.Description("Optional head commit SHA")),
			mcp.WithNumber("fail_if_new_smells", mcp.Description("Optional gate: flag blocked when new smells exceed N"))),
		s.handleGetImpactReport,
	)
	ms.AddTool(
		mcp.NewTool("list_vulnerabilities",
			mcp.WithDescription("List known-vulnerable dependencies from the offline advisory snapshot (F4). Optional severity filter (comma-separated)."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("severity", mcp.Description("Comma-separated severities to filter, e.g. high,medium"))),
		s.handleListVulnerabilities,
	)
	ms.AddTool(
		mcp.NewTool("get_sbom",
			mcp.WithDescription("Return the project's software bill of materials: deduplicated dependency inventory from imports facts (F4)."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("format", mcp.Description("Output format (default json; cyclonedx supported)"))),
		s.handleGetSBOM,
	)
	ms.AddTool(
		mcp.NewTool("get_architecture_report",
			mcp.WithDescription("Generate a markdown architecture report for a project: overview, entry points, hubs, smells, clusters, call flows, OKF (F5)."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("sections", mcp.Description("Comma-separated sections (overview,entry_points,hubs,smells,clusters,call_flows,okf); empty = all")),
			mcp.WithBoolean("include_ai", mcp.Description("Append an AI narrative summary (requires AI service)"))),
		s.handleGetArchitectureReport,
	)

	// Semantic search + agent (require AI service)
	if s.aiSvc != nil {
		ms.AddTool(
			mcp.NewTool("semantic_search",
				mcp.WithDescription("Vector similarity search over a project's embedded symbols. Errors if the project has no embeddings."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
				mcp.WithString("query", mcp.Required(), mcp.Description("Natural language query")),
				mcp.WithNumber("k", mcp.Description("Number of results (default 10)"))),
			s.handleSemanticSearch,
		)
		ms.AddTool(
			mcp.NewTool("agent_execute",
				mcp.WithDescription("Run the GCA multi-step reasoning agent: plan analysis steps, execute datalog queries, and synthesize a narrative."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
				mcp.WithString("query", mcp.Required(), mcp.Description("Natural language analysis request"))),
			s.handleAgentExecute,
		)
	}

	// OKF ingest/export
	ms.AddTool(
		mcp.NewTool("okf_export",
			mcp.WithDescription("Export a project's OKF concepts to a bundle directory as markdown files with YAML frontmatter."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("output_dir", mcp.Required(), mcp.Description("Absolute path to write the bundle"))),
		s.handleOKFExport,
	)
	// okf_ingest mutates the store; only exposed in writable mode.
	if !s.readOnly {
		ms.AddTool(
			mcp.NewTool("okf_ingest",
				mcp.WithDescription("Ingest an OKF v0.1 bundle directory (markdown + YAML frontmatter) as knowledge concepts. Requires a writable server."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
				mcp.WithString("bundle_dir", mcp.Required(), mcp.Description("Absolute path to the OKF bundle directory"))),
			s.handleOKFIngest,
		)
	}

	// Incremental ingestion
	ms.AddTool(
		mcp.NewTool("ingest_status",
			mcp.WithDescription("Report a project's ingest state: last ingested commit, schema version, file count. Optionally compare against a git working tree."),
			mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
			mcp.WithString("source_dir", mcp.Description("Absolute path to the project source for git comparison"))),
		s.handleIngestStatus,
	)
	// ingest_incremental mutates the store; only exposed in writable mode.
	if !s.readOnly {
		ms.AddTool(
			mcp.NewTool("ingest_incremental",
				mcp.WithDescription("Re-ingest only files changed since the last ingest. Requires a writable server and an absolute source_dir."),
				mcp.WithString("project", mcp.Required(), mcp.Description("Project ID")),
				mcp.WithString("source_dir", mcp.Required(), mcp.Description("Absolute path to the project source directory")),
				mcp.WithString("from_commit", mcp.Description("Start commit for git-based incremental (default: last ingested commit)")),
				mcp.WithString("to_commit", mcp.Description("End commit for git-based incremental (default: working tree)")),
				mcp.WithBoolean("skip_embed", mcp.Description("Skip embedding generation"))),
			s.handleIngestIncremental,
		)
	}
}

// --- Resource Handlers ---

func (s *Server) handleGraphSummary(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	project := templateParam(request, "project")
	if project == "" {
		return nil, fmt.Errorf("missing project in resource URI")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", project)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(mustJSON(map[string]any{"project": project, "fact_count": store.Count()})),
		},
	}, nil
}

func (s *Server) handleFileContent(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	project := templateParam(request, "project")
	path := templateParam(request, "path")
	if project == "" || path == "" {
		return nil, fmt.Errorf("invalid resource URI")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", project)
	}
	doc, err := store.GetContentByKey(path)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/plain",
			Text:     string(doc),
		},
	}, nil
}

func (s *Server) handleSchemaConventions(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/markdown",
			Text:     schemaConventionsMarkdown,
		},
	}, nil
}

func (s *Server) handleSmellsResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	project := templateParam(request, "project")
	if project == "" {
		return nil, fmt.Errorf("invalid resource URI")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	analytical, err := s.mgr.GetAnalyticalStore(project)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", project)
	}

	typeBySubject := queryMultiMap(ctx, analytical, common.GetNamedQuery("smell_type"), "Subject", "Type")
	sevBySubject := queryMultiMap(ctx, analytical, common.GetNamedQuery("smell_severity"), "Subject", "Severity")

	var entries []smellEntry
	for subject, types := range typeBySubject {
		sevs := sevBySubject[subject]
		for i, typ := range types {
			sev := ""
			if i < len(sevs) {
				sev = sevs[i]
			}
			entries = append(entries, smellEntry{Subject: subject, Type: typ, Severity: sev})
		}
	}

	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal smells: %v", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(b),
		},
	}, nil
}

func (s *Server) handleHealthResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	project := templateParam(request, "project")
	if project == "" {
		return nil, fmt.Errorf("invalid resource URI")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	analytical, err := s.mgr.GetAnalyticalStore(project)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", project)
	}

	fileDebt := queryDebtMap(ctx, analytical)
	fileSmells := querySmellMap(ctx, analytical)
	fileHubScore := queryHubScoreMap(ctx, analytical)

	files, totalArchDebt, totalSecurity := s.computeFileHealth(fileDebt, fileSmells, fileHubScore)
	overallScore := 100 - totalArchDebt/10
	if overallScore < 0 {
		overallScore = 0
	}

	b, err := json.MarshalIndent(map[string]any{
		"overall_score":         overallScore,
		"total_security_alerts": totalSecurity,
		"total_arch_debt":       totalArchDebt,
		"files":                 files,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal health: %v", err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(b),
		},
	}, nil
}

// --- Tool Handlers ---

func (s *Server) handleListProjects(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projects, err := s.mgr.ListProjects()
	if err != nil {
		return errorResult("failed to list projects: %v", err), nil
	}
	return jsonResult(projects), nil
}

func (s *Server) handleSearchNodes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	query, err := requireString(args, "query")
	if err != nil {
		return errorResult("%v", err), nil
	}
	limit := optionalInt(args, "limit", defaultSearchLimit)

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	var results []string
	lowerQuery := strings.ToLower(query)
	for fact := range store.Scan("", config.PredicateDefines, "") {
		if obj, ok := fact.Object.(string); ok {
			if strings.Contains(strings.ToLower(obj), lowerQuery) {
				results = append(results, obj)
				if len(results) >= limit {
					break
				}
			}
		}
	}
	return mcp.NewToolResultText(strings.Join(results, "\n")), nil
}

func (s *Server) handleGetOutgoingEdges(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	nodeID, err := requireString(args, "node_id")
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	var lines []string
	for fact := range store.Scan(nodeID, "", "") {
		lines = append(lines, fmt.Sprintf("%s -> %s", fact.Predicate, fact.Object))
	}
	if len(lines) == 0 {
		return mcp.NewToolResultText("No outgoing edges found."), nil
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func (s *Server) handleGetIncomingEdges(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	nodeID, err := requireString(args, "node_id")
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	var lines []string
	for fact := range store.Scan("", "", nodeID) {
		lines = append(lines, fmt.Sprintf("%s -> %s", fact.Subject, fact.Predicate))
	}
	if len(lines) == 0 {
		return mcp.NewToolResultText("No incoming edges found."), nil
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func (s *Server) handleScanFacts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	subj, _ := args["subject"].(string)
	pred, _ := args["predicate"].(string)
	obj, _ := args["object"].(string)
	limit := optionalInt(args, "limit", defaultScanLimit)
	cursor, _ := args["cursor"].(string)
	start := decodeCursor(cursor)

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	var lines []string
	for fact := range store.Scan(subj, pred, obj) {
		lines = append(lines, fmt.Sprintf("%s --[%s]--> %s", fact.Subject, fact.Predicate, fact.Object))
	}
	page, next := slicePage(lines, start, limit)
	return jsonResult(map[string]any{
		"facts":       page,
		"count":       len(page),
		"next_cursor": next,
	}), nil
}

func (s *Server) handleGetNodeMetadata(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	nodeID, err := requireString(args, "node_id")
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	hydrated, err := s.graph.HydrateShallow(ctx, store, []string{nodeID})
	if err != nil {
		return errorResult("hydration failed: %v", err), nil
	}
	if len(hydrated) == 0 {
		return mcp.NewToolResultText("{}"), nil
	}
	h := hydrated[0]
	return jsonResult(map[string]any{
		"id":             h.ID,
		"kind":           h.Kind,
		"metadata":       h.Metadata,
		"children_count": len(h.Children),
	}), nil
}

func (s *Server) handleTraceImpactPath(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	startNode, err := requireString(args, "start_node")
	if err != nil {
		return errorResult("%v", err), nil
	}
	endNode, err := requireString(args, "end_node")
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	graph, err := s.graph.FindShortestPath(ctx, project, startNode, endNode)
	if err != nil {
		return errorResult("pathfinding failed: %v", err), nil
	}
	return jsonResult(graph), nil
}

func (s *Server) handleGetClusters(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	nodes, links := buildStructuralGraph(store)
	result := s.clustering.DetectCommunitiesLeiden(nodes, links)
	return jsonResult(result), nil
}

func (s *Server) handleDatalogQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	query, err := requireString(args, "query")
	if err != nil {
		return errorResult("%v", err), nil
	}
	limit := optionalInt(args, "limit", defaultQueryLimit)
	cursor, _ := args["cursor"].(string)
	start := decodeCursor(cursor)

	s.mu.Lock()
	defer s.mu.Unlock()

	results, err := s.graph.ExecuteQuery(ctx, project, query)
	if err != nil {
		return errorResult("query failed: %v", err), nil
	}
	end := len(results)
	next := ""
	if start >= end {
		results = results[:0]
	} else {
		if limit > 0 && start+limit < end {
			end = start + limit
			next = encodeCursor(end)
		}
		results = results[start:end]
	}
	return jsonResult(map[string]any{
		"results":     results,
		"count":       len(results),
		"next_cursor": next,
	}), nil
}

// --- Health summary ---

type fileHealth struct {
	FileName       string   `json:"file_name"`
	TotalDebtScore int      `json:"total_debt_score"`
	SecurityIssues int      `json:"security_issues"`
	ArchSmells     []string `json:"arch_smells"`
}

func (s *Server) handleHealthSummary(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	analytical, err := s.mgr.GetAnalyticalStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	fileDebt := queryDebtMap(ctx, analytical)
	fileSmells := querySmellMap(ctx, analytical)
	fileHubScore := queryHubScoreMap(ctx, analytical)

	files, totalArchDebt, totalSecurity := s.computeFileHealth(fileDebt, fileSmells, fileHubScore)
	overallScore := 100 - totalArchDebt/10
	if overallScore < 0 {
		overallScore = 0
	}

	return jsonResult(map[string]any{
		"overall_score":         overallScore,
		"total_security_alerts": totalSecurity,
		"total_arch_debt":       totalArchDebt,
		"files":                 files,
	}), nil
}

func queryDebtMap(ctx context.Context, store *meb.MEBStore) map[string]int {
	result := make(map[string]int)
	rows, err := gcamdb.Query(ctx, store, common.GetNamedQuery("health_debt"))
	if err != nil {
		return result
	}
	for _, r := range rows {
		subject, _ := r["Subject"].(string)
		debtStr, _ := r["Debt"].(string)
		if subject == "" || debtStr == "" {
			continue
		}
		if debt, err := strconv.Atoi(debtStr); err == nil {
			result[subject] = debt
		}
	}
	return result
}

func querySmellMap(ctx context.Context, store *meb.MEBStore) map[string][]string {
	result := make(map[string][]string)
	rows, err := gcamdb.Query(ctx, store, common.GetNamedQuery("smell_type"))
	if err != nil {
		return result
	}
	for _, r := range rows {
		subject, _ := r["Subject"].(string)
		smellType, _ := r["Type"].(string)
		if subject != "" && smellType != "" {
			result[subject] = append(result[subject], smellType)
		}
	}
	return result
}

func queryHubScoreMap(ctx context.Context, store *meb.MEBStore) map[string]int {
	result := make(map[string]int)
	rows, err := gcamdb.Query(ctx, store, common.GetNamedQuery("hub_score"))
	if err != nil {
		return result
	}
	for _, r := range rows {
		subject, _ := r["Subject"].(string)
		scoreStr, _ := r["Score"].(string)
		if subject == "" {
			continue
		}
		if score, err := strconv.Atoi(scoreStr); err == nil {
			result[subject] = score
		}
	}
	return result
}

func (s *Server) computeFileHealth(
	fileDebt map[string]int,
	fileSmells map[string][]string,
	fileHubScore map[string]int,
) ([]fileHealth, int, int) {
	var files []fileHealth
	totalArchDebt := 0
	totalSecurity := 0

	for file, smells := range fileSmells {
		debt, secIssues, archSmells := s.scoreFile(file, smells, fileDebt, fileHubScore)
		files = append(files, fileHealth{
			FileName:       file,
			TotalDebtScore: debt,
			SecurityIssues: secIssues,
			ArchSmells:     archSmells,
		})
		totalArchDebt += debt
		totalSecurity += secIssues
	}
	for file, debt := range fileDebt {
		if _, exists := fileSmells[file]; !exists {
			files = append(files, fileHealth{FileName: file, TotalDebtScore: debt})
			totalArchDebt += debt
		}
	}
	return files, totalArchDebt, totalSecurity
}

func (s *Server) scoreFile(
	file string,
	smells []string,
	fileDebt map[string]int,
	fileHubScore map[string]int,
) (debt int, secIssues int, archSmells []string) {
	for _, smell := range smells {
		if s.smellReg.IsSecurity(smell) {
			secIssues++
		} else {
			archSmells = append(archSmells, smell)
		}
	}
	if pre, ok := fileDebt[file]; ok {
		return pre, secIssues, archSmells
	}
	for _, smell := range smells {
		if w, ok := s.smellReg.Weight(smell); ok {
			debt += w
		} else {
			debt += s.smellReg.DefaultWeight()
		}
	}
	if hub, ok := fileHubScore[file]; ok {
		debt += hub
	}
	return debt, secIssues, archSmells
}

// --- Smells ---

type smellEntry struct {
	Subject  string `json:"subject"`
	Type     string `json:"type"`
	Severity string `json:"severity,omitempty"`
}

func (s *Server) handleListSmells(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	filterSev, _ := args["severity"].(string)
	filterType, _ := args["type"].(string)

	s.mu.Lock()
	defer s.mu.Unlock()

	analytical, err := s.mgr.GetAnalyticalStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	typeBySubject := queryMultiMap(ctx, analytical, common.GetNamedQuery("smell_type"), "Subject", "Type")
	sevBySubject := queryMultiMap(ctx, analytical, common.GetNamedQuery("smell_severity"), "Subject", "Severity")

	var entries []smellEntry
	for subject, types := range typeBySubject {
		sevs := sevBySubject[subject]
		for i, typ := range types {
			sev := ""
			if i < len(sevs) {
				sev = sevs[i]
			}
			if filterSev != "" && sev != filterSev {
				continue
			}
			if filterType != "" && typ != filterType {
				continue
			}
			entries = append(entries, smellEntry{Subject: subject, Type: typ, Severity: sev})
		}
	}
	return jsonResult(entries), nil
}

// queryMultiMap runs a named query and returns a map of subject -> []value.
func queryMultiMap(ctx context.Context, store *meb.MEBStore, query, subjectKey, valueKey string) map[string][]string {
	result := make(map[string][]string)
	rows, err := gcamdb.Query(ctx, store, query)
	if err != nil {
		return result
	}
	for _, r := range rows {
		subject, _ := r[subjectKey].(string)
		value, _ := r[valueKey].(string)
		if subject != "" && value != "" {
			result[subject] = append(result[subject], value)
		}
	}
	return result
}

// --- Semantic search ---

func (s *Server) handleSemanticSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.aiSvc == nil {
		return errorResult("semantic search unavailable: AI service not initialized"), nil
	}
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	query, err := requireString(args, "query")
	if err != nil {
		return errorResult("%v", err), nil
	}
	k := optionalInt(args, "k", defaultSemanticK)

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}
	if store.DebugInfo().NumVectors <= 0 {
		return errorResult("project %s has no embeddings; re-ingest with embeddings to use semantic search", project), nil
	}

	results, err := s.graph.SemanticSearch(ctx, project, query, k, s.aiSvc)
	if err != nil {
		return errorResult("semantic search failed: %v", err), nil
	}
	return jsonResult(results), nil
}

// --- Agent ---

func (s *Server) handleAgentExecute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.aiSvc == nil {
		return errorResult("agent unavailable: AI service not initialized"), nil
	}
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	query, err := requireString(args, "query")
	if err != nil {
		return errorResult("%v", err), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	modelAdapter := ai.NewAIServiceModelAdapter(s.aiSvc)
	orch := agent.NewOrchestrator(modelAdapter, store)
	predicateNames := []string{
		config.PredicateDefines,
		config.PredicateCalls,
		config.PredicateImports,
		config.PredicateHasDoc,
		config.PredicateInPackage,
		config.PredicateHasRole,
		config.PredicateHasTag,
		config.PredicateKind,
	}

	session, err := orch.Run(ctx, project, query, predicateNames)
	if err != nil {
		return errorResult("agent execution failed: %v", err), nil
	}
	return jsonResult(agent.AgentResponse{
		SessionID: session.ID,
		Steps:     session.Steps,
		Narrative: session.Narrative,
	}), nil
}

// --- OKF ---

func (s *Server) handleOKFIngest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.mgr.ReadOnly() {
		return errorResult("server is read-only; start with --writable (or GCA_WRITABLE=true) to ingest OKF bundles"), nil
	}
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	bundleDir, err := requireString(args, "bundle_dir")
	if err != nil {
		return errorResult("%v", err), nil
	}
	if !filepath.IsAbs(bundleDir) {
		return errorResult("bundle_dir must be an absolute path"), nil
	}
	if info, statErr := os.Stat(bundleDir); statErr != nil || !info.IsDir() {
		return errorResult("bundle_dir does not exist or is not a directory: %s", bundleDir), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.mgr.EnsureProject(project); err != nil {
		return errorResult("failed to ensure project: %v", err), nil
	}
	report, err := okf.Ingest(ctx, s.mgr, s.mgr.BaseDir(), okf.IngestOptions{
		ProjectID: project,
		BundleDir: bundleDir,
	})
	if err != nil {
		return errorResult("okf ingest failed: %v", err), nil
	}
	return jsonResult(report), nil
}

func (s *Server) handleOKFExport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	outputDir, err := requireString(args, "output_dir")
	if err != nil {
		return errorResult("%v", err), nil
	}
	if !filepath.IsAbs(outputDir) {
		return errorResult("output_dir must be an absolute path"), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	report, err := okf.Export(ctx, s.mgr, okf.ExportOptions{
		ProjectID: project,
		OutputDir: outputDir,
	})
	if err != nil {
		return errorResult("okf export failed: %v", err), nil
	}
	return jsonResult(report), nil
}

// --- Ingest ---

func (s *Server) handleIngestStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	sourceDir, _ := args["source_dir"].(string)

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetSourceStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	status := map[string]any{
		"project":                project,
		"last_commit_sha":        ingest.GetLastCommitSHA(store),
		"schema_version":         ingest.GetSchemaVersion(store),
		"current_schema_version": config.SchemaVersion,
		"version_mismatch":       false,
		"fact_count":             store.Count(),
	}

	storedVersion := ingest.GetSchemaVersion(store)
	if storedVersion != "" && storedVersion != config.SchemaVersion {
		status["version_mismatch"] = true
		status["warning"] = fmt.Sprintf("store was ingested with schema version %s; re-ingest recommended (current: %s)", storedVersion, config.SchemaVersion)
	}

	incrState, stateErr := ingest.LoadIncrementalState(store)
	if stateErr == nil {
		status["file_count"] = len(incrState.FileHashes)
	}

	// Optional git comparison against a working tree.
	if sourceDir != "" {
		if !filepath.IsAbs(sourceDir) {
			return errorResult("source_dir must be an absolute path"), nil
		}
		if ingest.IsGitRepo(sourceDir) {
			head, headErr := ingest.GetHEADCommitSHA(sourceDir)
			if headErr == nil {
				status["head_commit"] = head
			}
			last := ingest.GetLastCommitSHA(store)
			if last != "" {
				if behind, behindErr := ingest.CountCommitsBehind(sourceDir, last); behindErr == nil {
					status["commits_behind"] = behind
				}
			}
		}
	}
	return jsonResult(status), nil
}

func (s *Server) handleIngestIncremental(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.mgr.ReadOnly() {
		return errorResult("server is read-only; start with --writable (or GCA_WRITABLE=true) to run incremental ingest"), nil
	}
	args := request.GetArguments()
	project, err := requireProject(args)
	if err != nil {
		return errorResult("%v", err), nil
	}
	sourceDir, err := requireString(args, "source_dir")
	if err != nil {
		return errorResult("%v", err), nil
	}
	if !filepath.IsAbs(sourceDir) {
		return errorResult("source_dir must be an absolute path"), nil
	}
	if info, statErr := os.Stat(sourceDir); statErr != nil || !info.IsDir() {
		return errorResult("source_dir does not exist or is not a directory: %s", sourceDir), nil
	}

	fromCommit, _ := args["from_commit"].(string)
	toCommit, _ := args["to_commit"].(string)
	skipEmbed, _ := args["skip_embed"].(bool)

	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.mgr.GetStore(project)
	if err != nil {
		return errorResult("project not found: %s", project), nil
	}

	opts := &ingest.IngestOptions{
		SkipEmbeddings: skipEmbed,
		FromCommit:     fromCommit,
		ToCommit:       toCommit,
	}
	state := ingest.NewIngestState()
	if err := ingest.RunIncrementalWithOptions(store, project, sourceDir, state, opts); err != nil {
		return errorResult("incremental ingest failed: %v", err), nil
	}
	return jsonResult(map[string]any{
		"project_id": project,
		"status":     "completed",
		"symbols":    len(state.SymbolTable),
		"files":      len(state.FileIndex),
	}), nil
}

// --- Helpers ---

// templateParam extracts a named variable from a resource template request.
// mcp-go passes matched template arguments via Params.Arguments; the value is
// the uritemplate Value whose V field is a []string (single-element for
// non-list captures).
func templateParam(request mcp.ReadResourceRequest, name string) string {
	v, ok := request.Params.Arguments[name]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []string:
		if len(val) > 0 {
			return val[0]
		}
	}
	return ""
}

func mustJSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte(fmt.Sprintf(`{"error": %q}`, err.Error()))
	}
	return b
}

// buildStructuralGraph collects nodes and links from structural predicates
// (calls, imports, defines) for clustering.
func buildStructuralGraph(store *meb.MEBStore) ([]service.GraphNode, []service.GraphLink) {
	var nodes []service.GraphNode
	var links []service.GraphLink
	nodeSet := make(map[string]bool)
	preds := []string{config.PredicateCalls, config.PredicateImports, config.PredicateDefines}

	for _, pred := range preds {
		for fact := range store.Scan("", pred, "") {
			src := fact.Subject
			dst, ok := fact.Object.(string)
			if !ok {
				continue
			}
			if !nodeSet[src] {
				nodes = append(nodes, service.GraphNode{ID: src})
				nodeSet[src] = true
			}
			if !nodeSet[dst] {
				nodes = append(nodes, service.GraphNode{ID: dst})
				nodeSet[dst] = true
			}
			links = append(links, service.GraphLink{Source: src, Target: dst})
		}
	}
	return nodes, links
}

// schemaConventionsMarkdown is the static schema documentation resource.
var schemaConventionsMarkdown = `# GCA Knowledge Graph Conventions

## 1. Node Types
- 'file': A source code file (e.g., github.com/duynguyendang/meb/store.go)
- 'function': A named function or method.
- 'struct': A data structure definition.
- 'cluster': A logical community identified by the Leiden algorithm.

## 2. Predicates (Relationships)
- 'defines': [file] -> [function/struct]. The file contains the definition.
- 'calls': [function] -> [function]. Direct function call.
- 'references': [function/struct] -> [struct]. Usage of a type.
- 'imports': [file] -> [file]. Dependency between files.
- 'belongs_to': [any node] -> [cluster]. Mapping from code to Leiden community.

## 3. Usage Guidelines
- To find impact: Search for 'calls' or 'references' where the Object is the target node.
- To find architecture: Search for 'belongs_to' to see the logical grouping.
- To trace file-level deps: Use the 'imports' predicate.
`
