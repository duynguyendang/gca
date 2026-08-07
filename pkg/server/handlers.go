package server

import (
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	apperrors "github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ephemeral"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/gca/pkg/ooda"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/gin-gonic/gin"
)

// handleProjects returns a list of available projects.
// Query parameters: none
// Response: JSON array of project objects with id, name, and metadata.
func (s *Server) handleProjects(c *gin.Context) {
	projects, err := s.graphService.ListProjects()
	if err != nil {
		logger.Error("handleProjects error", "error", err)
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to list projects", err))
		return
	}
	c.JSON(http.StatusOK, projects)
}

// handleQuery executes a Datalog query and returns the results in a graph format.
// Request body: {"query": "<datalog query>"}
// Query parameters:
//   - project: project ID to query
//   - lazy: enable lazy loading (default: false)
//   - raw: return raw results instead of graph (default: false)
//   - nocluster: disable auto-clustering (default: false)
//
// Response: JSON graph with nodes and links, or raw query results.
func (s *Server) handleQuery(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	// Validate and sanitize query
	sanitizedQuery, err := ValidateAndSanitizeQuery(req.Query)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "query is invalid", err))
		return
	}

	// If query is empty, return empty graph to prevent frontend crashes
	if sanitizedQuery == "" {
		c.JSON(http.StatusOK, gin.H{"nodes": []interface{}{}, "links": []interface{}{}})
		return
	}

	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	lazy := c.Query("lazy") == "true"
	hydrate := c.Query("hydrate") != "false" // Hydrate by default unless ?hydrate=false
	raw := c.Query("raw") == "true"
	autocluster := c.Query("nocluster") != "true" // Auto-cluster by default unless ?nocluster=true

	if raw {
		results, err := s.graphService.ExecuteQuery(c.Request.Context(), projectID, req.Query)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
		return
	}

	// Delegate to service
	graph, err := s.graphService.ExportGraph(c.Request.Context(), projectID, req.Query, hydrate, lazy, 0, 0)
	if err != nil {
		handleError(c, err)
		return
	}

	// Auto-cluster if too many nodes
	if autocluster && len(graph.Nodes) > config.AutoClusterThreshold {
		clustered, clusterErr := s.graphService.GetClusterGraph(c.Request.Context(), projectID, req.Query)
		if clusterErr == nil && len(clustered.Nodes) > 0 {
			c.JSON(http.StatusOK, clustered)
			return
		}
		// Fall back to original if clustering fails
	}

	c.JSON(http.StatusOK, graph)
}

// handleGraph returns a composite graph for a specific file.
// Query parameters:
//   - project: project ID
//   - file: file ID to get graph for
//   - lazy: enable lazy loading (default: false)
//
// Response: JSON graph with nodes and links showing file relationships.
func (s *Server) handleGraph(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	fileID, ok := extractSymbolID(c, "file")
	if !ok {
		return
	}
	lazy := c.Query("lazy") == "true"

	graph, err := s.graphService.GetFileGraph(c.Request.Context(), projectID, fileID, lazy)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleSource returns source code for a given file ID.
// Query parameters:
//   - project: project ID
//   - id: file or symbol ID
//   - start: optional start line number (1-based)
//   - end: optional end line number
//
// Response: Plain text source code for the specified range.
func (s *Server) handleSource(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	id, ok := extractSymbolID(c, "id")
	if !ok {
		return
	}

	content, err := s.graphService.GetSource(projectID, id)
	if err != nil {
		handleError(c, err)
		return
	}

	startStr := c.Query("start")
	endStr := c.Query("end")

	start, err := strconv.Atoi(startStr)
	if err != nil {
		start = 1
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		end = -1
	}

	lines := strings.Split(content, "\n")

	// Normalize line range bounds
	if start < 1 {
		start = 1
	}
	if end == -1 || end > len(lines) {
		end = len(lines)
	}

	if start > len(lines) || start > end {
		c.String(http.StatusOK, "")
		return
	}

	slice := lines[start-1 : end]
	result := strings.Join(slice, "\n")

	c.String(http.StatusOK, result)
}

// handleSummary returns the project summary.
func (s *Server) handleSummary(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	summary, err := s.graphService.GenerateSummary(projectID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

// handlePredicates returns the list of active predicates in the database.
func (s *Server) handlePredicates(c *gin.Context) {
	projectID := c.Query("project")

	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}

	if projectID == "" {
		c.JSON(http.StatusOK, gin.H{"predicates": []map[string]string{}})
		return
	}

	if err := ValidateProjectID(projectID); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "invalid project ID", err))
		return
	}

	results, err := s.graphService.GetPredicates(projectID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"predicates": results})
}

