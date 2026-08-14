package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ooda"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/gca/pkg/service/ai"
	"github.com/duynguyendang/meb"
)

const testProjectID = "testproj"

// --- Shared mock types ---

type testMockLLMClient struct {
	mu        sync.Mutex
	callCount int
	response  string
	err       error
}

func (m *testMockLLMClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *testMockLLMClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

type testMockPromptLoader struct {
	promptsDir string
}

func (l *testMockPromptLoader) LoadPrompt(name string) (*prompts.Prompt, error) {
	name = strings.TrimPrefix(name, "prompts/")
	return prompts.LoadPrompt(filepath.Join(l.promptsDir, name))
}

type testMockAIService struct {
	store        *meb.MEBStore
	promptsDir   string
	llm          common.LLMClient
	storeManager *testMockStoreManager

	// Configurable responses
	oodaResponse string
	oodaErr      error
	askResponse  *ai.AskResponse
	askErr       error
	streamErr    error
	embedding    []float32
}

func (m *testMockAIService) HandleRequestOODA(ctx context.Context, req ai.AIRequest) (string, error) {
	if m.oodaErr != nil {
		return "", m.oodaErr
	}
	if m.oodaResponse != "" {
		return m.oodaResponse, nil
	}
	// Default: use real OODA loop with mock LLM
	storeManager := m.storeManager
	if storeManager == nil {
		storeManager = &testMockStoreManager{store: m.store}
	}
	promptLoader := &testMockPromptLoader{promptsDir: m.promptsDir}
	config := ooda.NewOODAConfig(storeManager, promptLoader, m.llm)
	loop := ooda.NewOODALoopFromConfig(config)
	return ooda.RunOODATask(ctx, loop, req.ProjectID, req.Query, ooda.GCATask(req.Task), req.SymbolID, req.Data)
}

func (m *testMockAIService) HandleRequest(ctx context.Context, req ai.AIRequest) (string, error) {
	return m.HandleRequestOODA(ctx, req)
}

func (m *testMockAIService) HandleRequestStream(ctx context.Context, req ai.AIRequest, onChunk func(string) error) error {
	if m.streamErr != nil {
		return m.streamErr
	}
	resp, err := m.HandleRequestOODA(ctx, req)
	if err != nil {
		return err
	}
	return onChunk(resp)
}

func (m *testMockAIService) HandleAsk(ctx context.Context, req ai.AskRequest) (*ai.AskResponse, error) {
	if m.askErr != nil {
		return nil, m.askErr
	}
	if m.askResponse != nil {
		return m.askResponse, nil
	}
	return &ai.AskResponse{
		Answer:   "mock answer for: " + req.Query,
		Query:    "triples(?S, ?P, ?O)",
		Intent:   "task_insight",
		Confidence: 0.9,
	}, nil
}

func (m *testMockAIService) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	if m.embedding != nil {
		return m.embedding, nil
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

type testMockStoreManager struct {
	store *meb.MEBStore
}

func (m *testMockStoreManager) GetStore(projectID string) (*meb.MEBStore, error) {
	return m.store, nil
}

func (m *testMockStoreManager) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return m.store, nil
}

// --- Test data population ---

// populateBasicTestData adds a minimal set of facts for testing handlers.
// Covers: files, symbols, calls, roles, tags, packages, routes.
func populateBasicTestData(s *meb.MEBStore) {
	facts := []meb.Fact{
		// Files
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateDefines, Object: "handleUser"},
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateInPackage, Object: "handlers"},
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateHasLanguage, Object: "go"},

		// Symbols
		{Subject: "handleUser", Predicate: config.PredicateHasKind, Object: "function"},
		{Subject: "handleUser", Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler},
		{Subject: "handleUser", Predicate: config.PredicateStartLine, Object: "10"},
		{Subject: "handleUser", Predicate: config.PredicateEndLine, Object: "50"},

		// Routes
		{Subject: "/api/users", Predicate: config.PredicateHandledBy, Object: "handleUser"},
		{Subject: "/api/users", Predicate: "http_method", Object: "GET"},

		// Dependencies
		{Subject: "auth.go", Predicate: config.PredicateDefines, Object: "AuthService"},
		{Subject: "auth.go", Predicate: config.PredicateHasTag, Object: "auth"},
		{Subject: "AuthService", Predicate: config.PredicateHasRole, Object: "auth_service"},
		{Subject: "handleUser", Predicate: config.PredicateCalls, Object: "AuthService"},

		{Subject: "db/user_repo.go", Predicate: config.PredicateDefines, Object: "UserRepo"},
		{Subject: "db/user_repo.go", Predicate: config.PredicateHasTag, Object: "database"},
		{Subject: "UserRepo", Predicate: config.PredicateHasRole, Object: config.RoleDataContract},
		{Subject: "handleUser", Predicate: config.PredicateCalls, Object: "UserRepo"},

		// Second handler
		{Subject: "handlers/order_handler.go", Predicate: config.PredicateDefines, Object: "handleOrder"},
		{Subject: "handlers/order_handler.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "handleOrder", Predicate: config.PredicateHasKind, Object: "function"},
		{Subject: "handleOrder", Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler},
		{Subject: "/api/orders", Predicate: config.PredicateHandledBy, Object: "handleOrder"},
		{Subject: "/api/orders", Predicate: "http_method", Object: "POST"},

		// Utility
		{Subject: "utils/validator.go", Predicate: config.PredicateDefines, Object: "ValidateInput"},
		{Subject: "utils/validator.go", Predicate: config.PredicateHasTag, Object: "utility"},
		{Subject: "ValidateInput", Predicate: config.PredicateHasRole, Object: config.RoleUtility},
		{Subject: "handleUser", Predicate: config.PredicateCalls, Object: "ValidateInput"},

		// Test file
		{Subject: "handlers/user_handler_test.go", Predicate: config.PredicateDefines, Object: "TestHandleUser"},
	}
	s.AddFactBatch(facts)
}

