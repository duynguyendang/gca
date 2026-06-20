//go:build integration
// +build integration

package ingest

import (
	"context"
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func createTestStore(t *testing.T) (*meb.MEBStore, string, func()) {
	tmpDir, err := os.MkdirTemp("", "gca-integration-test-*")
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
		{Subject: "main.go", Predicate: config.PredicateHasTag, Object: "frontend"},
		{Subject: "main.go", Predicate: config.PredicateDefines, Object: "App"},
		{Subject: "App", Predicate: config.PredicateDefines, Object: "render"},
		{Subject: "server.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "server.go", Predicate: config.PredicateDefines, Object: "Handler"},
		{Subject: "Handler", Predicate: config.PredicateDefines, Object: "ServeHTTP"},
		{Subject: "server.go", Predicate: config.PredicateImports, Object: "net/http"},
		{Subject: "main.go", Predicate: config.PredicateImports, Object: "react"},
		{Subject: "router.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "router.go", Predicate: config.PredicateDefines, Object: "Router"},
		{Subject: "Router", Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler},
		{Subject: "/test", Predicate: config.PredicateHandledBy, Object: "Handler"},
		{Subject: "main.go", Predicate: config.PredicateCalls, Object: "App.render"},
		{Subject: "Handler", Predicate: config.PredicateCalls, Object: "Router"},
		{Subject: "api.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "api.go", Predicate: config.PredicateDefines, Object: "handleAPI"},
		{Subject: "handleAPI", Predicate: config.PredicateHasRole, Object: config.RoleDataContract},
	}
	s.AddFactBatch(facts)
}

func TestEnhanceVirtualTriples_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	populateTestData(s)

	err := EnhanceVirtualTriples(s)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples failed: %v", err)
	}

	routeCount := 0
	for range s.Scan("", config.PredicateHandledBy, "") {
		routeCount++
	}
	t.Logf("Routes after EnhanceVirtualTriples: %d", routeCount)

	handlerCount := 0
	for range s.Scan("", config.PredicateHasRole, config.RoleAPIHandler) {
		handlerCount++
	}
	t.Logf("Handlers after EnhanceVirtualTriples: %d", handlerCount)
}

func TestSymbolResolver_BuildImportMap_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	populateTestData(s)

	sr := NewSymbolResolver(s)
	err := sr.BuildImportMap(s)
	if err != nil {
		t.Errorf("BuildImportMap failed: %v", err)
	}

	if len(sr.importMap) == 0 {
		t.Error("Expected import map to have entries")
	}
	t.Logf("Import map size: %d", len(sr.importMap))
}

func TestSymbolResolver_BuildCallGraph_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	populateTestData(s)

	sr := NewSymbolResolver(s)
	cg, err := sr.BuildCallGraph(s)
	if err != nil {
		t.Errorf("BuildCallGraph failed: %v", err)
	}

	if cg == nil {
		t.Fatal("CallGraph should not be nil")
	}

	t.Logf("CallGraph: %d callers, %d callees", len(cg.Calls), len(cg.CalledBy))

	if len(cg.Calls) == 0 {
		t.Error("Expected at least some calls in the graph")
	}
}

func TestCallGraph_GetCallees_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	populateTestData(s)

	sr := NewSymbolResolver(s)
	cg, err := sr.BuildCallGraph(s)
	if err != nil {
		t.Fatalf("BuildCallGraph failed: %v", err)
	}

	for caller, callees := range cg.Calls {
		result := cg.GetCallees(caller)
		if result == nil && len(callees) > 0 {
			t.Error("GetCallees returned nil for caller with known callees")
		}
		t.Logf("Caller %s has %d callees", caller, len(callees))
		break
	}
}

func TestCallGraph_DetectCycles_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFactBatch([]meb.Fact{
		{Subject: "a.go", Predicate: config.PredicateDefines, Object: "A"},
		{Subject: "b.go", Predicate: config.PredicateDefines, Object: "B"},
		{Subject: "A", Predicate: config.PredicateCalls, Object: "B"},
		{Subject: "B", Predicate: config.PredicateCalls, Object: "A"},
	})

	sr := NewSymbolResolver(s)
	cg, err := sr.BuildCallGraph(s)
	if err != nil {
		t.Fatalf("BuildCallGraph failed: %v", err)
	}

	cycles := cg.DetectCycles()
	t.Logf("Detected %d cycles in call graph", len(cycles))
}

