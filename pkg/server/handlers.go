package server

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/gin-gonic/gin"
)

// validateAndSanitizeQuery validates and sanitizes a Datalog query string
func validateAndSanitizeQuery(query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Trim whitespace
	query = strings.TrimSpace(query)

	// Check for excessively long queries
	if len(query) > config.MaxQueryLength {
		return "", fmt.Errorf("query exceeds maximum length of %d characters", config.MaxQueryLength)
	}

	// Sanitize HTML entities to prevent XSS
	query = html.EscapeString(query)

	// Check for potentially dangerous patterns
	dangerousPatterns := []string{
		"<script",
		"javascript:",
		"onload=",
		"onerror=",
		"onclick=",
	}

	lowerQuery := strings.ToLower(query)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerQuery, pattern) {
			return "", fmt.Errorf("query contains potentially dangerous content")
		}
	}

	return query, nil
}

// validateProjectID validates a project ID
func validateProjectID(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(projectID, "..") || strings.Contains(projectID, "/") || strings.Contains(projectID, "\\") {
		return fmt.Errorf("invalid project ID format")
	}

	// Check for excessively long project IDs
	if len(projectID) > config.MaxProjectIDLength {
		return fmt.Errorf("project ID exceeds maximum length")
	}

	return nil
}

// validateSymbolID validates a symbol ID
func validateSymbolID(symbolID string) error {
	if symbolID == "" {
		return fmt.Errorf("symbol ID cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(symbolID, "..") {
		return fmt.Errorf("invalid symbol ID format")
	}

	// Check for excessively long symbol IDs
	if len(symbolID) > config.MaxSymbolIDLength {
		return fmt.Errorf("symbol ID exceeds maximum length")
	}

	return nil
}

// validateIDs validates a list of IDs
func validateIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("IDs list cannot be empty")
	}

	if len(ids) > config.MaxIDsCount {
		return fmt.Errorf("too many IDs (maximum %d)", config.MaxIDsCount)
	}

	for _, id := range ids {
		if err := validateSymbolID(id); err != nil {
			return err
		}
	}

	return nil
}

// validateEmbedding validates an embedding vector
func validateEmbedding(embedding []float32) error {
	if len(embedding) == 0 {
		return fmt.Errorf("embedding cannot be empty")
	}

	// Check for reasonable embedding dimensions
	if len(embedding) > config.MaxEmbeddingDim {
		return fmt.Errorf("embedding dimensions exceed maximum")
	}

	return nil
}

// validateLimit validates a limit parameter
func validateLimit(limit int, maxLimit int) error {
	if limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}

	if limit > maxLimit {
		return fmt.Errorf("limit exceeds maximum of %d", maxLimit)
	}

	return nil
}

// validateOffset validates an offset parameter
func validateOffset(offset int) error {
	if offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}

	if offset > config.MaxOffset {
		return fmt.Errorf("offset exceeds maximum")
	}

	return nil
}

// validateCursor validates a cursor string
func validateCursor(cursor string) error {
	if cursor == "" {
		return nil // Empty cursor is valid
	}

	// Check for excessively long cursors
	if len(cursor) > config.MaxCursorLength {
		return fmt.Errorf("cursor exceeds maximum length")
	}

	// Check for dangerous patterns
	dangerousPatterns := []string{
		"<script",
		"javascript:",
		"onload=",
		"onerror=",
	}

	lowerCursor := strings.ToLower(cursor)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCursor, pattern) {
			return fmt.Errorf("cursor contains potentially dangerous content")
		}
	}

	return nil
}

// sanitizeString sanitizes a string input
func sanitizeString(input string) string {
	// Trim whitespace
	input = strings.TrimSpace(input)

	// Sanitize HTML entities
	input = html.EscapeString(input)

	return input
}

// validateDepth validates a depth parameter
func validateDepth(depth int) error {
	if depth < 0 {
		return fmt.Errorf("depth cannot be negative")
	}

	if depth > config.MaxDepth {
		return fmt.Errorf("depth exceeds maximum of %d", config.MaxDepth)
	}

	return nil
}

// validateClusters validates a clusters parameter
func validateClusters(clusters int) error {
	if clusters <= 0 {
		return fmt.Errorf("clusters must be positive")
	}

	if clusters > config.MaxClusters {
		return fmt.Errorf("clusters exceeds maximum of %d", config.MaxClusters)
	}

	return nil
}

// isValidQueryPattern checks if a query pattern is valid
func isValidQueryPattern(query string) bool {
	// Basic validation - query should contain parentheses for Datalog
	if !strings.Contains(query, "(") || !strings.Contains(query, ")") {
		return false
	}

	// Check for balanced parentheses
	count := 0
	for _, char := range query {
		if char == '(' {
			count++
		} else if char == ')' {
			count--
		}
		if count < 0 {
			return false
		}
	}

	return count == 0
}