// populateSmellTestData adds smell-related facts to the analytical store.
func populateSmellTestData(s *meb.MEBStore) {
	facts := []meb.Fact{
		// Circular dependency
		{Subject: "a.go", Predicate: "has_smell_type", Object: "circular_dependency"},
		{Subject: "a.go", Predicate: "has_smell_severity", Object: "high"},
		{Subject: "a.go", Predicate: "smell_detail", Object: "a.go -> b.go -> a.go"},

		// God file
		{Subject: "big.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "big.go", Predicate: "has_smell_severity", Object: "critical"},

		// Hub
		{Subject: "hub.go", Predicate: "has_smell_type", Object: "hub"},
		{Subject: "hub.go", Predicate: "has_smell_severity", Object: "medium"},

		// Health scores
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateHasHealthScore, Object: "85"},
		{Subject: "handlers/order_handler.go", Predicate: config.PredicateHasHealthScore, Object: "72"},
		{Subject: "big.go", Predicate: config.PredicateHasHealthScore, Object: "30"},

		// Health debt
		{Subject: "big.go", Predicate: config.PredicateHasHealthDebt, Object: "15"},
		{Subject: "a.go", Predicate: config.PredicateHasHealthDebt, Object: "8"},

		// Entry points
		{Subject: "main.go", Predicate: config.PredicateHasKind, Object: "function"},
		{Subject: "main.go", Predicate: "has_out_degree", Object: "5"},
		{Subject: "main.go", Predicate: "has_in_degree", Object: "0"},
	}
	s.AddFactBatch(facts)
}

// populateSurpriseTestData adds surprise-related facts.
func populateSurpriseTestData(s *meb.MEBStore) {
	facts := []meb.Fact{
		{Subject: "a.go", Predicate: "surprise_cross_community", Object: "b.go"},
		{Subject: "a.go", Predicate: "surprise_cross_language", Object: "c.py"},
		{Subject: "a.go", Predicate: "surprise_peripheral_hub", Object: "hub.go"},
		{Subject: "a.go", Predicate: "surprise_score", Object: "0.85"},
	}
	s.AddFactBatch(facts)
}

// populateKnowledgeGapTestData adds knowledge gap facts.
func populateKnowledgeGapTestData(s *meb.MEBStore) {
	facts := []meb.Fact{
		{Subject: "orphan.go", Predicate: "gap_isolated", Object: "true"},
		{Subject: "hotspot.go", Predicate: "gap_untested_hotspot", Object: "true"},
		{Subject: "thin_cluster", Predicate: "gap_thin_community", Object: "true"},
		{Subject: "single_file_cluster", Predicate: "gap_single_file_community", Object: "true"},
	}
	s.AddFactBatch(facts)
}