// handleSymbols provides fast symbol search/autocomplete.
// Query parameters:
//   - project: project ID
//   - q: search query string
//   - p: predicate to filter by (default: "defines")
//   - all: if set, search across all predicates
//
// Response: JSON with symbols array containing matching symbol IDs.
func (s *Server) handleSymbols(c *gin.Context) {
	projectID := c.Query("project")
	query := c.Query("q")

	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}

	if projectID == "" {
		c.JSON(http.StatusOK, gin.H{"symbols": []string{}})
		return
	}

	if err := ValidateProjectID(projectID); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "project ID is required or invalid", err))
		return
	}

	// Validate and sanitize query parameter
	query = SanitizeString(query)
	if len(query) > config.MaxSearchQueryLength {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "query exceeds maximum length", nil))
		return
	}

	predicate := c.Query("p")
	if predicate == "" && c.Query("all") != "true" {
		predicate = config.PredicateDefines
	}

	// Validate predicate parameter
	if predicate != "" {
		predicate = SanitizeString(predicate)
		if len(predicate) > config.MaxPredicateLength {
			handleError(c, apperrors.NewAppError(http.StatusBadRequest, "predicate exceeds maximum length", nil))
			return
		}
	}

	results, err := s.graphService.SearchSymbols(projectID, query, predicate, 50)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"symbols": results})
}

