package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/gin-gonic/gin"
)

// handleProjects returns a list of available projects.
// handleProjects returns a list of available projects.
func (s *Server) handleProjects(c *gin.Context) {
	projects, err := s.graphService.ListProjects()
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, projects)
}

// handleQuery executes a Datalog query and returns the results in a graph format.
func (s *Server) handleQuery(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Invalid request body", err))
		return
	}

	// If query is empty, return empty graph to prevent frontend crashes
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusOK, gin.H{"nodes": []interface{}{}, "links": []interface{}{}})
		return
	}

	projectID := c.Query("project")
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

	// Auto-cluster if too many nodes (>500)
	if autocluster && len(graph.Nodes) > 500 {
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
func (s *Server) handleGraph(c *gin.Context) {
	projectID := c.Query("project")
	fileID := c.Query("file")
	lazy := c.Query("lazy") == "true"

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}
	if fileID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing file ID", nil))
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
func (s *Server) handleSource(c *gin.Context) {
	id := c.Query("id")
	projectID := c.Query("project")

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}

	content, err := s.graphService.GetSource(projectID, id)
	if err != nil {
		handleError(c, err)
		return
	}

	// Handle line range extraction if requested
	// Keep presentation logic in handler? Or move to service?
	// Line range is view logic, maybe keep here or helper.
	// Check lines...
	// ... (Existing slice logic) ...

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
	// ... (Normalization logic) ...
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

	results, err := s.graphService.GetPredicates(projectID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"predicates": results})
}

// handleSymbols provides fast symbol search/autocomplete.
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

	predicate := c.Query("p")
	if predicate == "" && c.Query("all") != "true" {
		predicate = "defines" // Hardcoded literal instead of importing meb? Or import meb just for constant if needed.
		// meb is not imported yet, I removed it.
		// Let's assume "defines" string or import meb if critical.
		// The original code used meb.PredDefines.
		// GraphService doesn't expose constant.
		// Let's just use string "defines".
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

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
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
	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}

	autocluster := c.Query("nocluster") != "true"

	graph, err := s.graphService.GetProjectMap(c.Request.Context(), projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	// Auto-cluster if too many nodes (>500)
	if autocluster && len(graph.Nodes) > 500 {
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
	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
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

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}
	if fileID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing file ID", nil))
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
	if id == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing id parameter", nil))
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
	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
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

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}
	if id == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing id parameter", nil))
		return
	}

	depth := 3
	if depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil {
			depth = d
		}
	}

	graph, err := s.graphService.GetFileCalls(c.Request.Context(), projectID, id, depth)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}

// handleError helper
func handleError(c *gin.Context, err error) {
	appErr := errors.MapError(err)
	c.JSON(appErr.Code, gin.H{"error": appErr.Message})
}

// handleFlowPath returns the shortest call graph path between two symbols/files.
func (s *Server) handleFlowPath(c *gin.Context) {
	projectID := c.Query("project")
	from := c.Query("from")
	to := c.Query("to")

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}
	if from == "" || to == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing from/to parameters", nil))
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

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}
	if source == "" || target == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing source/target parameters", nil))
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
// GET /v1/semantic-search?project=X&q=query&k=10
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

	if projectID == "" || query == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project or q parameter", nil))
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

	if projectID == "" || query == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project or query parameter", nil))
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
	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}

	graph, err := s.graphService.GetSubgraph(c.Request.Context(), projectID, req.Ids)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
}