func TestCallGraph_FindReachable_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFactBatch([]meb.Fact{
		{Subject: "a.go", Predicate: config.PredicateDefines, Object: "A"},
		{Subject: "b.go", Predicate: config.PredicateDefines, Object: "B"},
		{Subject: "c.go", Predicate: config.PredicateDefines, Object: "C"},
		{Subject: "A", Predicate: config.PredicateCalls, Object: "B"},
		{Subject: "B", Predicate: config.PredicateCalls, Object: "C"},
	})

	sr := NewSymbolResolver(s)
	cg, err := sr.BuildCallGraph(s)
	if err != nil {
		t.Fatalf("BuildCallGraph failed: %v", err)
	}

	reached := cg.FindReachable("A", "C", 5)
	t.Logf("From A reachable to C: %v", reached)
}

func TestCallGraph_GetCallers_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFactBatch([]meb.Fact{
		{Subject: "a.go", Predicate: config.PredicateDefines, Object: "A"},
		{Subject: "b.go", Predicate: config.PredicateDefines, Object: "B"},
		{Subject: "A", Predicate: config.PredicateCalls, Object: "B"},
	})

	sr := NewSymbolResolver(s)
	cg, err := sr.BuildCallGraph(s)
	if err != nil {
		t.Fatalf("BuildCallGraph failed: %v", err)
	}

	callers := cg.GetCallers("B")
	t.Logf("B has %d callers", len(callers))
}

func TestCallGraph_LeastCommonAncestor_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFactBatch([]meb.Fact{
		{Subject: "a.go", Predicate: config.PredicateDefines, Object: "A"},
		{Subject: "b.go", Predicate: config.PredicateDefines, Object: "B"},
		{Subject: "c.go", Predicate: config.PredicateDefines, Object: "C"},
		{Subject: "A", Predicate: config.PredicateCalls, Object: "C"},
		{Subject: "B", Predicate: config.PredicateCalls, Object: "C"},
	})

	sr := NewSymbolResolver(s)
	cg, err := sr.BuildCallGraph(s)
	if err != nil {
		t.Fatalf("BuildCallGraph failed: %v", err)
	}

	lca := cg.LeastCommonAncestor("A", "B", 5)
	t.Logf("LCA of A and B: %s", lca)
}

func TestAnalyzer_setAnalyticsVersion_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	populateTestData(s)

	analyzer := NewAnalyzer(nil, nil)
	err := analyzer.setAnalyticsVersion(s)
	if err != nil {
		t.Errorf("setAnalyticsVersion failed: %v", err)
	}

	count := 0
	for range s.Scan("", "analytics_version", "") {
		count++
	}
	if count == 0 {
		t.Error("Expected analytics_version fact to be set")
	}
}

func TestAnalyzer_getAnalyticsVersion_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFact(meb.Fact{Subject: "analytics", Predicate: "analytics_version", Object: "2.0"})

	count := 0
	for range s.Scan("", "analytics_version", "") {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 analytics_version fact, got %d", count)
	}
}

type mockTemplateStoreForAnalysis struct{}

func (m *mockTemplateStoreForAnalysis) ListTemplates(ctx context.Context, projectID, category string) ([]*TemplateStoreQuery, error) {
	return nil, nil
}

func (m *mockTemplateStoreForAnalysis) GetTemplate(ctx context.Context, projectID, templateID string) (*TemplateStoreQuery, error) {
	return nil, nil
}