// handleFiles returns a list of all ingested files for the project.
// Optional: ?prefix=path/to/package to filter files by prefix
func (s *Server) handleFiles(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	prefix := c.Query("prefix")

	// Validate and sanitize prefix parameter
	if prefix != "" {
		// Sanitize the prefix
		prefix = SanitizeString(prefix)

		// Check for path traversal attempts - normalize path separators
		normalized := strings.ReplaceAll(prefix, "\\", "/")
		if strings.Contains(normalized, "..") {
			handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Invalid prefix format", nil))
			return
		}

		// Check for excessively long prefixes
		if len(prefix) > config.MaxPrefixLength {
			handleError(c, apperrors.NewAppError(http.StatusBadRequest, "prefix exceeds maximum length", nil))
			return
		}
	}

	files, err := s.graphService.ListFiles(projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	// Filter by prefix if provided
	if prefix != "" {
		// Extract the package suffix (last segment) for matching
		// e.g., "github.com/google/mangle/ast" -> "ast"
		pkgSuffix := prefix
		if idx := strings.LastIndex(prefix, "/"); idx != -1 {
			pkgSuffix = prefix[idx+1:]
		}
		dirPrefix := pkgSuffix + "/"

		var filtered []string
		for _, f := range files {
			// Match either full prefix OR directory prefix
			if strings.HasPrefix(f, prefix) || strings.HasPrefix(f, dirPrefix) {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	c.JSON(http.StatusOK, files)
}

// handleGraphMap returns a high-level view of file dependencies.
func (s *Server) handleGraphMap(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	autocluster := c.Query("nocluster") != "true"

	graph, err := s.graphService.GetProjectMap(c.Request.Context(), projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	// Auto-cluster if too many nodes
	if autocluster && len(graph.Nodes) > config.AutoClusterThreshold {
		clustered, clusterErr := s.graphService.ClusterGraphData(graph)
		if clusterErr == nil && len(clustered.Nodes) > 0 {
			c.JSON(http.StatusOK, clustered)
			return
		}
	}

	c.JSON(http.StatusOK, graph)
}

// handleGraphManifest returns a compressed project manifest for the AI.
func (s *Server) handleGraphManifest(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	manifest, err := s.graphService.GetManifest(c.Request.Context(), projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, manifest)
}

// handleFileDetails returns detailed internal symbols for a file.
func (s *Server) handleFileDetails(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	fileID, ok := extractSymbolID(c, "file")
	if !ok {
		return
	}

	graph, err := s.graphService.GetFileDetails(c.Request.Context(), projectID, fileID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleHydrate returns the full hydrated symbol for a given ID.
func (s *Server) handleHydrate(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	id, ok := extractSymbolID(c, "id")
	if !ok {
		return
	}

	symbol, err := s.graphService.GetSymbol(c.Request.Context(), projectID, id)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, symbol)
}

// handleGraphBackbone returns a filtered graph showing only cross-file dependencies.
func (s *Server) handleGraphBackbone(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	aggregate := c.Query("aggregate") == "true"
	graph, err := s.graphService.GetBackboneGraph(c.Request.Context(), projectID, aggregate)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleFileCalls returns a recursive file-to-file call graph.
func (s *Server) handleFileCalls(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	id, ok := extractSymbolID(c, "id")
	if !ok {
		return
	}
	depthStr := c.Query("depth")

	depth := 1 // Default to direct dependencies only
	if depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil {
			depth = d
		}
	}
	// Enforce max depth for performance - limit to 2 levels max
	if depth > 2 {
		depth = 2
	}

	graph, err := s.graphService.GetFileCalls(c.Request.Context(), projectID, id, depth)
	if err != nil {
		logger.Error("handleFileCalls error", "error", err)
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleError is a helper that converts errors to JSON responses.
// It uses the errors.MapError function to convert errors to AppError with HTTP status codes.
func handleError(c *gin.Context, err error) {
	appErr := apperrors.MapError(err)
	c.JSON(appErr.Code, gin.H{"error": appErr.Message})
}

// handleFlowPath returns the shortest call graph path between two symbols/files.
func (s *Server) handleFlowPath(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	from, ok := extractSymbolID(c, "from")
	if !ok {
		return
	}
	to, ok := extractSymbolID(c, "to")
	if !ok {
		return
	}

	graph, err := s.graphService.GetFlowPath(c.Request.Context(), projectID, from, to)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleGraphPath returns the shortest interaction path between two symbols using BFS.
func (s *Server) handleGraphPath(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	source, ok := extractSymbolID(c, "source")
	if !ok {
		return
	}
	target, ok := extractSymbolID(c, "target")
	if !ok {
		return
	}

	graph, err := s.graphService.FindShortestPath(c.Request.Context(), projectID, source, target)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleSemanticSearch performs vector similarity search on embedded documentation.
// Query parameters:
//   - project: project ID
//   - q: search query string
//   - k: number of results to return (default: 10, max: 50)
//
// Response: JSON with query, count, and results array of matching symbols.
func (s *Server) handleSemanticSearch(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	query := c.Query("q")
	kStr := c.DefaultQuery("k", "10")

	k, err := strconv.Atoi(kStr)
	if err != nil || k <= 0 {
		k = 10
	}
	if k > 50 {
		k = 50 // Cap results
	}
	if query == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Missing q parameter", nil))
		return
	}

	// Validate and sanitize query
	query = SanitizeString(query)
	if len(query) > config.MaxQueryLength {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "query exceeds maximum length", nil))
		return
	}

	// Get embedding for query using AI Service
	if s.aiService == nil {
		handleError(c, apperrors.NewAppError(http.StatusServiceUnavailable, "AI service not initialized", nil))
		return
	}

	results, err := s.graphService.SemanticSearch(c.Request.Context(), projectID, query, k, s.aiService)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"count":   len(results),
		"results": results,
	})
}

// handleGraphCluster returns a clustered graph for large result sets.
// GET /v1/graph/cluster?project=X&query=...
func (s *Server) handleGraphCluster(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	query := c.Query("query")
	if query == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Missing query parameter", nil))
		return
	}

	// Validate and sanitize query
	query = SanitizeString(query)
	if len(query) > config.MaxQueryLength {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "query exceeds maximum length", nil))
		return
	}

	graph, err := s.graphService.GetClusterGraph(c.Request.Context(), projectID, query)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleGraphSubgraph returns a subgraph matching the provided IDs.
func (s *Server) handleGraphSubgraph(c *gin.Context) {
	var req struct {
		Ids []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	// Validate IDs list
	if err := ValidateIDs(req.Ids); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "symbol ID is required or invalid", err))
		return
	}

	graph, err := s.graphService.GetSubgraph(c.Request.Context(), projectID, req.Ids)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleGraphCommunities returns the hierarchical community structure.
func (s *Server) handleGraphCommunities(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	hierarchy, err := s.graphService.DetectCommunityHierarchy(c.Request.Context(), projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, hierarchy)
}

// handleHybridCluster performs k-means clustering on vector results while preserving community structure.
func (s *Server) handleHybridCluster(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	var req struct {
		Embedding []float32 `json:"embedding"`
		Limit     int       `json:"limit"`
		Clusters  int       `json:"clusters"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	// Validate embedding
	if err := ValidateEmbedding(req.Embedding); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "symbol ID is required or invalid", err))
		return
	}

	// Validate and set default values for limit and clusters
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Clusters <= 0 {
		req.Clusters = 5
	}

	// Validate limit
	if err := ValidateLimit(req.Limit, 1000); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "symbol ID is required or invalid", err))
		return
	}

	// Validate clusters
	if err := ValidateClusters(req.Clusters); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "symbol ID is required or invalid", err))
		return
	}

	result, err := s.graphService.GetHybridClusters(c.Request.Context(), projectID, req.Embedding, req.Limit, req.Clusters)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGraphPaginated returns a paginated subset of the graph for lazy loading.
// Query parameters:
//   - project: project ID
//   - query: Datalog query string
//   - cursor: pagination cursor from previous response (optional)
//   - limit: maximum nodes to return (default: 100, max: 1000)
//   - offset: starting offset as alternative to cursor (optional)
//
// Response: JSON graph with paginated nodes/links and next cursor.
func (s *Server) handleGraphPaginated(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	query := c.Query("query")
	if query == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Missing query parameter", nil))
		return
	}

	// Parse pagination options
	cursorStr := c.Query("cursor")
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))

	cursor, err := export.ParseCursor(cursorStr)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Invalid cursor format", err))
		return
	}

	// Use cursor offset if provided, otherwise use query offset
	if cursor.Offset > 0 && offset == 0 {
		offset = cursor.Offset
	}
	if limit > 0 {
		cursor.Limit = limit
	}

	// Server-side pagination: pass limit/offset to query execution
	pageLimit := cursor.Limit
	pageOffset := offset

	graph, err := s.graphService.ExportGraph(c.Request.Context(), projectID, query, true, false, pageLimit, pageOffset)
	if err != nil {
		handleError(c, err)
		return
	}

	// Determine HasMore: if we received exactly pageLimit rows, there may be more
	hasMore := len(graph.Nodes) >= pageLimit && pageLimit > 0

	// Build next cursor
	nextCursor := ""
	if hasMore {
		nextOffset := pageOffset + pageLimit
		cursor := export.GraphCursor{
			Offset: nextOffset,
			Limit:  pageLimit,
		}
		if buf, err := json.Marshal(cursor); err == nil {
			nextCursor = string(buf)
		}
	}

	paginatedGraph := &export.D3Graph{
		Nodes:      graph.Nodes,
		Links:      graph.Links,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		TotalNodes: len(graph.Nodes), // Client can track total via offset
	}
	c.JSON(http.StatusOK, paginatedGraph)
}

// handleWhoCalls returns all callers of a symbol (backward slice).
// Query parameters:
//   - project: project ID
//   - symbol: symbol ID to find callers for
//   - depth: maximum traversal depth (default: 1, max: 10)
//   - focused: if true and depth<=1, uses direct scan (faster, no full graph build)
//
// Response: JSON graph with callers and call relationships.
func (s *Server) handleWhoCalls(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	symbolID := c.Query("symbol")
	depth, _ := strconv.Atoi(c.Query("depth"))
	if symbolID == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Missing symbol parameter", nil))
		return
	}

	if depth <= 0 {
		depth = 1
	}
	if depth > 10 {
		depth = 10
	}

	var graph *export.D3Graph
	var err error

	// Use focused methods for depth=1 to avoid building full call graph
	if depth == 1 {
		graph, err = s.graphService.GetWhoCallsFocusedGraph(c.Request.Context(), projectID, symbolID)
	} else {
		graph, err = s.graphService.GetWhoCalls(c.Request.Context(), projectID, symbolID, depth)
	}

	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleWhatCalls returns all callees of a symbol (forward slice).
// Query parameters:
//   - project: project ID
//   - symbol: symbol ID to find callees for
//   - depth: maximum traversal depth (default: 1, max: 10)
//   - focused: if true and depth<=1, uses direct scan (faster, no full graph build)
//
// Response: JSON graph with callees and call relationships.
func (s *Server) handleWhatCalls(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	symbolID := c.Query("symbol")
	depth, _ := strconv.Atoi(c.Query("depth"))
	if symbolID == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Missing symbol parameter", nil))
		return
	}

	if depth <= 0 {
		depth = 1
	}
	if depth > 10 {
		depth = 10
	}

	var graph *export.D3Graph
	var err error

	// Use focused methods for depth=1 to avoid building full call graph
	if depth == 1 {
		graph, err = s.graphService.GetWhatCallsFocusedGraph(c.Request.Context(), projectID, symbolID)
	} else {
		graph, err = s.graphService.GetWhatCalls(c.Request.Context(), projectID, symbolID, depth)
	}

	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleCheckReachability checks if symbol A can reach symbol B.
// Query parameters:
//   - project: project ID
//   - from: source symbol ID
//   - to: target symbol ID
//   - depth: maximum traversal depth (default: 5, max: 20)
//
// Response: JSON with reachable: true/false
func (s *Server) handleCheckReachability(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	fromID := c.Query("from")
	toID := c.Query("to")
	depth, _ := strconv.Atoi(c.Query("depth"))
	if fromID == "" || toID == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Missing from or to parameter", nil))
		return
	}

	reachable, err := s.graphService.CheckReachability(c.Request.Context(), projectID, fromID, toID, depth)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"reachable": reachable, "from": fromID, "to": toID})
}

// handleDetectCycles returns all cycles in the call graph.
// Query parameters:
//   - project: project ID
//
// Response: JSON with array of cycles (each cycle is array of symbol IDs)
func (s *Server) handleDetectCycles(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	cycles, err := s.graphService.DetectCycles(c.Request.Context(), projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"cycles": cycles, "count": len(cycles)})
}

// handleFindLCA finds the least common ancestor of two symbols.
// Query parameters:
//   - project: project ID
//   - a: first symbol ID
//   - b: second symbol ID
//   - depth: maximum traversal depth (default: 10, max: 30)
//
// Response: JSON with lca: symbol ID or null
func (s *Server) handleFindLCA(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}
	symbolA := c.Query("a")
	symbolB := c.Query("b")
	depth, _ := strconv.Atoi(c.Query("depth"))
	if symbolA == "" || symbolB == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Missing a or b parameter", nil))
		return
	}

	lca, err := s.graphService.FindLCA(c.Request.Context(), projectID, symbolA, symbolB, depth)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"lca": lca, "a": symbolA, "b": symbolB})
}

// handleEnrichCalledBy adds called_by predicates to the graph store.
// Query parameters:
//   - project: project ID
//
// Response: JSON with status
func (s *Server) handleEnrichCalledBy(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	err := s.graphService.EnrichWithCalledBy(c.Request.Context(), projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "enriched", "predicate": "called_by"})
}

// handleAsk is a unified endpoint for natural language queries.
// It classifies the intent, converts to Datalog, executes, and synthesizes an answer.
//
// Request body: ai.AskRequest
//   - project_id: project ID (required)
//   - query: natural language question (required)
//   - symbol_id: optional symbol to focus on
//   - depth: optional traversal depth
//   - context: optional conversation history
//
// Response: ai.AskResponse
//   - answer: synthesized natural language answer
//   - query: generated Datalog query
//   - intent: detected intent (who_calls, what_calls, explain, etc.)
//   - confidence: intent classification confidence (0-1)
//   - results: raw query results
//   - summary: brief summary
//   - error: error message if any
func (s *Server) handleAsk(c *gin.Context) {
	var req struct {
		ProjectID           string                `json:"project_id"`
		Query               string                `json:"query"`
		SymbolID            string                `json:"symbol_id"`
		Depth               int                   `json:"depth"`
		Context             string                `json:"context"`
		ConversationHistory []ai.ConversationTurn `json:"conversation_history"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	if req.ProjectID == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "project_id is required", nil))
		return
	}
	if req.Query == "" {
		handleError(c, apperrors.NewAppError(http.StatusBadRequest, "query is required", nil))
		return
	}

	askReq := ai.AskRequest{
		ProjectID:           req.ProjectID,
		Query:               req.Query,
		SymbolID:            req.SymbolID,
		Depth:               req.Depth,
		Context:             req.Context,
		ConversationHistory: req.ConversationHistory,
	}

	resp, err := s.aiService.HandleAsk(c.Request.Context(), askReq)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "internal server error", err))
		return
	}

	c.JSON(http.StatusOK, resp)
}

