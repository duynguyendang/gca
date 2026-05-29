//go:build integration
// +build integration

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
	"github.com/duynguyendang/meb/store"
)

const (
	testProjectID = "testproj"
	cannedTest    = `func TestIntegration_HandleUser_ServeHTTP(t *testing.T) {
	ts := httptest.NewServer(setupTestRouter())
	defer ts.Close()

	t.Run("GET /api/users/:id returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", ts.URL+"/api/users/123", nil)
		w := httptest.NewRecorder()
		ts.Config.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("GET /api/users/:id with invalid id returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", ts.URL+"/api/users/abc", nil)
		w := httptest.NewRecorder()
		ts.Config.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}`
)

type mockLLMClient struct {
	mu        sync.Mutex
	callCount int
	response  string
}

func (m *mockLLMClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return m.response, nil
}

func (m *mockLLMClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

type mockPromptLoader struct {
	promptsDir string
}

func (l *mockPromptLoader) LoadPrompt(name string) (*prompts.Prompt, error) {
		name = strings.TrimPrefix(name, "prompts/")
		return prompts.LoadPrompt(filepath.Join(l.promptsDir, name))
	}

type mockAIService struct {
	store       *meb.MEBStore
	promptsDir  string
	llm         common.LLMClient
	storeManager *testStoreManager
}

func (m *mockAIService) HandleRequestOODA(ctx context.Context, req ai.AIRequest) (string, error) {
	storeManager := m.storeManager
	if storeManager == nil {
		storeManager = &testStoreManager{store: m.store}
	}
	promptLoader := &mockPromptLoader{promptsDir: m.promptsDir}
	config := ooda.NewOODAConfig(storeManager, promptLoader, m.llm)
	loop := ooda.NewOODALoopFromConfig(config)
	return ooda.RunOODATask(ctx, loop, req.ProjectID, req.Query, ooda.GCATask(req.Task), req.SymbolID, req.Data)
}

func (m *mockAIService) HandleRequest(ctx context.Context, req ai.AIRequest) (string, error) {
	return "", nil
}

func (m *mockAIService) HandleAsk(ctx context.Context, req ai.AskRequest) (*ai.AskResponse, error) {
	return &ai.AskResponse{}, nil
}

func (m *mockAIService) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

// testStoreManager implements the ooda.StoreManager interface.
type testStoreManager struct {
	store *meb.MEBStore
}

func (m *testStoreManager) GetStore(projectID string) (*meb.MEBStore, error) {
	return m.store, nil
}

func (m *testStoreManager) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return nil, nil
}

func createTestStore(t *testing.T) (*meb.MEBStore, string, func()) {
	tmpDir, err := os.MkdirTemp("", "gca-testgen-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		s.Close()
		os.RemoveAll(tmpDir)
	}

	return s, tmpDir, cleanup
}

func populateTestData(s *meb.MEBStore) {
	facts := []meb.Fact{
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateDefines, Object: "handleUser"},
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "handleUser", Predicate: config.PredicateHasKind, Object: "function"},
		{Subject: "handlers/user_handler.go", Predicate: config.PredicateInPackage, Object: "handlers"},
		{Subject: "handleUser", Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler},
		{Subject: "/api/users", Predicate: config.PredicateHandledBy, Object: "handleUser"},
		{Subject: "/api/users", Predicate: "http_method", Object: "GET"},

		{Subject: "auth.go", Predicate: config.PredicateDefines, Object: "AuthService"},
		{Subject: "auth.go", Predicate: config.PredicateHasTag, Object: "auth"},
		{Subject: "AuthService", Predicate: config.PredicateHasRole, Object: "auth_service"},
		{Subject: "handleUser", Predicate: config.PredicateCalls, Object: "AuthService"},

		{Subject: "db/user_repo.go", Predicate: config.PredicateDefines, Object: "UserRepo"},
		{Subject: "user_repo.go", Predicate: config.PredicateHasTag, Object: "database"},
		{Subject: "UserRepo", Predicate: config.PredicateHasRole, Object: config.RoleDataContract},
		{Subject: "handleUser", Predicate: config.PredicateCalls, Object: "UserRepo"},

		{Subject: "handlers/order_handler.go", Predicate: config.PredicateDefines, Object: "handleOrder"},
		{Subject: "handlers/order_handler.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "handleOrder", Predicate: config.PredicateHasKind, Object: "function"},
		{Subject: "handleOrder", Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler},
		{Subject: "/api/orders", Predicate: config.PredicateHandledBy, Object: "handleOrder"},
		{Subject: "/api/orders", Predicate: "http_method", Object: "POST"},
		{Subject: "/api/orders/:id", Predicate: config.PredicateHandledBy, Object: "handleOrder"},

		{Subject: "handlers/admin_handler.go", Predicate: config.PredicateDefines, Object: "handleAdmin"},
		{Subject: "admin_handler.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "handleAdmin", Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler},
		{Subject: "/api/admin", Predicate: config.PredicateHandledBy, Object: "handleAdmin"},

		{Subject: "middleware/auth_middleware.go", Predicate: config.PredicateDefines, Object: "AuthMiddleware"},
		{Subject: "handleAdmin", Predicate: config.PredicateCalls, Object: "AuthMiddleware"},

		{Subject: "utils/validator.go", Predicate: config.PredicateDefines, Object: "ValidateInput"},
		{Subject: "handleUser", Predicate: config.PredicateCalls, Object: "ValidateInput"},

		{Subject: "handlers/user_handler_test.go", Predicate: config.PredicateDefines, Object: "TestHandleUser"},
	}
	s.AddFactBatch(facts)
}

