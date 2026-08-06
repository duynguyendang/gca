package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/agent"
	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ephemeral"
	"github.com/duynguyendang/gca/pkg/logger"
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
	ReflectAnyOrigin bool // bypass allowlist: echo request Origin for any caller
}

// DefaultCORSConfig returns a secure default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://localhost:5173",
			"http://localhost:8080",
			"https://gca-hackathon.web.app",
			"https://gca-hackathon.firebaseapp.com",
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

// AIService defines the interface for AI-powered operations.
type AIService interface {
	HandleRequestOODA(ctx context.Context, req ai.AIRequest) (string, error)
	HandleRequest(ctx context.Context, req ai.AIRequest) (string, error)
	HandleRequestStream(ctx context.Context, req ai.AIRequest, onChunk func(string) error) error
	HandleAsk(ctx context.Context, req AskRequest) (*AskResponse, error)
	GetEmbedding(ctx context.Context, text string) ([]float32, error)
}

// AskRequest mirrors ai.AskRequest for the interface.
type AskRequest = ai.AskRequest

// AskResponse mirrors ai.AskResponse for the interface.
type AskResponse = ai.AskResponse

// Server holds the state for the REST API server.
type Server struct {
	manager        *manager.StoreManager
	graphService   *service.GraphService
	aiService      AIService
	mangleClient   *manglesdk.Client
	queryService   *registry.QueryService
	smellRegistry  *registry.SmellRegistry
	ephemeralStore *ephemeral.EphemeralStore
	rateLimiter    *RateLimiter
	sourceDir      string
	router         *gin.Engine
}

// NewServer creates a new Server instance.
func NewServer(mgr *manager.StoreManager, sourceDir string) *Server {
	rateLimiter := newRateLimiterFromEnv()
	r := gin.Default()
	r.Use(RequestIDMiddleware())
	r.Use(CORSMiddleware())
	r.Use(func(c *gin.Context) {
		key := c.ClientIP()
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			key = "api:" + apiKey
		}
		if !rateLimiter.Allow(key) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Please try again later.",
				"retry_after": 1,
			})
			c.Abort()
			return
		}
		c.Next()
	})
	r.Use(ValidationMiddleware())
	r.Use(CompressionMiddleware())

	svc := service.NewGraphService(mgr)

	aiSvc, err := ai.NewAIService(context.Background(), mgr)
	if err != nil {
		logger.Warn("Failed to initialize AI Service", "error", err)
		aiSvc = nil
	} else {
		logger.Info("AI Service initialized successfully")
	}

	// Initialize Manglekit Client for GenePool queries
	mangleClient, err := manglesdk.NewClient(context.Background())
	if err != nil {
		logger.Warn("Failed to initialize Manglekit Client", "error", err)
		mangleClient = nil
	} else {
		logger.Info("Manglekit Client initialized successfully")
	}

	// Initialize Query Service
	var queryService *registry.QueryService
	if mangleClient != nil {
		queryRegistry := registry.NewQueryRegistry(mangleClient.Engine())
		policyPath := config.GenePoolPath
		if err := queryRegistry.LoadQueriesFromGenePool(context.Background(), policyPath); err != nil {
			logger.Warn("Failed to load query registry", "error", err)
		} else {
			logger.Info("Query registry initialized successfully")
		}
		queryService = registry.NewQueryService(queryRegistry)
	}

	smellRegistry := registry.NewSmellRegistry(mgr)
	if err := smellRegistry.LoadFromPolicies(context.Background(), ""); err != nil {
		logger.Warn("Failed to load smell registry from policies", "error", err)
	}

	s := &Server{
		manager:        mgr,
		graphService:   svc,
		aiService:      aiSvc,
		mangleClient:   mangleClient,
		queryService:   queryService,
		smellRegistry:  smellRegistry,
		ephemeralStore: ephemeral.NewEphemeralStore(0),
		rateLimiter:    rateLimiter,
		sourceDir:      sourceDir,
		router:         r,
	}
	s.setupRoutes()
	return s
}