type GraphDiffRequest struct {
	BeforeSnapshot string `json:"before_snapshot_path"`
	AfterSnapshot  string `json:"after_snapshot_path"`
	ProjectID      string `json:"project_id"`
	BeforeID       string `json:"before_id"`
	AfterID        string `json:"after_id"`
}

func (s *Server) handleGraphDiff(c *gin.Context) {
	var req GraphDiffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	diffService := service.NewGraphDiffService()
	var beforeSnap, afterSnap *service.GraphSnapshot
	var err error

	if req.BeforeSnapshot != "" {
		beforeSnap, err = diffService.LoadSnapshot(req.BeforeSnapshot)
		if err != nil {
			handleError(c, apperrors.NewAppError(http.StatusBadRequest, "failed to load before snapshot", err))
			return
		}
	} else if req.ProjectID != "" && req.BeforeID != "" {
		store, err := s.manager.GetStore(req.ProjectID)
		if err != nil {
			handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to get store", err))
			return
		}
		snap, err := diffService.TakeSnapshot(c.Request.Context(), store, req.ProjectID)
		if err != nil {
			handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to take snapshot", err))
			return
		}
		beforeSnap = snap
	}

	if req.AfterSnapshot != "" {
		afterSnap, err = diffService.LoadSnapshot(req.AfterSnapshot)
		if err != nil {
			handleError(c, apperrors.NewAppError(http.StatusBadRequest, "failed to load after snapshot", err))
			return
		}
	} else if req.ProjectID != "" {
		store, err := s.manager.GetStore(req.ProjectID)
		if err != nil {
			handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to get store", err))
			return
		}
		afterSnap, err = diffService.TakeSnapshot(c.Request.Context(), store, req.ProjectID)
		if err != nil {
			handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to take snapshot", err))
			return
		}
	}

	if beforeSnap == nil && afterSnap == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "must provide either snapshots or project_id"})
		return
	}

	if beforeSnap == nil {
		beforeSnap = &service.GraphSnapshot{Nodes: map[string]service.NodeSnap{}, Edges: map[string]service.EdgeSnap{}, Communities: map[string]int{}}
	}
	if afterSnap == nil {
		afterSnap = &service.GraphSnapshot{Nodes: map[string]service.NodeSnap{}, Edges: map[string]service.EdgeSnap{}, Communities: map[string]int{}}
	}

	diff := diffService.DiffSnapshots(beforeSnap, afterSnap)
	c.JSON(http.StatusOK, diff)
}

