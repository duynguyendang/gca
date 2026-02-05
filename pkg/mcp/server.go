package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SingleProjectManager adapts a single store to the ProjectStoreManager interface.
type SingleProjectManager struct {
	store *meb.MEBStore
}

func (m *SingleProjectManager) GetStore(projectID string) (*meb.MEBStore, error) {
	return m.store, nil
}

func (m *SingleProjectManager) ListProjects() ([]manager.ProjectMetadata, error) {
	return []manager.ProjectMetadata{{Name: "default"}}, nil
}

// MCPServer wraps the GCA store to expose it via MCP.
type MCPServer struct {
	store      *meb.MEBStore
	graph      *service.GraphService
	clustering *service.ClusteringService
}

// Run starts the MCP server on Stdio.
func Run(ctx context.Context, store *meb.MEBStore) error {
	s := server.NewMCPServer(
		"GCA-Backend",
		"0.1.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	mgr := &SingleProjectManager{store: store}
	ms := &MCPServer{
		store:      store,
		graph:      service.NewGraphService(mgr),
		clustering: service.NewClusteringService(),
	}

	// --- Resources ---

	// Resource: Graph Summary
	s.AddResource(
		mcp.NewResource(
			"gca://graph/summary",
			"Graph Summary",
			mcp.WithResourceDescription("Summary statistics of the graph database"),
			mcp.WithMIMEType("application/json"),
		),
		ms.handleGraphSummary,
	)

	// Resource: File Content
	// Pattern: gca://files/{path}
	s.AddResource(
		mcp.NewResource(
			"gca://files/{path}",
			"File Content",
			mcp.WithResourceDescription("Content of a source file"),
			mcp.WithMIMEType("text/plain"), // Most source files are text
		),
		ms.handleFileContent,
	)

	// Resource: Schema Conventions
	s.AddResource(
		mcp.NewResource(
			"gca://schema/conventions",
			"Schema Conventions",
			mcp.WithResourceDescription("Architectural schema and naming conventions for GCA"),
			mcp.WithMIMEType("text/markdown"),
		),
		ms.handleSchemaConventions,
	)

	// --- Tools ---

	// Tool: Search Nodes
	s.AddTool(
		mcp.NewTool(
			"search_nodes",
			mcp.WithDescription("Search for nodes (symbols, files) in the graph."),
			mcp.WithString("query", mcp.Required(), mcp.Description("The search query string")),
			mcp.WithNumber("limit", mcp.Description("Max number of results (default 10)")),
		),
		ms.handleSearchNodes,
	)

	// Tool: Get Outgoing Edges (Dependencies)
	s.AddTool(
		mcp.NewTool(
			"get_outgoing_edges",
			mcp.WithDescription("Get outgoing edges (dependencies/calls) from a specific node."),
			mcp.WithString("node_id", mcp.Required(), mcp.Description("The ID of the source node")),
		),
		ms.handleGetOutgoingEdges,
	)

	// Tool: Get Incoming Edges (Consumers)
	s.AddTool(
		mcp.NewTool(
			"get_incoming_edges",
			mcp.WithDescription("Get incoming edges (consumers/callers) to a specific node."),
			mcp.WithString("node_id", mcp.Required(), mcp.Description("The ID of the target node")),
		),
		ms.handleGetIncomingEdges,
	)

	// Tool: Scan Facts (Direct DB Access)
	s.AddTool(
		mcp.NewTool(
			"scan_facts",
			mcp.WithDescription("Scan raw facts from the database (Subject, Predicate, Object). Empty fields act as wildcards."),
			mcp.WithString("subject", mcp.Description("Subject filter")),
			mcp.WithString("predicate", mcp.Description("Predicate filter")),
			mcp.WithString("object", mcp.Description("Object filter")),
		),
		ms.handleScanFacts,
	)

	// Tool: Get Clusters (Community Detection)
	s.AddTool(
		mcp.NewTool(
			"get_clusters",
			mcp.WithDescription("Detect clusters/communities in the graph using Leiden algorithm."),
		),
		ms.handleGetClusters,
	)

	// Tool: Get Node Metadata
	s.AddTool(
		mcp.NewTool(
			"get_node_metadata",
			mcp.WithDescription("Get detailed metadata for a node (kind, package, tags, etc.)."),
			mcp.WithString("node_id", mcp.Required(), mcp.Description("The ID of the node")),
		),
		ms.handleGetNodeMetadata,
	)

	// Tool: Trace Impact Path
	s.AddTool(
		mcp.NewTool(
			"trace_impact_path",
			mcp.WithDescription("Trace the shortest dependency path between two nodes, considering edge weights."),
			mcp.WithString("start_node", mcp.Required(), mcp.Description("Start node ID")),
			mcp.WithString("end_node", mcp.Required(), mcp.Description("End node ID")),
		),
		ms.handleTraceImpactPath,
	)

	// Start the server on Stdio
	slog.Info("Starting MCP server on Stdio")
	return server.ServeStdio(s)
}

// --- Resource Handlers ---

func (ms *MCPServer) handleGraphSummary(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	count := ms.store.Count()
	summary := map[string]interface{}{
		"fact_count": count,
	}

	jsonBytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal summary: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(jsonBytes),
		},
	}, nil
}