// NewServerWithAIService creates a Server with a custom AIService (used for testing).
func NewServerWithAIService(mgr *manager.StoreManager, sourceDir string, aiSvc AIService) *Server {
	rateLimiter := newRateLimiterFromEnv()
	r := gin.Default()
	r.Use(RequestIDMiddleware())
	r.Use(CORSMiddleware())
	r.Use(func(c *gin.Context) {
		key := c.ClientIP()
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			key = "api:" + apiKey
		}
		if !rateLimiter.Allow(key) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Please try again later.",
				"retry_after": 1,
			})
			c.Abort()
			return
		}
		c.Next()
	})
	r.Use(ValidationMiddleware())
	r.Use(CompressionMiddleware())

	svc := service.NewGraphService(mgr)

	smellRegistry := registry.NewSmellRegistry(mgr)

	s := &Server{
		manager:        mgr,
		graphService:   svc,
		aiService:      aiSvc,
		mangleClient:   nil,
		queryService:   nil,
		smellRegistry:  smellRegistry,
		ephemeralStore: ephemeral.NewEphemeralStore(0),
		rateLimiter:    rateLimiter,
		sourceDir:      sourceDir,
		router:         r,
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

// Close shuts down background resources (ephemeral store sweeper, rate limiter).
func (s *Server) Close() {
	s.ephemeralStore.Close()
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}
}

func (s *Server) setupRoutes() {
	s.router.GET("/api/health", s.healthCheck)
	s.router.GET("/api/ready", s.readyCheck)
	s.router.GET("/api/metrics", s.metricsHandler)
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

	// Cross-Reference Analysis
	s.router.GET("/api/v1/graph/who-calls", s.handleWhoCalls)
	s.router.GET("/api/v1/graph/what-calls", s.handleWhatCalls)
	s.router.GET("/api/v1/graph/reachable", s.handleCheckReachability)
	s.router.GET("/api/v1/graph/cycles", s.handleDetectCycles)
	s.router.GET("/api/v1/graph/lca", s.handleFindLCA)
	s.router.POST("/api/v1/graph/enrich-called-by", s.handleEnrichCalledBy)

	// AI Endpoints
	s.router.POST("/api/v1/ai/ask", s.handleAIAsk)
	s.router.POST("/api/v1/ai/classify", s.handleAIClassify)

	// Unified Ask Endpoint (NL -> Datalog -> Answer)
	s.router.POST("/api/v1/ask", s.handleAsk)

	// Agent Endpoint (multi-step reasoning)
	s.router.POST("/api/v1/agent/execute", s.handleAgentExecute)

	// Query Registry (GenePool pre-defined queries)
	if s.queryService != nil {
		s.queryService.AddRoute(s.router)
		logger.Info("Query service routes registered")
	}

	// Health Summary endpoints
	s.router.GET("/api/v1/health/summary", s.handleHealthSummary)
	s.router.GET("/api/v1/health/summary/v2", s.handleHealthSummaryV2)

	// Store health (meb v0.6 DebugInfo)
	s.router.GET("/api/v1/store/health", s.handleStoreHealth)

	// Analysis Endpoints
	s.router.GET("/api/v1/analysis/surprise", s.handleSurpriseAnalysis)
	s.router.GET("/api/v1/analysis/knowledge-gaps", s.handleKnowledgeGaps)
	s.router.POST("/api/v1/graph/diff", s.handleGraphDiff)

	// Snapshot Endpoints
	s.router.POST("/api/v1/graph/snapshots", s.handleCreateSnapshot)
	s.router.GET("/api/v1/graph/snapshots", s.handleListSnapshots)

	// Review Session Endpoints (Ephemeral Federation)
	s.router.POST("/api/v1/review/session", s.handleCreateReviewSession)
	s.router.POST("/api/v1/review/session/:id/query", s.handleReviewSessionQuery)

	// Ingest Endpoints
	s.router.POST("/api/v1/ingest/incremental", s.handleIncrementalIngest)

	// Test Generation Endpoints
	s.router.POST("/api/v1/projects/:projectId/test/generate", s.handleTestGenerate)
	s.router.POST("/api/v1/projects/:projectId/test/generate-all", s.handleTestGenerateAll)

	// OKF Endpoints (query only — ingest is handled by gca ingest pipeline)
	s.router.GET("/api/v1/okf/orphans", s.handleOKFOrphans)
	s.router.GET("/api/v1/okf/concepts", s.handleOKFConceptsBatch)
	s.router.GET("/api/v1/okf/links", s.handleOKFLinksBatch)

	// Route Discovery
	s.router.GET("/api/v1/routes", s.handleRoutes)
}