type SnapshotInfo struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
	NodeCount int       `json:"node_count"`
	EdgeCount int       `json:"edge_count"`
}

type CreateSnapshotRequest struct {
	ProjectID string `json:"project_id" binding:"required"`
	Label     string `json:"label"`
}

func (s *Server) handleCreateSnapshot(c *gin.Context) {
	var req CreateSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if !extractProjectIDFromBody(c, req.ProjectID) {
		return
	}

	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to get store", err))
		return
	}

	diffService := service.NewGraphDiffService()
	snap, err := diffService.TakeSnapshot(c.Request.Context(), store, req.ProjectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to take snapshot", err))
		return
	}

	snapshotsDir := filepath.Join(s.sourceDir, req.ProjectID, "snapshots")
	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to create snapshots directory", err))
		return
	}

	snapshotID := fmt.Sprintf("snap_%d", time.Now().UnixNano())
	if req.Label != "" {
		snapshotID = fmt.Sprintf("snap_%s_%d", req.Label, time.Now().UnixNano())
	}
	snapshotPath := filepath.Join(snapshotsDir, snapshotID+".json")

	if err := diffService.SaveSnapshot(snap, snapshotPath); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to save snapshot", err))
		return
	}

	c.JSON(http.StatusCreated, SnapshotInfo{
		ID:        snapshotID,
		Path:      snapshotPath,
		Timestamp: snap.Timestamp,
		NodeCount: snap.NodeCount,
		EdgeCount: snap.EdgeCount,
	})
}