func setupServerWithMockAI(t *testing.T) (*Server, *mockLLMClient, func()) {
	promptsDir := "/mnt/e/gca-v2/gca/prompts"
	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		t.Skipf("prompts dir not found at %s", promptsDir)
	}

	dataDir, err := os.MkdirTemp("", "gca-data-*")
	if err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}

	projDir := filepath.Join(dataDir, testProjectID)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to create project dir: %v", err)
	}

	meta := `{"name": "Test Project", "description": "Integration test project"}`
	if err := os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(meta), 0644); err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to write metadata: %v", err)
	}

	s, _, storeCleanup := createTestStore(t)
	populateTestData(s)

	llm := &mockLLMClient{response: cannedTest}
	mockStoreMgr := &testStoreManager{store: s}
	mockAI := &mockAIService{store: s, promptsDir: promptsDir, llm: llm, storeManager: mockStoreMgr}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	srv := NewServerWithAIService(mgr, dataDir, mockAI)

	cleanup := func() {
		storeCleanup()
		os.RemoveAll(dataDir)
	}
	return srv, llm, cleanup
}

func TestHandleTestGenerate(t *testing.T) {
	srv, llm, cleanup := setupServerWithMockAI(t)
	defer cleanup()

	body := strings.NewReader(`{"target": "handleUser", "query": "generate tests", "depth": 3}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/testproj/test/generate", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	answer, ok := resp["answer"].(string)
	if !ok {
		t.Fatal("expected 'answer' field in response")
	}
	if answer != cannedTest {
		t.Errorf("expected canned response, got: %s", answer)
	}
	if llm.CallCount() == 0 {
		t.Error("expected at least one LLM call")
	}
}

func TestHandleTestGenerate_MissingProject(t *testing.T) {
	srv, _, cleanup := setupServerWithMockAI(t)
	defer cleanup()

	body := strings.NewReader(`{"target": "handleUser"}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/nonexistent/test/generate", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (handler delegates to AI service which uses mock store), got %d", w.Code)
	}
}

func TestHandleTestGenerate_InvalidBody(t *testing.T) {
	srv, _, cleanup := setupServerWithMockAI(t)
	defer cleanup()

	body := strings.NewReader(`{invalid}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/testproj/test/generate", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTestGenerateAll(t *testing.T) {
	srv, llm, cleanup := setupServerWithMockAI(t)
	defer cleanup()

	body := strings.NewReader(`{"depth": 3}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/testproj/test/generate-all", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	generated, ok := resp["generated"].(float64)
	if !ok {
		t.Fatal("expected 'generated' field")
	}
	if generated < 1 {
		t.Errorf("expected at least 1 generated test, got %v", generated)
	}

	total, ok := resp["total"].(float64)
	if !ok {
		t.Fatal("expected 'total' field")
	}
	if total < 1 {
		t.Errorf("expected at least 1 total handler, got %v", total)
	}

	if llm.CallCount() < 1 {
		t.Errorf("expected at least 1 LLM call, got %d", llm.CallCount())
	}
}

func TestHandleTestGenerateAll_EmptyProject(t *testing.T) {
	promptsDir := "/mnt/e/gca-v2/gca/prompts"
	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		t.Skipf("prompts dir not found at %s", promptsDir)
	}

	dataDir, err := os.MkdirTemp("", "gca-data-empty-*")
	if err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}
	defer os.RemoveAll(dataDir)

	projDir := filepath.Join(dataDir, "emptyproj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}
	meta := `{"name": "Empty Project", "description": "No handlers"}`
	if err := os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(meta), 0644); err != nil {
		t.Fatalf("Failed to write metadata: %v", err)
	}

	s, _, cleanup := createTestStore(t)
	defer cleanup()

	llm := &mockLLMClient{response: cannedTest}
	mockStoreMgr := &testStoreManager{store: s}
	mockAI := &mockAIService{store: s, promptsDir: promptsDir, llm: llm, storeManager: mockStoreMgr}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	srv := NewServerWithAIService(mgr, dataDir, mockAI)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/emptyproj/test/generate-all", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TestGenerateAllResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 handler from mock store, got %d", resp.Total)
	}
	if resp.Generated != 1 {
		t.Errorf("expected 1 generated for mock store, got %d", resp.Generated)
	}
}

func TestHandleTestGenerateAll_Concurrent(t *testing.T) {
	srv, llm, cleanup := setupServerWithMockAI(t)
	defer cleanup()

	body := strings.NewReader(`{"depth": 2}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/testproj/test/generate-all", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TestGenerateAllResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if llm.CallCount() != resp.Total {
		t.Errorf("LLM call count (%d) should equal total handlers (%d)", llm.CallCount(), resp.Total)
	}
}