// AI Handler
func (s *Server) handleAIAsk(c *gin.Context) {
	var req ai.AIRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "invalid request body", err))
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

	if err := ValidateProjectID(req.ProjectID); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "invalid project ID", err))
		return
	}

	if req.Query != "" {
		if err := ValidateQuery(req.Query); err != nil {
			handleError(c, errors.NewAppError(http.StatusBadRequest, "invalid query", err))
			return
		}
		req.Query = SanitizeString(req.Query)
	}

	useOODA := os.Getenv("USE_OODA_LOOP") == "true"

	if useOODA {
		// OODA loop streaming not yet implemented — send full answer as one event
		answer, err := s.aiService.HandleRequestOODA(c.Request.Context(), req)
		if err != nil {
			handleError(c, errors.NewAppError(http.StatusInternalServerError, "AI request failed", err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"answer": answer})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	streamFn := func(chunk string) error {
		c.SSEvent("", chunk)
		c.Writer.Flush()
		return nil
	}
	if err := s.aiService.HandleRequestStream(c.Request.Context(), req, streamFn); err != nil {
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
		return
	}
	c.SSEvent("", "[DONE]")
	c.Writer.Flush()
}

// handleAIClassify returns just the intent classification without executing a full query.
// GET or POST /api/v1/ai/classify
func (s *Server) handleAIClassify(c *gin.Context) {
	var req struct {
		ProjectID           string `json:"project_id" form:"project_id"`
		Query               string `json:"query" form:"query"`
		ConversationHistory []ai.ConversationTurn `json:"conversation_history"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "invalid request body", err))
		return
	}

	if req.ProjectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ProjectID is required"})
		return
	}
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query is required"})
		return
	}

	intentResult := ai.ClassifyIntentWithContext(req.Query, req.ConversationHistory)

	c.JSON(http.StatusOK, gin.H{
		"intent":     string(intentResult.Intent),
		"confidence": intentResult.Confidence,
	})
}

// Agent Execute Handler - multi-step reasoning pipeline
func (s *Server) handleAgentExecute(c *gin.Context) {
	var req agent.AgentRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "invalid request body", err))
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
		handleError(c, errors.NewAppError(http.StatusBadRequest, "invalid project ID", err))
		return
	}

	// Validate and sanitize Query
	if err := ValidateQuery(req.Query); err != nil {
		handleError(c, errors.NewAppError(http.StatusBadRequest, "invalid query", err))
		return
	}
	req.Query = SanitizeString(req.Query)

	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		handleError(c, errors.NewAppError(http.StatusNotFound, "project not found", err))
		return
	}

	// Wrap the AIService in an adapter that satisfies agent.ModelInterface
	aiSvc, ok := s.aiService.(*ai.AIService)
	if !ok {
		handleError(c, errors.NewAppError(http.StatusInternalServerError, "AI service not available", nil))
		return
	}
	modelAdapter := ai.NewAIServiceModelAdapter(aiSvc)
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
		logger.Error("Agent Execute failed", "error", err)
		handleError(c, errors.NewAppError(http.StatusInternalServerError, "agent execution failed", err))
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
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// Readiness check - verifies store connectivity
func (s *Server) readyCheck(c *gin.Context) {
	projects, err := s.manager.ListProjects()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"error":  "failed to list projects",
		})
		return
	}

	// Check each project's store
	projectStatuses := make(map[string]string)
	for _, p := range projects {
		if _, err := s.manager.GetStore(p.ID); err != nil {
			projectStatuses[p.ID] = fmt.Sprintf("store_error: %v", err)
		} else {
			projectStatuses[p.ID] = "ok"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ready",
		"projects": projectStatuses,
	})
}

// Simple metrics endpoint - tracks request counts and latency
func (s *Server) metricsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"endpoint": "/api/metrics",
		"status":   "operational",
	})
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

	if os.Getenv("CORS_REFLECT_ANY") == "true" {
		config.ReflectAnyOrigin = true
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// CORS_REFLECT_ANY bypass: echo the request's Origin for any caller.
		if config.ReflectAnyOrigin && origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
			c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
			c.Writer.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))
			if config.AllowCredentials {
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if config.MaxAge > 0 {
				c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
			}
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
			c.Next()
			return
		}

		// Check if origin is allowed
		allowed := false
		usesWildcard := false
		for _, allowedOrigin := range config.AllowOrigins {
			if allowedOrigin == "*" {
				// Wildcard is only allowed in development AND requires AllowCredentials=false
				if os.Getenv("GIN_MODE") != "release" && !config.AllowCredentials {
					allowed = true
					usesWildcard = true
					break
				}
			} else if strings.EqualFold(allowedOrigin, origin) {
				allowed = true
				break
			}
		}

		if allowed {
			if usesWildcard {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			}
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
