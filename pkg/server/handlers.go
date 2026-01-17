package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/repl"
	"github.com/gin-gonic/gin"
)

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

	// Hydrate Nodes
	// Collect IDs
	if len(graph.Nodes) > 0 {
		ids := make([]meb.DocumentID, len(graph.Nodes))
		for i, n := range graph.Nodes {
			ids[i] = meb.DocumentID(n.ID)
		}

		// Hydrate
		hydrated, err := store.Hydrate(c.Request.Context(), ids)
		if err != nil {
			// Log warning but return graph? Or fail?
			// Let's log and proceed with partial hydration if possible, but Hydrate returns all or error.
			// Ideally we shouldn't fail the whole query if hydration fails for some reason, but errgroup returns first error.
			// Let's return error for now to be safe.
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Hydration failed: %v", err)})
			return
		}

		// Map back to nodes
		hMap := make(map[meb.DocumentID]meb.HydratedSymbol)
		for _, h := range hydrated {
			hMap[h.ID] = h
		}

		for i := range graph.Nodes {
			n := &graph.Nodes[i]
			if h, ok := hMap[meb.DocumentID(n.ID)]; ok {
				n.Code = h.Content
				if h.Kind != "" {
					n.Kind = h.Kind
				}
				// Metadata to other fields if needed?
				// D3Node has Language. We can infer from metadata if present?
				// h.Metadata might have "language".
			}
		}
	}

	c.JSON(http.StatusOK, graph)
}

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

	// Use Hydrator (GetDocument)
	doc, err := store.GetDocument(meb.DocumentID(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	content := string(doc.Content)

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
	if start > end {
		c.String(http.StatusOK, "")
		return
	}

	// Adjust 0-based
	if end > len(lines) {
		end = len(lines)
	}
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