func (s *Server) handleListSnapshots(c *gin.Context) {
	projectID, ok := extractProjectID(c)
	if !ok {
		return
	}

	snapshotsDir := filepath.Join(s.sourceDir, projectID, "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, []SnapshotInfo{})
			return
		}
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to read snapshots directory", err))
		return
	}

	diffService := service.NewGraphDiffService()
	snapshots := make([]SnapshotInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		snapPath := filepath.Join(snapshotsDir, entry.Name())
		snap, err := diffService.LoadSnapshot(snapPath)
		if err != nil {
			continue
		}
		id := entry.Name()
		if len(id) > 5 {
			id = id[:len(id)-5]
		}
		snapshots = append(snapshots, SnapshotInfo{
			ID:        id,
			Path:      snapPath,
			Timestamp: snap.Timestamp,
			NodeCount: snap.NodeCount,
			EdgeCount: snap.EdgeCount,
		})
	}

	c.JSON(http.StatusOK, snapshots)
}

type IncrementalIngestRequest struct {
	ProjectID  string `json:"project_id" binding:"required"`
	SourceDir  string `json:"source_dir" binding:"required"`
	FromCommit string `json:"from_commit"`
	ToCommit   string `json:"to_commit"`
	SkipEmbed  bool   `json:"skip_embed"`
}

