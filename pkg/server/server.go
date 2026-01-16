package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/repl"
	"github.com/gin-gonic/gin"
)

// Server holds the state for the REST API server.
type Server struct {
	manager   *manager.StoreManager
	sourceDir string
	router    *gin.Engine
}

// NewServer creates a new Server instance.
func NewServer(mgr *manager.StoreManager, sourceDir string) *Server {
	r := gin.Default()
	s := &Server{
		manager:   mgr,
		sourceDir: sourceDir,
		router:    r,
	}
	s.setupRoutes()
	return s
}

// Run starts the server on the specified address.
func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

func (s *Server) setupRoutes() {
	s.router.GET("/health", s.healthCheck)
	s.router.GET("/v1/projects", s.handleProjects)
	s.router.POST("/v1/query", s.handleQuery)
	s.router.GET("/v1/source", s.handleSource)
	s.router.GET("/v1/summary", s.handleSummary)
	s.router.GET("/v1/predicates", s.handlePredicates)
	s.router.GET("/v1/symbols", s.handleSymbols)
}

// healthCheck returns 200 OK.
func (s *Server) healthCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}

// handleProjects returns a list of available projects.
func (s *Server) handleProjects(c *gin.Context) {
	projects, err := s.manager.ListProjects()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list projects: %v", err)})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Get project ID from query parameter
	projectID := c.Query("project")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'project' query parameter"})
		return
	}

	store, err := s.manager.GetStore(projectID)
	if err != nil {
		if os.IsNotExist(err) || err.Error() == fmt.Sprintf("project not found: %s", projectID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to load project: %v", err)})
		}
		return
	}

	results, err := store.Query(c.Request.Context(), req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Query execution failed: %v", err)})
		return
	}

	// Use the shared D3 exporter
	graph, err := export.ExportD3(c.Request.Context(), store, req.Query, results)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Graph transformation failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, graph)
}

// Graph structures

// handleSource returns source code for a given file ID.
func (s *Server) handleSource(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'id' parameter"})
		return
	}

	projectID := c.Query("project")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'project' query parameter"})
		return
	}

	store, err := s.manager.GetStore(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Verify ID exists using LookUpID
	if _, exists := store.LookupID(id); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "File ID not found in index"})
		return
	}

	// Fetch source code from "has_source_code" predicate
	// We scan for the source code fact.
	var content string
	found := false
	for fact := range store.Scan(id, meb.PredHasSourceCode, "", "") {
		if str, ok := fact.Object.(string); ok {
			content = str
			found = true
			break
		}
	}

	if !found {
		// Fallback or error?
		// If "has_source_code" is missing, maybe it wasn't ingested with code?
		// We return 404 or empty.
		c.JSON(http.StatusNotFound, gin.H{"error": "Source code not available for this ID"})
		return
	}

	// Handle line range extraction if requested
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
	if start < 1 {
		start = 1
	}
	if end == -1 || end > len(lines) {
		end = len(lines)
	}

	if start > len(lines) {
		c.String(http.StatusOK, "")
		return
	}

	// Adjust 0-based
	slice := lines[start-1 : end]
	result := strings.Join(slice, "\n")

	c.String(http.StatusOK, result)
}

// handleSummary returns the project summary.
func (s *Server) handleSummary(c *gin.Context) {
	projectID := c.Query("project")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'project' query parameter"})
		return
	}

	store, err := s.manager.GetStore(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	summary, err := repl.GenerateProjectSummary(store)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate summary: %v", err)})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// handlePredicates returns the list of active predicates in the database.
func (s *Server) handlePredicates(c *gin.Context) {
	projectID := c.Query("project")

	// If no project specified, try to pick the first one available
	if projectID == "" {
		projects, err := s.manager.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}

	if projectID == "" {
		c.JSON(http.StatusOK, gin.H{"predicates": []map[string]string{}})
		return
	}

	store, err := s.manager.GetStore(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	activePreds, err := store.GetAllPredicates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get predicates: %v", err)})
		return
	}

	// Static metadata for known system predicates provided by meb.SystemPredicates
	// (Map removed)

	var results []map[string]string
	for _, p := range activePreds {
		// Only include predicates that are in the SystemPredicates registry (Whitelist)
		if m, ok := meb.SystemPredicates[p]; ok {
			results = append(results, map[string]string{
				"name":        p,
				"description": m.Description,
				"example":     m.Example,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"predicates": results})
}

// handleSymbols provides fast symbol search/autocomplete.
func (s *Server) handleSymbols(c *gin.Context) {
	projectID := c.Query("project")
	query := c.Query("q")

	if projectID == "" {
		// Default to first project if available, logic similar to predicates?
		// Or keep strictly requiring projectID for search.
		// Let's allow default to match predicates behavior for consistency/ease of use.
		projects, err := s.manager.ListProjects()
		if err == nil && len(projects) > 0 {
			projectID = projects[0].ID
		}
	}

	if projectID == "" {
		c.JSON(http.StatusOK, gin.H{"symbols": []string{}})
		return
	}

	store, err := s.manager.GetStore(projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Limit results to 50 for autocomplete
	// Use 'p' param for predicate filter, default to 'defines_symbol' to show only defined symbols by default.
	// This matches user expectation of "symbols not facts".
	predicate := c.Query("p")
	if predicate == "" && c.Query("all") != "true" {
		predicate = meb.PredDefines
	}

	results, err := store.SearchSymbols(query, 50, predicate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"symbols": results})
}
