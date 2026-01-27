package server

import (
	"net/http"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/gin-gonic/gin"
)

// Server holds the state for the REST API server.
type Server struct {
	manager      *manager.StoreManager
	graphService *service.GraphService
	sourceDir    string
	router       *gin.Engine
}

// NewServer creates a new Server instance.
func NewServer(mgr *manager.StoreManager, sourceDir string) *Server {
	r := gin.Default()
	r.Use(CORSMiddleware())

	svc := service.NewGraphService(mgr)

	s := &Server{
		manager:      mgr,
		graphService: svc,
		sourceDir:    sourceDir,
		router:       r,
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
	s.router.GET("/v1/graph", s.handleGraph)
	s.router.GET("/v1/graph/map", s.handleGraphMap)
	s.router.GET("/v1/graph/file-details", s.handleFileDetails)
	s.router.GET("/v1/graph/file-calls", s.handleFileCalls)
	s.router.GET("/v1/graph/backbone", s.handleGraphBackbone)
	s.router.GET("/v1/graph/file-backbone", s.handleFileBackbone)
	s.router.GET("/v1/hydrate", s.handleHydrate)
	s.router.POST("/v1/query", s.handleQuery)
	s.router.GET("/v1/source", s.handleSource)
	s.router.GET("/v1/summary", s.handleSummary)
	s.router.GET("/v1/predicates", s.handlePredicates)
	s.router.GET("/v1/symbols", s.handleSymbols)
	s.router.GET("/v1/files", s.handleFiles)
	s.router.GET("/v1/search/flow", s.handleFlowPath)
}

// Health check
func (s *Server) healthCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}

// CORSMiddleware handles CORS headers.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