func TestTagRoles_WithTestData(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFactBatch([]meb.Fact{
		{Subject: "router.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "router.go", Predicate: config.PredicateDefines, Object: "Router"},
		{Subject: "/api/users", Predicate: config.PredicateHandledBy, Object: "Router"},
		{Subject: "main.go", Predicate: config.PredicateHasTag, Object: "frontend"},
		{Subject: "main.go", Predicate: config.PredicateDefines, Object: "App"},
		{Subject: "models/user.go", Predicate: config.PredicateInPackage, Object: "models/types"},
		{Subject: "models/user.go", Predicate: config.PredicateDefines, Object: "User"},
	})

	err := TagRoles(s)
	if err != nil {
		t.Errorf("TagRoles failed: %v", err)
	}

	handlerCount := 0
	for range s.Scan("", config.PredicateHasRole, config.RoleAPIHandler) {
		handlerCount++
	}
	t.Logf("Handlers after TagRoles: %d", handlerCount)

	dataContractCount := 0
	for range s.Scan("", config.PredicateHasRole, config.RoleDataContract) {
		dataContractCount++
	}
	t.Logf("Data contracts after TagRoles: %d", dataContractCount)
}

func TestTagRoles_EmptyStore(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	err := TagRoles(context.Background(), s)
	if err != nil {
		t.Errorf("TagRoles failed: %v", err)
	}
}

func TestEmitFactFromTemplate_Basic(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	analyzer := NewAnalyzer(nil, nil)

	result := map[string]any{
		"File": "test.go",
	}
	tmpl := &TemplateStoreQuery{
		ID:        "test_smell",
		Predicate: "has_smell",
		SmellType: "long_function",
	}

	err := analyzer.emitFactFromTemplate(s, result, tmpl)
	if err != nil {
		t.Errorf("emitFactFromTemplate failed: %v", err)
	}

	count := 0
	for range s.Scan("", "has_smell", "") {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 has_smell fact, got %d", count)
	}
}

func TestEmitFactFromTemplate_WithSubjectA(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	analyzer := NewAnalyzer(nil, nil)

	result := map[string]any{
		"A": "symbol_main",
	}
	tmpl := &TemplateStoreQuery{
		ID:        "entry_point",
		Predicate: "is_entry_point",
	}

	err := analyzer.emitFactFromTemplate(s, result, tmpl)
	if err != nil {
		t.Errorf("emitFactFromTemplate failed: %v", err)
	}

	count := 0
	for range s.Scan("", "is_entry_point", "") {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 is_entry_point fact, got %d", count)
	}
}

func TestEmitFactFromTemplate_NoSubject(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	analyzer := NewAnalyzer(nil, nil)

	result := map[string]any{}
	tmpl := &TemplateStoreQuery{
		ID:       "test",
		Predicate: "has_test",
	}

	err := analyzer.emitFactFromTemplate(s, result, tmpl)
	if err == nil {
		t.Error("Expected error for empty subject")
	}
}

func TestEmitFactFromTemplate_DefaultPredicate(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	analyzer := NewAnalyzer(nil, nil)

	result := map[string]any{
		"File": "handler.go",
	}
	tmpl := &TemplateStoreQuery{
		ID:       "api_handler",
		Predicate: "",
	}

	err := analyzer.emitFactFromTemplate(s, result, tmpl)
	if err != nil {
		t.Errorf("emitFactFromTemplate failed: %v", err)
	}

	count := 0
	for range s.Scan("", "has_api_handler", "") {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 has_api_handler fact (default predicate), got %d", count)
	}
}

func TestEmitFactFromTemplate_WithMetadata(t *testing.T) {
	s, _, cleanup := createTestStore(t)
	defer cleanup()

	analyzer := NewAnalyzer(nil, nil)

	result := map[string]any{
		"File": "utils.go",
	}
	tmpl := &TemplateStoreQuery{
		ID:          "god_file",
		Predicate:   "has_smell",
		Category:    "architecture",
		Severity:    "high",
		SmellType:   "god_file",
		Description: "File has too many responsibilities",
	}

	err := analyzer.emitFactFromTemplate(s, result, tmpl)
	if err != nil {
		t.Errorf("emitFactFromTemplate failed: %v", err)
	}

	count := 0
	for range s.Scan("utils.go", "has_smell", "") {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 has_smell fact for utils.go, got %d", count)
	}
}

func TestExecuteRulesFromTemplates_NilTemplateStore(t *testing.T) {
	_, _, cleanup := createTestStore(t)
	defer cleanup()

	analyzer := NewAnalyzer(nil, nil)

	err := analyzer.executeRulesFromTemplates(context.Background(), "test-project")
	if err == nil {
		t.Error("Expected error for nil template store")
	}
	if err.Error() != "template store not available" {
		t.Errorf("Expected 'template store not available' error, got: %v", err)
	}
}

func TestExecuteRulesFromTemplates_EmptyTemplates(t *testing.T) {
	s, tmpDir, cleanup := createTestStore(t)
	defer cleanup()

	_ = s

	mgr := &mockStoreManagerForAnalysis{
		sourceStore:     nil,
		analyticalStore: nil,
		projectID:       "test",
		tmpDir:          tmpDir,
	}

	analyzerWithMgr := NewAnalyzer(mgr, &mockTemplateStoreForAnalysis{})

	err := analyzerWithMgr.executeRulesFromTemplates(context.Background(), "test-project")
	if err != nil {
		t.Errorf("executeRulesFromTemplates should handle empty templates: %v", err)
	}
}

type mockStoreManagerForAnalysis struct {
	sourceStore     *meb.MEBStore
	analyticalStore *meb.MEBStore
	projectID       string
	tmpDir          string
}

func (m *mockStoreManagerForAnalysis) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	if m.sourceStore != nil {
		return m.sourceStore, nil
	}
	cfg := store.DefaultConfig(m.tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		return nil, err
	}
	m.sourceStore = s
	return s, nil
}

