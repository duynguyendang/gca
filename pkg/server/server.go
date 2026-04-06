package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/agent"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/registry"
	"github.com/duynguyendang/gca/pkg/service"
	"github.com/duynguyendang/gca/pkg/service/ai"
	manglesdk "github.com/duynguyendang/manglekit/sdk"
	"github.com/gin-gonic/gin"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig returns a secure default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://localhost:5173",
			"http://localhost:8080",
		},
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"DELETE",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
			"X-Request-ID",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"X-Request-ID",
		},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}
}

// Server holds the state for the REST API server.
type Server struct {
	manager      *manager.StoreManager
	graphService *service.GraphService
	aiService    *ai.AIService
	mangleClient *manglesdk.Client
	queryService *registry.QueryService
	sourceDir    string
	router       *gin.Engine
}

// NewServer creates a new Server instance.
func NewServer(mgr *manager.StoreManager, sourceDir string) *Server {
	r := gin.Default()
	r.Use(RequestIDMiddleware())
	r.Use(CORSMiddleware())
	r.Use(RateLimitMiddleware())
	r.Use(ValidationMiddleware())
	r.Use(CompressionMiddleware())

	svc := service.NewGraphService(mgr)

	aiSvc, err := ai.NewAIService(context.Background(), mgr)
	if err != nil {
		log.Printf("Warning: Failed to initialize AI Service: %v. AI features disabled.", err)
		aiSvc = nil
	} else {
		log.Println("AI Service initialized successfully")
	}

	// Initialize Manglekit Client for GenePool queries
	mangleClient, err := manglesdk.NewClient(context.Background())
	if err != nil {
		log.Printf("Warning: Failed to initialize Manglekit Client: %v. Query features disabled.", err)
		mangleClient = nil
	} else {
		log.Println("Manglekit Client initialized successfully")

		// Load query policies
		policyPath := config.GenePoolPath
		if err := mangleClient.Engine().LoadPolicy(context.Background(), policyPath); err != nil {
			log.Printf("Warning: Failed to load query policies from %s: %v", policyPath, err)
		} else {
			log.Printf("Query policies loaded from %s", policyPath)
		}
	}

	// Initialize Query Service
	var queryService *registry.QueryService
	if mangleClient != nil {
		queryRegistry := registry.NewQueryRegistry(mangleClient.Engine())
		policyPath := config.GenePoolPath
		if err := queryRegistry.LoadQueriesFromGenePool(context.Background(), policyPath); err != nil {
			log.Printf("Warning: Failed to load query registry: %v", err)
		} else {
			log.Println("Query registry initialized successfully")
		}
		queryService = registry.NewQueryService(queryRegistry)
	}

	s := &Server{
		manager:      mgr,
		graphService: svc,
		aiService:    aiSvc,
		mangleClient: mangleClient,
		queryService: queryService,
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

// Handler returns the underlying HTTP handler (Gin engine).
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) setupRoutes() {
	s.router.GET("/api/health", s.healthCheck)
	s.router.GET("/api/v1/projects", s.handleProjects)
	s.router.GET("/api/v1/graph", s.handleGraph)
	s.router.GET("/api/v1/graph/paginated", s.handleGraphPaginated) // Lazy loading support
	s.router.GET("/api/v1/graph/manifest", s.handleGraphManifest)
	s.router.GET("/api/v1/graph/map", s.handleGraphMap)
	s.router.GET("/api/v1/graph/file-details", s.handleFileDetails)
	s.router.GET("/api/v1/graph/file-calls", s.handleFileCalls)
	s.router.GET("/api/v1/graph/backbone", s.handleGraphBackbone)
	s.router.GET("/api/v1/graph/file-backbone", s.handleFileBackbone)
	s.router.GET("/api/v1/hydrate", s.handleHydrate)
	s.router.POST("/api/v1/query", s.handleQuery)
	s.router.GET("/api/v1/source", s.handleSource)
	s.router.GET("/api/v1/summary", s.handleSummary)
	s.router.GET("/api/v1/predicates", s.handlePredicates)
	s.router.GET("/api/v1/symbols", s.handleSymbols)
	s.router.GET("/api/v1/files", s.handleFiles)
	s.router.GET("/api/v1/search/flow", s.handleFlowPath)
	s.router.GET("/api/v1/graph/path", s.handleGraphPath)
	s.router.GET("/api/v1/graph/cluster", s.handleGraphCluster)
	s.router.GET("/api/v1/semantic-search", s.handleSemanticSearch)
	s.router.GET("/api/v1/graph/communities", s.handleGraphCommunities)
	s.router.POST("/api/v1/graph/hybrid-cluster", s.handleHybridCluster)
	s.router.POST("/api/v1/graph/subgraph", s.handleGraphSubgraph)

	// AI Endpoints
	s.router.POST("/api/v1/ai/ask", s.handleAIAsk)

	// Agent Endpoint (multi-step reasoning)
	s.router.POST("/api/v1/agent/execute", s.handleAgentExecute)

	// Query Registry (GenePool pre-defined queries)
	if s.queryService != nil {
		s.queryService.AddRoute(s.router)
		log.Println("Query service routes registered")
	}
}

// AI Handler
func (s *Server) handleAIAsk(c *gin.Context) {
	var req ai.AIRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if s.aiService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service not initialized (missing API Key)"})
		return
	}

	if req.ProjectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ProjectID is required"})
		return
	}

	// Validate ProjectID
	if err := ValidateProjectID(req.ProjectID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate and sanitize Query
	if req.Query != "" {
		if err := ValidateQuery(req.Query); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.Query = SanitizeString(req.Query)
	}

	useOODA := os.Getenv("USE_OODA_LOOP") == "true"

	var answer string
	var err error

	if useOODA {
		answer, err = s.aiService.HandleRequestOODA(c.Request.Context(), req)
		if err != nil {
			log.Printf("AI OODA Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		answer, err = s.aiService.HandleRequest(c.Request.Context(), req)
		if err != nil {
			log.Printf("AI Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"answer": answer})
}

// Agent Execute Handler - multi-step reasoning pipeline
func (s *Server) handleAgentExecute(c *gin.Context) {
	var req agent.AgentRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if s.aiService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service not initialized (missing API Key)"})
		return
	}

	if req.ProjectID == "" || req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id and query are required"})
		return
	}

	// Validate ProjectID
	if err := ValidateProjectID(req.ProjectID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate and sanitize Query
	if err := ValidateQuery(req.Query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Query = SanitizeString(req.Query)

	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found: " + req.ProjectID})
		return
	}

	// Wrap the AIService in an adapter that satisfies agent.ModelInterface
	modelAdapter := ai.NewAIServiceModelAdapter(s.aiService)
	orch := agent.NewOrchestrator(modelAdapter, store)

	predicateNames := []string{
		config.PredicateDefines,
		config.PredicateCalls,
		config.PredicateImports,
		config.PredicateHasDoc,
		config.PredicateInPackage,
		config.PredicateHasRole,
		config.PredicateHasTag,
		config.PredicateKind,
	}

	ctx := c.Request.Context()
	session, err := orch.Run(ctx, req.ProjectID, req.Query, predicateNames)
	if err != nil {
		log.Printf("[Agent] Execute failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, agent.AgentResponse{
		SessionID: session.ID,
		Steps:     session.Steps,
		Narrative: session.Narrative,
	})
}

// Health check
func (s *Server) healthCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}

// CORSMiddleware handles CORS headers with a secure policy.
func CORSMiddleware() gin.HandlerFunc {
	config := DefaultCORSConfig()

	// Override with environment variables if provided
	if envOrigins := os.Getenv("CORS_ALLOW_ORIGINS"); envOrigins != "" {
		config.AllowOrigins = strings.Split(envOrigins, ",")
		for i := range config.AllowOrigins {
			config.AllowOrigins[i] = strings.TrimSpace(config.AllowOrigins[i])
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range config.AllowOrigins {
			if allowedOrigin == "*" {
				// Wildcard is only allowed in development
				if os.Getenv("GIN_MODE") != "release" {
					allowed = true
					break
				}
			} else if strings.EqualFold(allowedOrigin, origin) {
				allowed = true
				break
			}
		}

		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}

		// Set other CORS headers
		c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
		c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
		c.Writer.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))

		if config.AllowCredentials {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if config.MaxAge > 0 {
			c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
		}

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