func (ms *MCPServer) handleFileContent(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	// Extract path from URI: gca://files/{path}
	uriStr := request.Params.URI
	prefix := "gca://files/"
	if !strings.HasPrefix(uriStr, prefix) {
		return nil, fmt.Errorf("invalid URI format")
	}
	path := strings.TrimPrefix(uriStr, prefix)

	// Retrieve document
	// DocumentID in store seems to be just the string path/ID
	doc, err := ms.store.GetDocument(meb.DocumentID(path))
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	if doc.Content == nil {
		return nil, fmt.Errorf("no content available for file: %s", path)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/plain", // TODO: Detect mime type if possible, or assume text for code
			Text:     string(doc.Content),
		},
	}, nil
}

func (ms *MCPServer) handleSchemaConventions(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	content := `
# GCA Knowledge Graph Conventions

## 1. Node Types
- 'file': A source code file (e.g., pkg/meb/store.go)
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
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/markdown",
			Text:     content,
		},
	}, nil
}

// --- Tool Handlers ---

func (ms *MCPServer) handleSearchNodes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	query, ok := args["query"].(string)
	if !ok {
		return mcp.NewToolResultError("query argument required"), nil
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// Use store.SearchSymbols
	results, err := ms.store.SearchSymbols(query, limit, "")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	return mcp.NewToolResultText(strings.Join(results, "\n")), nil
}

func (ms *MCPServer) handleGetOutgoingEdges(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	nodeID, ok := args["node_id"].(string)
	if !ok {
		return mcp.NewToolResultError("node_id argument required"), nil
	}

	var formatted []string
	// Scan(s=nodeID, p="", o="", g="")
	for fact, err := range ms.store.Scan(nodeID, "", "", "") {
		if err != nil {
			continue
		}
		// Format: Predicate -> Object
		formatted = append(formatted, fmt.Sprintf("%s -> %s", fact.Predicate, fact.Object))
	}

	if len(formatted) == 0 {
		return mcp.NewToolResultText("No outgoing edges found."), nil
	}

	return mcp.NewToolResultText(strings.Join(formatted, "\n")), nil
}

func (ms *MCPServer) handleGetIncomingEdges(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	nodeID, ok := args["node_id"].(string)
	if !ok {
		return mcp.NewToolResultError("node_id argument required"), nil
	}

	var formatted []string
	// Scan(s="", p="", o=nodeID, g="")
	for fact, err := range ms.store.Scan("", "", nodeID, "") {
		if err != nil {
			continue
		}
		// Format: Subject -> Predicate
		formatted = append(formatted, fmt.Sprintf("%s -> %s", fact.Subject, fact.Predicate))
	}

	if len(formatted) == 0 {
		return mcp.NewToolResultText("No incoming edges found."), nil
	}

	return mcp.NewToolResultText(strings.Join(formatted, "\n")), nil
}

func (ms *MCPServer) handleScanFacts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	s, _ := args["subject"].(string)
	p, _ := args["predicate"].(string)
	o, _ := args["object"].(string)

	var formatted []string
	count := 0
	maxResults := 50 // Safety limit

	for fact, err := range ms.store.Scan(s, p, o, "") {
		if err != nil {
			continue // Skip errors during iteration
		}
		formatted = append(formatted, fmt.Sprintf("%s --[%s]--> %s", fact.Subject, fact.Predicate, fact.Object))
		count++
		if count >= maxResults {
			formatted = append(formatted, "... (truncated)")
			break
		}
	}

	if len(formatted) == 0 {
		return mcp.NewToolResultText("No facts found."), nil
	}

	return mcp.NewToolResultText(strings.Join(formatted, "\n")), nil
}

func (ms *MCPServer) handleGetClusters(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 1. Build simple graph from store
	nodes := []service.GraphNode{}
	links := []service.GraphLink{}
	nodeSet := make(map[string]bool)

	// Scan all triples is too expensive for huge graph tool call, but for clustering we need structure.
	// We'll limit to "calls", "imports", "defines" using iterator logic if possible, but Scan is iterable.
	// For "GetClusters", scanning whole DB might be slow.
	// Let's rely on cached graph or just scan specific predicates.
	// Or we scan EVERYTHING and filter.

	// Scanning everything
	// NOTE: This might be slow on large DBs.
	// Optimized approach: Only scan structural edges.
	structuralPreds := []string{meb.PredCalls, meb.PredImports, meb.PredDefines}

	for _, pred := range structuralPreds {
		for fact, err := range ms.store.Scan("", pred, "", "") {
			if err != nil {
				continue
			}
			src := fact.Subject.String()
			dst := fact.Object.(string)

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

	// 2. Run Clustering
	result := ms.clustering.DetectCommunitiesLeiden(nodes, links)

	// 3. Return JSON
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("failed to marshal clusters"), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (ms *MCPServer) handleGetNodeMetadata(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	nodeID, ok := args["node_id"].(string)
	if !ok {
		return mcp.NewToolResultError("node_id argument required"), nil
	}

	// Use Hydrate to get metadata
	ids := []meb.DocumentID{meb.DocumentID(nodeID)}
	hydrated, err := ms.store.Hydrate(ctx, ids, true) // shallow hydration
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("hydration failed: %v", err)), nil
	}
	if len(hydrated) == 0 {
		return mcp.NewToolResultText("{}"), nil // Not found
	}

	h := hydrated[0]
	// Clean up for JSON
	meta := map[string]interface{}{
		"id":             h.ID,
		"kind":           h.Kind,
		"metadata":       h.Metadata,
		"children_count": len(h.Children),
	}

	jsonBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("failed to marshal metadata"), nil
	}
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (ms *MCPServer) handleTraceImpactPath(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	startNode, ok1 := args["start_node"].(string)
	endNode, ok2 := args["end_node"].(string)
	if !ok1 || !ok2 {
		return mcp.NewToolResultError("start_node and end_node arguments required"), nil
	}

	// Use GraphService.FindShortestPath
	// We need to pass a ProjectID. In Single Store Mode, ProjectID is used for prefixes but store is shared.
	// Our SingleProjectManager returns the store regardless of ID.
	// So we can pass a dummy project ID or derive it.
	projectID := "default"
	if strings.Contains(startNode, "/") {
		projectID = strings.Split(startNode, "/")[0]
	}

	graph, err := ms.graph.FindShortestPath(ctx, projectID, startNode, endNode)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("pathfinding failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("failed to marshal graph"), nil
	}
	return mcp.NewToolResultText(string(jsonBytes)), nil
}