func (m *mockStoreManagerForAnalysis) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	if m.analyticalStore != nil {
		return m.analyticalStore, nil
	}
	cfg := store.DefaultConfig(m.tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		return nil, err
	}
	m.analyticalStore = s
	return s, nil
}

type mockTemplateStoreWithTemplates struct {
	templates []*TemplateStoreQuery
}

func (m *mockTemplateStoreWithTemplates) ListTemplates(ctx context.Context, projectID, category string) ([]*TemplateStoreQuery, error) {
	return m.templates, nil
}

func (m *mockTemplateStoreWithTemplates) GetTemplate(ctx context.Context, projectID, templateID string) (*TemplateStoreQuery, error) {
	for _, t := range m.templates {
		if t.ID == templateID {
			return t, nil
		}
	}
	return nil, nil
}

func TestExecuteRulesFromTemplates_WithMockTemplates(t *testing.T) {
	t.Skip("Test requires separate temp dirs for source and analytical stores due to Badger locking")

	s, tmpDir, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFactBatch([]meb.Fact{
		{Subject: "main.go", Predicate: config.PredicateDefines, Object: "main"},
		{Subject: "main.go", Predicate: config.PredicateCalls, Object: "fmt.Println"},
	})

	templates := []*TemplateStoreQuery{
		{
			ID:       "entry_point",
			Body:     "triples(?S, 'defines', 'main')",
			Predicate: "is_entry_point",
		},
	}

	mgr := &mockStoreManagerForAnalysis{
		tmpDir: tmpDir,
	}

	analyzer := NewAnalyzer(mgr, &mockTemplateStoreWithTemplates{templates: templates})

	err := analyzer.executeRulesFromTemplates(context.Background(), "test-project")
	if err != nil {
		t.Errorf("executeRulesFromTemplates failed: %v", err)
	}
}

func TestRunPostIngestAnalysis_VersionCheck(t *testing.T) {
	s, tmpDir, cleanup := createTestStore(t)
	defer cleanup()

	s.AddFact(meb.Fact{Subject: "analytics", Predicate: "analytics_version", Object: CurrentAnalyticsVersion})

	mgr := &mockStoreManagerForAnalysis{
		analyticalStore: s,
		tmpDir:          tmpDir,
	}

	analyzer := NewAnalyzer(mgr, &mockTemplateStoreWithTemplates{templates: []*TemplateStoreQuery{}})

	err := analyzer.RunPostIngestAnalysis(context.Background(), "test-project")
	if err != nil {
		t.Errorf("RunPostIngestAnalysis failed: %v", err)
	}
}