// handleProjects returns a list of available projects.
// Query parameters: none
// Response: JSON array of project objects with id, name, and metadata.
func (s *Server) handleProjects(c *gin.Context) {
	projects, err := s.graphService.ListProjects()
	if err != nil {
		fmt.Printf("handleProjects error: %v\n", err)
		c.JSON(500, gin.H{"error": err.Error()})
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
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	// Validate and sanitize query
	sanitizedQuery, err := validateAndSanitizeQuery(req.Query)
	if err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

	// If query is empty, return empty graph to prevent frontend crashes
	if sanitizedQuery == "" {
		c.JSON(http.StatusOK, gin.H{"nodes": []interface{}{}, "links": []interface{}{}})
		return
	}

	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	lazy := c.Query("lazy") == "true"
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
	graph, err := s.graphService.ExportGraph(c.Request.Context(), projectID, req.Query, true, lazy)
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
	projectID := c.Query("project")
	fileID := c.Query("file")
	lazy := c.Query("lazy") == "true"

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(fileID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

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
	id := c.Query("id")
	projectID := c.Query("project")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(id); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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

	// If no project specified, try to pick the first one available
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

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

	// Validate and sanitize query parameter
	query = sanitizeString(query)
	if len(query) > config.MaxSearchQueryLength {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "query exceeds maximum length", nil))
		return
	}

	predicate := c.Query("p")
	if predicate == "" && c.Query("all") != "true" {
		predicate = config.PredicateDefines
	}

	// Validate predicate parameter
	if predicate != "" {
		predicate = sanitizeString(predicate)
		if len(predicate) > config.MaxPredicateLength {
			handleError(c, errors.NewAppError(http.StatusBadRequest, "predicate exceeds maximum length", nil))
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
	projectID := c.Query("project")
	prefix := c.Query("prefix")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

	// Validate and sanitize prefix parameter
	if prefix != "" {
		// Sanitize the prefix
		prefix = sanitizeString(prefix)

		// Check for path traversal attempts
		if strings.Contains(prefix, "..") || strings.Contains(prefix, "\\") {
			handleError(c, errors.NewAppError(http.StatusBadRequest, "Invalid prefix format", nil))
			return
		}

		// Check for excessively long prefixes
		if len(prefix) > config.MaxPrefixLength {
			handleError(c, errors.NewAppError(http.StatusBadRequest, "prefix exceeds maximum length", nil))
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
	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	fileID := c.Query("file")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(fileID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	id := c.Query("id")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(id); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	id := c.Query("id")
	depthStr := c.Query("depth")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(id); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

	depth := config.PathfinderDepthLimit
	if depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil {
			depth = d
		}
	}
	// Enforce max depth for performance
	if depth > 2 {
		depth = 2
	}

	graph, err := s.graphService.GetFileCalls(c.Request.Context(), projectID, id, depth)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleError is a helper that converts errors to JSON responses.
// It uses the errors.MapError function to convert errors to AppError with HTTP status codes.
func handleError(c *gin.Context, err error) {
	appErr := errors.MapError(err)
	c.JSON(appErr.Code, gin.H{"error": appErr.Message})
}

// handleFlowPath returns the shortest call graph path between two symbols/files.
func (s *Server) handleFlowPath(c *gin.Context) {
	projectID := c.Query("project")
	from := c.Query("from")
	to := c.Query("to")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(from); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(to); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	source := c.Query("source")
	target := c.Query("target")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(source); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if err := validateSymbolID(target); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	query := c.Query("q")
	kStr := c.DefaultQuery("k", "10")

	k, err := strconv.Atoi(kStr)
	if err != nil || k <= 0 {
		k = 10
	}
	if k > 50 {
		k = 50 // Cap results
	}

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if query == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing q parameter", nil))
		return
	}

	// Validate and sanitize query
	query = sanitizeString(query)
	if len(query) > config.MaxQueryLength {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "query exceeds maximum length", nil))
		return
	}

	// Get embedding for query using Gemini Service
	if s.geminiService == nil {
		handleError(c, errors.NewAppError(http.StatusServiceUnavailable, "AI service not initialized", nil))
		return
	}

	results, err := s.graphService.SemanticSearch(c.Request.Context(), projectID, query, k, s.geminiService)
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
	projectID := c.Query("project")
	query := c.Query("query")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if query == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing query parameter", nil))
		return
	}

	// Validate and sanitize query
	query = sanitizeString(query)
	if len(query) > config.MaxQueryLength {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "query exceeds maximum length", nil))
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
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

	// Validate IDs list
	if err := validateIDs(req.Ids); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

	var req struct {
		Embedding []float32 `json:"embedding"`
		Limit     int       `json:"limit"`
		Clusters  int       `json:"clusters"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	// Validate embedding
	if err := validateEmbedding(req.Embedding); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	if err := validateLimit(req.Limit, 1000); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}

	// Validate clusters
	if err := validateClusters(req.Clusters); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
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
	projectID := c.Query("project")
	query := c.Query("query")

	if err := validateProjectID(projectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, err.Error(), err))
		return
	}
	if query == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing query parameter", nil))
		return
	}

	// Get the full graph first (in production, this should be optimized to only fetch needed data)
	graph, err := s.graphService.ExportGraph(c.Request.Context(), projectID, query, true, false)
	if err != nil {
		handleError(c, err)
		return
	}

	// Parse pagination options
	cursorStr := c.Query("cursor")
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))

	cursor, err := export.ParseCursor(cursorStr)
	if err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Invalid cursor format", err))
		return
	}

	// Use cursor offset if provided, otherwise use query offset
	if cursor.Offset > 0 && offset == 0 {
		offset = cursor.Offset
	}
	if limit > 0 {
		cursor.Limit = limit
	}

	// Paginate the graph
	opts := export.GraphPageOptions{
		Limit:  cursor.Limit,
		Offset: offset,
	}

	paginatedGraph, _ := graph.PaginateGraph(opts)
	c.JSON(http.StatusOK, paginatedGraph)
}
