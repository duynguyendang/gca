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
	s.router.POST("/v1/query", s.handleQuery)
	s.router.GET("/v1/source", s.handleSource)
	s.router.GET("/v1/summary", s.handleSummary)
	s.router.GET("/v1/predicates", s.handlePredicates)
	s.router.GET("/v1/symbols", s.handleSymbols)
}

// Health check
func (s *Server) healthCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}
