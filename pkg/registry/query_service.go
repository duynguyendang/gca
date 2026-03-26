package registry

import (
	"fmt"
	"net/http"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/gin-gonic/gin"
)

// QueryService handles HTTP requests for pre-defined queries
type QueryService struct {
	registry *QueryRegistry
}

// NewQueryService creates a new query service
func NewQueryService(registry *QueryRegistry) *QueryService {
	return &QueryService{
		registry: registry,
	}
}

// ExecuteQueryRequest represents a request to execute a pre-defined query
type ExecuteQueryRequest struct {
	QueryName string                 `json:"query_name" binding:"required"`
	Params    map[string]interface{} `json:"params"`
	ProjectID string                 `json:"project_id" binding:"required"`
}

// ExecuteQueryResponse represents the response from a query execution
type ExecuteQueryResponse struct {
	QueryName  string                 `json:"query_name"`
	Query      string                 `json:"query"`
	Results    []map[string]any       `json:"results"`
	Count      int                    `json:"count"`
	ExecutionTime int64               `json:"execution_time_ms"`
}

// ListQueriesResponse represents the response from listing queries
type ListQueriesResponse struct {
	Queries    []*QueryDefinition `json:"queries"`
	Categories map[string][]string `json:"categories"`
	Total      int                 `json:"total"`
}

// RegisterQueryRequest represents a request to register a new query
type RegisterQueryRequest struct {
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	Category    string            `json:"category" binding:"required"`
	Tier        int               `json:"tier" binding:"required,min=1,max=3"`
	Template    string            `json:"template" binding:"required"`
	Parameters  []QueryParameter `json:"parameters"`
	Examples    []QueryExample   `json:"examples"`
}

// ExecuteQuery handles the execution of a pre-defined query
// POST /api/v1/queries/execute
func (s *QueryService) ExecuteQuery(c *gin.Context) {
	var req ExecuteQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate query request
	if err := s.registry.ValidateQuery(c.Request.Context(), req.QueryName, req.Params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build query
	query, err := s.registry.ExecuteQuery(c.Request.Context(), req.QueryName, req.Params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Execute query via graph service
	// (This would integrate with the existing graph service)
	// For now, return the generated Datalog query
	c.JSON(http.StatusOK, gin.H{
		"query_name": req.QueryName,
		"query":      query,
		"params":     req.Params,
		"message":    "Query generated successfully. Execute with POST /api/v1/query",
	})
}

// ListQueries returns all available pre-defined queries
// GET /api/v1/queries
func (s *QueryService) ListQueries(c *gin.Context) {
	category := c.Query("category")

	var queries []*QueryDefinition
	if category != "" {
		queries = s.registry.ListQueriesByCategory(category)
	} else {
		queries = s.registry.ListQueries()
	}

	// Build categories map
	categories := make(map[string][]string)
	for _, def := range queries {
		categories[def.Category] = append(categories[def.Category], def.Name)
	}

	c.JSON(http.StatusOK, ListQueriesResponse{
		Queries:    queries,
		Categories: categories,
		Total:      len(queries),
	})
}

// GetQuery returns details for a specific query
// GET /api/v1/queries/:name
func (s *QueryService) GetQuery(c *gin.Context) {
	name := c.Param("name")

	def, err := s.registry.GetQuery(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, def)
}

// RegisterQuery registers a new pre-defined query
// POST /api/v1/queries
func (s *QueryService) RegisterQuery(c *gin.Context) {
	var req RegisterQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if query already exists
	if _, err := s.registry.GetQuery(req.Name); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("query '%s' already exists", req.Name)})
		return
	}

	// Add query to registry
	// (This would save to GenePool and reload)
	def := &QueryDefinition{
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Tier:        req.Tier,
		Template:    req.Template,
		Parameters:  req.Parameters,
		Examples:    req.Examples,
	}

	// TODO: Persist to GenePool
	// For now, just add to in-memory registry

	c.JSON(http.StatusCreated, gin.H{
		"message": "Query registered successfully",
		"query":   def,
	})
}

// ReloadQueries reloads queries from GenePool
// POST /api/v1/queries/reload
func (s *QueryService) ReloadQueries(c *gin.Context) {
	// Reload queries from GenePool
	err := s.registry.LoadQueriesFromGenePool(c.Request.Context(), config.GenePoolPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Queries reloaded successfully",
	})
}

// AddRoute adds query service routes to the router
func (s *QueryService) AddRoute(router *gin.Engine) {
	api := router.Group("/api/v1/queries")
	{
		api.POST("/execute", s.ExecuteQuery)
		api.GET("", s.ListQueries)
		api.GET("/:name", s.GetQuery)
		api.POST("", s.RegisterQuery)
		api.POST("/reload", s.ReloadQueries)
	}
}