func (s *Server) handleIncrementalIngest(c *gin.Context) {
	if s.manager.ReadOnly() {
		c.JSON(http.StatusConflict, gin.H{
			"error": "server is running in read-only mode; start with --writable (or GCA_WRITABLE=true) to run incremental ingest",
		})
		return
	}

	var req IncrementalIngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if !extractProjectIDFromBody(c, req.ProjectID) {
		return
	}

	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to get store", err))
		return
	}

	opts := &ingest.IngestOptions{
		SkipEmbeddings: req.SkipEmbed,
		FromCommit:     req.FromCommit,
		ToCommit:       req.ToCommit,
	}

	state := ingest.NewIngestState()
	if err := ingest.RunIncrementalWithOptions(store, req.ProjectID, req.SourceDir, state, opts); err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to run incremental ingest", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_id": req.ProjectID,
		"status":     "completed",
		"symbols":    len(state.SymbolTable),
		"files":      len(state.FileIndex),
	})
}

type TestGenerateRequest struct {
	Target string `json:"target"`
	Query  string `json:"query"`
	Depth  int    `json:"depth"`
}

func (s *Server) handleTestGenerate(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}

	var req TestGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	aiReq := ai.AIRequest{
		ProjectID: projectID,
		Task:      string(ooda.TaskTestGeneration),
		Query:     req.Query,
		SymbolID:  req.Target,
		Data:      map[string]any{"depth": req.Depth},
	}

	result, err := s.aiService.HandleRequestOODA(c.Request.Context(), aiReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "test generation failed", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"answer": result})
}

type TestGenerateAllRequest struct {
	Depth int `json:"depth"`
}

type TestGenerateAllResponse struct {
	Results   map[string]string `json:"results"`
	Errors    map[string]string `json:"errors"`
	Total     int               `json:"total"`
	Generated int               `json:"generated"`
	Failed    int               `json:"failed"`
}

func (s *Server) handleTestGenerateAll(c *gin.Context) {
	projectID := c.Param("projectId")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}

	var req TestGenerateAllRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("handleTestGenerateAll: invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	store, err := s.manager.GetStore(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	var handlers []string
	for fact := range store.ScanContext(c.Request.Context(), "", "has_role", "api_handler") {
		handlers = append(handlers, fact.Subject)
	}

	if len(handlers) == 0 {
		c.JSON(http.StatusOK, TestGenerateAllResponse{
			Results:   map[string]string{},
			Errors:    map[string]string{},
			Total:     0,
			Generated: 0,
			Failed:    0,
		})
		return
	}

	results := make(map[string]string)
	errors := make(map[string]string)
	generated := 0
	failed := 0

	const maxConcurrency = 3
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, handler := range handlers {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			aiReq := ai.AIRequest{
				ProjectID: projectID,
				Task:      string(ooda.TaskTestGeneration),
				SymbolID:  h,
				Data:      map[string]any{"depth": req.Depth},
			}

			result, err := s.aiService.HandleRequestOODA(c.Request.Context(), aiReq)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errors[h] = err.Error()
				failed++
			} else {
				results[h] = result
				generated++
			}
		}(handler)
	}
	wg.Wait()

	c.JSON(http.StatusOK, TestGenerateAllResponse{
		Results:   results,
		Errors:    errors,
		Total:     len(handlers),
		Generated: generated,
		Failed:    failed,
	})
}

