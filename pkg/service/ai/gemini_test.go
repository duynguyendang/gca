package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	"github.com/google/mangle/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock Store Manager
type MockManager struct {
	mock.Mock
}

func (m *MockManager) GetStore(projectID string) (*meb.MEBStore, error) {
	args := m.Called(projectID)
	return args.Get(0).(*meb.MEBStore), args.Error(1)
}

func TestHandleRequestPerformance(t *testing.T) {
	// Setup Temp Store
	dir := t.TempDir()
	cfg := store.DefaultConfig(dir)
	cfg.SyncWrites = false
	// Configure cache to ensure hits in memory
	cfg.BlockCacheSize = 10 << 20
	cfg.IndexCacheSize = 10 << 20

	s, err := meb.NewMEBStore(cfg)
	assert.NoError(t, err)
	defer s.Close()

	// Seed data (same as before)
	ctx := context.Background()

	// Add File Content
	docKey := string("main.go")
	content := []byte(`package main
import "pkg/foo"
func main() {
	foo.Bar()
}`)
	err = s.AddDocument(docKey, content, nil, nil)
	assert.NoError(t, err)

	// Add Symbol Content
	symKey := string("main.go:main")
	symContent := []byte("func main() {\n\tfoo.Bar()\n}")
	err = s.AddDocument(symKey, symContent, nil, nil)
	assert.NoError(t, err)

	// Add Facts (Triples)
	atom := ast.Atom{
		Predicate: ast.PredicateSym{Symbol: "triples", Arity: 3},
		Args: []ast.BaseTerm{
			ast.Constant{Type: ast.StringType, Symbol: "main.go:main"},
			ast.Constant{Type: ast.StringType, Symbol: "calls"},
			ast.Constant{Type: ast.StringType, Symbol: "pkg/foo:Bar"},
		},
	}
	s.Add(atom)

	// Add Defines fact
	atomDefines := ast.Atom{
		Predicate: ast.PredicateSym{Symbol: "triples", Arity: 3},
		Args: []ast.BaseTerm{
			ast.Constant{Type: ast.StringType, Symbol: "main.go"},
			ast.Constant{Type: ast.StringType, Symbol: "defines"},
			ast.Constant{Type: ast.StringType, Symbol: "main.go:main"},
		},
	}
	s.Add(atomDefines)

	// Add "pkg/foo:Bar" definition
	barKey := string("pkg/foo:Bar")
	barContent := []byte("func Bar() {}")
	s.AddDocument(barKey, barContent, nil, nil)

	// Initializing Service
	mgr := &MockManager{}
	mgr.On("GetStore", "test-project").Return(s, nil)

	svc := &GeminiService{
		manager: mgr,
	}

	// Warmup
	_, _ = svc.buildTaskPrompt(ctx, s, AIRequest{Task: "chat", Query: "warmup"})

	// Test Case 1: Chat Task with Semantic Context
	req := AIRequest{
		ProjectID: "test-project",
		Task:      "", // Trigger default case to call BuildPrompt
		Query:     "Explain what main.go does",
	}

	start := time.Now()
	// We test buildTaskPrompt directly to avoid calling actual Google AI
	prompt, err := svc.buildTaskPrompt(ctx, s, req)
	duration := time.Since(start)

	assert.NoError(t, err)

	// Verify Prompt Content
	t.Logf("Prompt: %s", prompt)
	assert.Contains(t, prompt, "User Question")

	// We should find at least one of these relevant contexts
	hasDefines := strings.Contains(prompt, "Defines")
	hasCalls := strings.Contains(prompt, "Calls")

	assert.True(t, hasDefines || hasCalls, "Prompt should contain either Defines or Calls")

	if hasCalls {
		assert.Contains(t, prompt, "pkg/foo:Bar", "Should mention called function")
	}

	// Verify Performance
	assert.Less(t, duration, 50*time.Millisecond, "buildTaskPrompt took too long")
}

func TestHandleRequestPrune(t *testing.T) {
	t.Skip("Skipping prune test without proper prompt file setup in test environment")
}

func TestHandleRequestWithExplicitSymbol(t *testing.T) {
	// Setup Temp Store
	dir := t.TempDir()
	cfg := store.DefaultConfig(dir)
	cfg.SyncWrites = false
	s, err := meb.NewMEBStore(cfg)
	assert.NoError(t, err)
	defer s.Close()

	// Add Symbol Content
	symKey := string("pkg/auth:Login")
	symContent := []byte("func Login() bool { return true }")
	s.AddDocument(symKey, symContent, nil, nil)

	mgr := &MockManager{}
	svc := &GeminiService{manager: mgr}

	req := AIRequest{
		Task:     "", // Trigger default case to call BuildPrompt
		Query:    "Analyze this",
		SymbolID: "pkg/auth:Login",
	}

	start := time.Now()
	prompt, err := svc.buildTaskPrompt(context.Background(), s, req)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Contains(t, prompt, "pkg/auth:Login")
	assert.Contains(t, prompt, "func Login()")

	t.Logf("Explicit Symbol Prompt built in %v", duration)
}
