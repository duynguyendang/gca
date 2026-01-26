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

	// Delegate to service
	graph, err := s.graphService.ExportGraph(c.Request.Context(), projectID, req.Query, true, lazy)
	if err != nil {
		handleError(c, err)
		return
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
func (s *Server) handleFiles(c *gin.Context) {
	projectID := c.Query("project")

	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}

	files, err := s.graphService.ListFiles(projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}

// handleGraphMap returns a high-level view of file dependencies.
func (s *Server) handleGraphMap(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "Missing project ID", nil))
		return
	}

	graph, err := s.graphService.GetProjectMap(c.Request.Context(), projectID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, graph)
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

	graph, err := s.graphService.GetBackboneGraph(c.Request.Context(), projectID)
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