// handleCreateReviewSession creates an ephemeral review session from a git diff.
// POST /api/v1/review/session
// Request: { project_id, diff, base_commit?, head_commit? }
func (s *Server) handleCreateReviewSession(c *gin.Context) {
	var req struct {
		ProjectID  string `json:"project_id"`
		Diff       string `json:"diff"`
		BaseCommit string `json:"base_commit,omitempty"`
		HeadCommit string `json:"head_commit,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}
	if req.ProjectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}
	if req.Diff == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "diff is required"})
		return
	}

	session, count, err := s.ephemeralStore.ParseDiffAndCreateSession(req.ProjectID, req.Diff)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session", "details": err.Error()})
		return
	}

	logger.Info("Review session created",
		"session_id", session.ID,
		"project_id", req.ProjectID,
		"facts", count)

	c.JSON(http.StatusOK, gin.H{
		"session_id":   session.ID,
		"project_id":   req.ProjectID,
		"expires_at":   session.ExpiresAt.Format(time.RFC3339),
		"facts_parsed": count,
		"base_commit":  req.BaseCommit,
		"head_commit":  req.HeadCommit,
	})
}

// handleReviewSessionQuery runs a federated query against an existing review session.
// POST /api/v1/review/session/:id/query
// Request: { query, project_id }
func (s *Server) handleReviewSessionQuery(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}

	var req struct {
		Query     string `json:"query"`
		ProjectID string `json:"project_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.ProjectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required for source/analytical store query"})
		return
	}

	fedReq := ephemeral.FederatedQueryRequest{
		SessionID: sessionID,
		ProjectID: req.ProjectID,
		Query:     req.Query,
	}

	result, err := ephemeral.FederatedQuery(c.Request.Context(), fedReq, s.ephemeralStore, s.manager)
	if err != nil {
		if stdErrors.Is(err, ephemeral.ErrSessionExpired) || ephemeral.IsNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	logger.Info("Federated query executed",
		"session_id", sessionID,
		"ephemeral", len(result.Ephemeral),
		"source", len(result.Source),
		"analytical", len(result.Analytical))

	c.JSON(http.StatusOK, result)
}

// handleRoutes returns all detected API routes from the codebase.
// Query parameters:
//   - project: project ID (auto-detected if omitted)
//
// Response: JSON array of route objects with method, path, and handler.
func (s *Server) handleRoutes(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		projects, err := s.graphService.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}
	if projectID == "" {
		c.JSON(http.StatusOK, gin.H{"routes": []interface{}{}})
		return
	}

	store, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		handleError(c, apperrors.NewAppError(http.StatusInternalServerError, "failed to get store", err))
		return
	}

	routes := make(map[string]map[string]string) // route -> {method, handler}

	for fact := range store.ScanContext(c.Request.Context(), "", config.PredicateHandledBy, "") {
		route := fact.Subject
		if handler, ok := fact.Object.(string); ok {
			if routes[route] == nil {
				routes[route] = make(map[string]string)
			}
			routes[route]["handler"] = handler
			routes[route]["path"] = route
		}
	}

	for fact := range store.ScanContext(c.Request.Context(), "", "http_method", "") {
		route := fact.Subject
		if method, ok := fact.Object.(string); ok {
			if routes[route] == nil {
				routes[route] = make(map[string]string)
			}
			routes[route]["method"] = method
		}
	}

	result := make([]map[string]string, 0, len(routes))
	for _, r := range routes {
		if r["method"] == "" {
			r["method"] = "ANY"
		}
		result = append(result, r)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i]["path"] < result[j]["path"]
	})

	c.JSON(http.StatusOK, gin.H{"routes": result})
}