// --- Test server setup ---

// testServerConfig holds options for creating a test server.
type testServerConfig struct {
	// AIService override (if nil, a default mock is created unless
	// NoAIService is set).
	AIService AIService
	// NoAIService forces a nil AI service on the server so handlers exercise
	// their "AI service not available" path.
	NoAIService bool
	// LLMClient override
	LLMClient common.LLMClient
	// LLMResponse is the canned response for the mock LLM
	LLMResponse string
	// LLMError is the error for the mock LLM
	LLMError error
	// OODAResponse overrides HandleRequestOODA response
	OODAResponse string
	// OODAError overrides HandleRequestOODA error
	OODAError error
	// AskResponse overrides HandleAsk response
	AskResponse *ai.AskResponse
	// AskError overrides HandleAsk error
	AskError error
	// SkipTestData skips populating basic test data
	SkipTestData bool
	// SkipSmellData skips populating smell test data
	SkipSmellData bool
	// ExtraFacts are added after basic test data
	ExtraFacts []meb.Fact
}

// setupTestServer creates a test server with a real MEB store and mock AI.
// Returns the server, the underlying store, and a cleanup function.
func setupTestServer(t *testing.T, cfg ...testServerConfig) (*Server, *meb.MEBStore, func()) {
	t.Helper()

	promptsDir := "/mnt/e/gca-v2/gca/prompts"
	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		t.Skipf("prompts dir not found at %s", promptsDir)
	}

	// Create temp data directory
	dataDir, err := os.MkdirTemp("", "gca-test-*")
	if err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}

	// Create project directory with metadata
	projDir := filepath.Join(dataDir, testProjectID)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to create project dir: %v", err)
	}
	meta := `{"name": "Test Project", "description": "Unit test project"}`
	if err := os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(meta), 0644); err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to write metadata: %v", err)
	}

	// Apply config
	var c testServerConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}

	// Create store manager FIRST, then seed via ITS store handle. Handlers read
	// through the manager's cached *meb.MEBStore; a separately-created store
	// instance opens a second Badger handle on the same directory and cannot
	// see writes made through the first one, so queries would return empty.
	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)

	// Open the source store handle for the test project so seeding shares the
	// same underlying MEB store the handlers will read.
	s, err := mgr.GetSourceStore(testProjectID)
	if err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to open test project store: %v", err)
	}

	// Populate test data
	if !c.SkipTestData {
		populateBasicTestData(s)
	}
	if !c.SkipSmellData {
		populateSmellTestData(s)
		populateSurpriseTestData(s)
		populateKnowledgeGapTestData(s)
	}
	if len(c.ExtraFacts) > 0 {
		s.AddFactBatch(c.ExtraFacts)
	}

	// Create mock LLM
	llm := c.LLMClient
	if llm == nil {
		llm = &testMockLLMClient{
			response: c.LLMResponse,
			err:      c.LLMError,
		}
	}

	// Create mock AI service
	var aiSvc AIService
	if !c.NoAIService {
		if c.AIService != nil {
			aiSvc = c.AIService
		} else {
			mockStoreMgr := &testMockStoreManager{store: s}
			aiSvc = &testMockAIService{
				store:        s,
				promptsDir:   promptsDir,
				llm:          llm,
				storeManager: mockStoreMgr,
				oodaResponse: c.OODAResponse,
				oodaErr:      c.OODAError,
				askResponse:  c.AskResponse,
				askErr:       c.AskError,
			}
		}
	}

	// Create server
	srv := NewServerWithAIService(mgr, dataDir, aiSvc)

	cleanup := func() {
		srv.Close()
		mgr.CloseAll()
		os.RemoveAll(dataDir)
	}

	return srv, s, cleanup
}

// --- Request helpers ---

// doRequest creates and executes an HTTP request against the server's router.
func doRequest(s *Server, method, path string, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// doJSONRequest creates and executes a JSON HTTP request.
func doJSONRequest(s *Server, method, path string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// requireStatus checks that the response has the expected status code.
func requireStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if w.Code != expected {
		t.Errorf("expected status %d, got %d: %s", expected, w.Code, w.Body.String())
	}
}

// requireJSON checks that the response body is valid JSON and unmarshals it.
func requireJSON(t *testing.T, w *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
		t.Fatalf("failed to unmarshal response: %v\nBody: %s", err, w.Body.String())
	}
}


