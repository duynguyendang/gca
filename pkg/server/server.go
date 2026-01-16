package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/export"
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
type GraphNode struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Group string `json:"group"`
}

type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

func transformToGraph(results []map[string]any) GraphResponse {
	nodeMap := make(map[string]GraphNode)
	var links []GraphLink

	for _, row := range results {
		// Assume triples(S, P, O) format roughly.
		// We really need to know WHICH variables map to S, P, O.
		// The query engine returns map[string]any.
		// Let's assume the query was something like triples(S, P, O).
		// But the input is generic Datalog.
		// In the requirement: "Transform the resulting triples into a flat graph format".
		// This implies the query MUST return S, P, O or similar structures.
		// Let's look for known variable names: S, P, O, Subject, Predicate, Object?
		// Or just map *any* relation found.

		// Heuristic: If we have at least 2 variables, treat as link?
		// Or if we specifically look for 'S', 'P', 'O'.
		// Let's try to find keys that look like Subject/Object.
		// Or simply: Any 2 values in a row form a link? That's risky.

		// Re-reading logic: "Transform the resulting triples into a flat graph format: { nodes: [], links: [] }."
		// This suggests the API expects the query to output "triples".
		// If the user queries `triples(?S, ?P, ?O)`, we get S, P, O.

		var subj, pred, obj string

		// Try to identify S, P, O from row
		if s, ok := row["S"]; ok {
			subj = fmt.Sprint(s)
		} else if s, ok := row["Subject"]; ok {
			subj = fmt.Sprint(s)
		}

		if p, ok := row["P"]; ok {
			pred = fmt.Sprint(p)
		} else if p, ok := row["Predicate"]; ok {
			pred = fmt.Sprint(p)
		}

		if o, ok := row["O"]; ok {
			obj = fmt.Sprint(o)
		} else if o, ok := row["Object"]; ok {
			obj = fmt.Sprint(o)
		}

		// If we didn't find them by name, maybe it's just implicit?
		// Let's fallback to "S, P, O" exact match as primary requirement for graph view.
		if subj != "" && obj != "" {
			if pred == "" {
				pred = "related"
			}

			// Add Nodes
			if _, exists := nodeMap[subj]; !exists {
				nodeMap[subj] = createNode(subj)
			}
			if _, exists := nodeMap[obj]; !exists {
				nodeMap[obj] = createNode(obj)
			}

			// Add Link
			links = append(links, GraphLink{
				Source: subj,
				Target: obj,
				Label:  pred,
			})
		}
	}

	var nodes []GraphNode
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	return GraphResponse{
		Nodes: nodes,
		Links: links,
	}
}

func createNode(id string) GraphNode {
	// Logic: set id as full path, name as short filename/symbol.
	// Enrich with kind (func, struct) and group (package).
	// Start with basic defaults.

	name := filepath.Base(id)
	group := filepath.Dir(id)
	kind := "unknown"

	// Heuristics for Kind based on ID format?
	// e.g., "mypkg/file.go:MyFunc" -> Kind: Code
	// This is "Smart Labeling".
	if strings.Contains(id, ".go") {
		kind = "file"
		if strings.Contains(id, ":") {
			kind = "symbol"
			parts := strings.Split(id, ":")
			if len(parts) > 1 {
				name = parts[1]
			}
		}
	}

	return GraphNode{
		ID:    id,
		Name:  name,
		Kind:  kind,
		Group: group,
	}
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

	// Verify ID exists using LookUpID
	if _, exists := store.LookupID(id); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "File ID not found in index"})
		return
	}

	// Fetch source code from "has_source_code" predicate
	// We scan for the source code fact.
	var content string
	found := false
	for fact, _ := range store.Scan(id, "has_source_code", "", "") {
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
