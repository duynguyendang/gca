package ingest

import (
	"context"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

func TestMockStore_AddFact(t *testing.T) {
	store := NewMockStore()
	fact := meb.Fact{Subject: "test.go", Predicate: "defines", Object: "main"}

	err := store.AddFact(fact)
	if err != nil {
		t.Errorf("AddFact should not error: %v", err)
	}
	if len(store.Facts) != 1 {
		t.Errorf("Store should have 1 fact, got %d", len(store.Facts))
	}
}

func TestMockStore_AddFactBatch(t *testing.T) {
	store := NewMockStore()
	facts := []meb.Fact{
		{Subject: "test.go", Predicate: "defines", Object: "main"},
		{Subject: "test.go", Predicate: "imports", Object: "fmt"},
	}

	err := store.AddFactBatch(facts)
	if err != nil {
		t.Errorf("AddFactBatch should not error: %v", err)
	}
	if len(store.Facts) != 2 {
		t.Errorf("Store should have 2 facts, got %d", len(store.Facts))
	}
}

func TestMockStore_DeleteFactsBySubject(t *testing.T) {
	store := NewMockStore()
	store.Facts = []meb.Fact{
		{Subject: "test.go", Predicate: "defines", Object: "main"},
		{Subject: "test.go", Predicate: "imports", Object: "fmt"},
		{Subject: "other.go", Predicate: "defines", Object: "helper"},
	}

	err := store.DeleteFactsBySubject("test.go")
	if err != nil {
		t.Errorf("DeleteFactsBySubject should not error: %v", err)
	}
	if len(store.Facts) != 1 {
		t.Errorf("Store should have 1 fact after delete, got %d", len(store.Facts))
	}
}

func TestMockStore_Scan(t *testing.T) {
	store := NewMockStore()
	store.Facts = []meb.Fact{
		{Subject: "test.go", Predicate: "defines", Object: "main"},
		{Subject: "test.go", Predicate: "imports", Object: "fmt"},
		{Subject: "other.go", Predicate: "defines", Object: "helper"},
	}

	count := 0
	for fact, err := range store.Scan("test.go", "", "") {
		if err != nil {
			t.Errorf("Scan should not return error: %v", err)
		}
		if fact.Subject == "test.go" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("Scan should return 2 facts for test.go, got %d", count)
	}
}

func TestMockStore_ScanByPredicate(t *testing.T) {
	store := NewMockStore()
	store.Facts = []meb.Fact{
		{Subject: "test.go", Predicate: "defines", Object: "main"},
		{Subject: "other.go", Predicate: "defines", Object: "helper"},
		{Subject: "test.go", Predicate: "imports", Object: "fmt"},
	}

	count := 0
	for _, err := range store.Scan("", "defines", "") {
		if err != nil {
			t.Errorf("Scan should not return error: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Errorf("Scan should return 2 facts with predicate defines, got %d", count)
	}
}

func TestMockStore_ScanByObject(t *testing.T) {
	store := NewMockStore()
	store.Facts = []meb.Fact{
		{Subject: "test.go", Predicate: "defines", Object: "main"},
		{Subject: "other.go", Predicate: "defines", Object: "helper"},
	}

	count := 0
	for _, err := range store.Scan("", "", "main") {
		if err != nil {
			t.Errorf("Scan should not return error: %v", err)
		}
		count++
	}
	if count != 1 {
		t.Errorf("Scan should return 1 fact with object main, got %d", count)
	}
}

func TestMockStore_GetContentByKey(t *testing.T) {
	store := NewMockStore()
	store.Documents["test.go"] = []byte("package main")

	content, err := store.GetContentByKey("test.go")
	if err != nil {
		t.Errorf("GetContentByKey should not error: %v", err)
	}
	if string(content) != "package main" {
		t.Errorf("GetContentByKey returned %q, want %q", string(content), "package main")
	}
}

func TestMockStore_GetContentByKey_Missing(t *testing.T) {
	store := NewMockStore()

	content, err := store.GetContentByKey("missing.go")
	if err != nil {
		t.Errorf("GetContentByKey should not error for missing key: %v", err)
	}
	if content != nil {
		t.Errorf("GetContentByKey should return nil for missing key")
	}
}

func TestMockStore_SetTopicID(t *testing.T) {
	store := NewMockStore()
	store.SetTopicID(123)

	if !store.Topics[123] {
		t.Error("Topic 123 should be set")
	}
}

func TestMockStoreManager_GetSourceStore(t *testing.T) {
	mgr := NewMockStoreManager()
	store, err := mgr.GetSourceStore("test-project")
	if err != nil {
		t.Errorf("GetSourceStore should not error: %v", err)
	}
	if store != nil {
		t.Error("GetSourceStore returns nil for MockStoreManager")
	}
}

func TestMockStoreManager_GetSourceStore_Error(t *testing.T) {
	mgr := NewMockStoreManager()
	mgr.SourceErr = context.DeadlineExceeded

	_, err := mgr.GetSourceStore("test-project")
	if err == nil {
		t.Error("GetSourceStore should return error when SourceErr is set")
	}
}

func TestMockStoreManager_GetAnalyticalStore(t *testing.T) {
	mgr := NewMockStoreManager()
	store, err := mgr.GetAnalyticalStore("test-project")
	if err != nil {
		t.Errorf("GetAnalyticalStore should not error: %v", err)
	}
	if store != nil {
		t.Error("GetAnalyticalStore returns nil for MockStoreManager")
	}
}

func TestMockTemplateStore_ListTemplates(t *testing.T) {
	ts := NewMockTemplateStore()
	ts.Templates = []*TemplateStoreQuery{
		{ID: "test_query", Body: "triples(A, 'calls', B)", Predicate: "has_test"},
	}

	templates, err := ts.ListTemplates(context.Background(), "test-project", "")
	if err != nil {
		t.Errorf("ListTemplates should not error: %v", err)
	}
	if len(templates) != 1 {
		t.Errorf("ListTemplates should return 1 template, got %d", len(templates))
	}
}

func TestMockTemplateStore_ListTemplates_Error(t *testing.T) {
	ts := NewMockTemplateStore()
	ts.Err = context.DeadlineExceeded

	_, err := ts.ListTemplates(context.Background(), "test-project", "")
	if err == nil {
		t.Error("ListTemplates should return error when Err is set")
	}
}

func TestMockTemplateStore_GetTemplate(t *testing.T) {
	ts := NewMockTemplateStore()
	ts.Templates = []*TemplateStoreQuery{
		{ID: "test_query", Body: "triples(A, 'calls', B)", Predicate: "has_test"},
	}

	template, err := ts.GetTemplate(context.Background(), "test-project", "test_query")
	if err != nil {
		t.Errorf("GetTemplate should not error: %v", err)
	}
	if template == nil {
		t.Fatal("GetTemplate should return template")
	}
	if template.ID != "test_query" {
		t.Errorf("GetTemplate.ID = %q, want %q", template.ID, "test_query")
	}
}

func TestMockTemplateStore_GetTemplate_NotFound(t *testing.T) {
	ts := NewMockTemplateStore()

	template, err := ts.GetTemplate(context.Background(), "test-project", "missing")
	if err != nil {
		t.Errorf("GetTemplate should not error for missing template: %v", err)
	}
	if template != nil {
		t.Error("GetTemplate should return nil for missing template")
	}
}

func TestShouldInjectTag_NotTagged(t *testing.T) {
	store := NewMockStore()
	result := shouldInjectTag(store, "test.go", "backend")
	if !result {
		t.Error("shouldInjectTag should return true when file is not tagged")
	}
}

func TestShouldInjectTag_AlreadyTagged(t *testing.T) {
	store := NewMockStore()
	store.Facts = []meb.Fact{
		{Subject: "test.go", Predicate: config.PredicateHasTag, Object: "backend"},
	}

	result := shouldInjectTag(store, "test.go", "backend")
	if result {
		t.Error("shouldInjectTag should return false when file is already tagged")
	}
}

func TestShouldInjectTag_DifferentTag(t *testing.T) {
	store := NewMockStore()
	store.Facts = []meb.Fact{
		{Subject: "test.go", Predicate: config.PredicateHasTag, Object: "frontend"},
	}

	result := shouldInjectTag(store, "test.go", "backend")
	if !result {
		t.Error("shouldInjectTag should return true when file has different tag")
	}
}

func TestSafeAddFact_NewFact(t *testing.T) {
	store := NewMockStore()

	result := safeAddFact(store, "test.go", "defines", "main")
	if !result {
		t.Error("safeAddFact should return true for new fact")
	}
	if len(store.Facts) != 1 {
		t.Errorf("Store should have 1 fact, got %d", len(store.Facts))
	}
}

func TestSafeAddFact_Duplicate(t *testing.T) {
	store := NewMockStore()
	store.Facts = []meb.Fact{
		{Subject: "test.go", Predicate: "defines", Object: "main"},
	}

	result := safeAddFact(store, "test.go", "defines", "main")
	if result {
		t.Error("safeAddFact should return false for duplicate fact")
	}
	if len(store.Facts) != 1 {
		t.Errorf("Store should still have 1 fact, got %d", len(store.Facts))
	}
}

func TestUpsertFact(t *testing.T) {
	store := NewMockStore()

	upsertFact(store, "test.go", "handled_by", "handler1")
	if len(store.Facts) != 1 {
		t.Errorf("Store should have 1 fact after upsert, got %d", len(store.Facts))
	}

	upsertFact(store, "test.go", "handled_by", "handler2")
	if len(store.Facts) != 1 {
		t.Errorf("Store should still have 1 fact after second upsert, got %d", len(store.Facts))
	}

	for _, f := range store.Facts {
		if obj, ok := f.Object.(string); ok {
			if obj != "handler2" {
				t.Errorf("Fact object should be handler2, got %q", obj)
			}
		}
	}
}

func TestVirtualFactMu(t *testing.T) {
	// Verify virtualFactMu is accessible (package-level var, not copied).
	virtualFactMu.Lock()
	virtualFactMu.Unlock()
}

func TestEnhanceVirtualTriples_EmptyStore(t *testing.T) {
	store := NewMockStore()
	err := EnhanceVirtualTriples(store)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples should not error with empty store: %v", err)
	}
}

func TestEnhanceVirtualTriples_WithFrontendTag(t *testing.T) {
	store := NewMockStore()
	store.Documents["main.go"] = []byte("package main")
	store.Facts = []meb.Fact{
		{Subject: "main.go", Predicate: config.PredicateHasTag, Object: "frontend"},
	}

	err := EnhanceVirtualTriples(store)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples should not error: %v", err)
	}
}

func TestEnhanceVirtualTriples_WithBackendTag(t *testing.T) {
	store := NewMockStore()
	store.Documents["server.go"] = []byte("package server")
	store.Facts = []meb.Fact{
		{Subject: "server.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "server.go", Predicate: config.PredicateDefines, Object: "main"},
	}

	err := EnhanceVirtualTriples(store)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples should not error: %v", err)
	}
}

func TestEnhanceVirtualTriples_WithBothTags(t *testing.T) {
	store := NewMockStore()
	store.Documents["main.go"] = []byte("package main")
	store.Documents["server.go"] = []byte("package server")
	store.Facts = []meb.Fact{
		{Subject: "main.go", Predicate: config.PredicateHasTag, Object: "frontend"},
		{Subject: "server.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "server.go", Predicate: config.PredicateDefines, Object: "main"},
	}

	err := EnhanceVirtualTriples(store)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples should not error: %v", err)
	}
}

func TestEnhanceVirtualTriples_WithSymbolDefinition(t *testing.T) {
	store := NewMockStore()
	store.Documents["server.go"] = []byte("package server")
	store.Facts = []meb.Fact{
		{Subject: "server.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "server.go", Predicate: config.PredicateDefines, Object: "Handler"},
		{Subject: "Handler", Predicate: config.PredicateDefines, Object: "ServeHTTP"},
	}

	err := EnhanceVirtualTriples(store)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples should not error: %v", err)
	}
}

func TestEnhanceVirtualTriples_FrontendWithRoute(t *testing.T) {
	store := NewMockStore()
	store.Documents["api_handler.go"] = []byte(`
package main
import "github.com/gin-gonic/gin"
func main() {
	r := gin.Default()
	r.GET("/test", testHandler)
}`)

	store.Facts = []meb.Fact{
		{Subject: "api_handler.go", Predicate: config.PredicateHasTag, Object: "backend"},
		{Subject: "api_handler.go", Predicate: config.PredicateDefines, Object: "testHandler"},
	}

	err := EnhanceVirtualTriples(store)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples should not error with gin router: %v", err)
	}
}

func TestEnhanceVirtualTriples_EmptyStoreScenario(t *testing.T) {
	store := NewMockStore()
	err := EnhanceVirtualTriples(store)
	if err != nil {
		t.Errorf("EnhanceVirtualTriples should not error with empty store: %v", err)
	}
}

func TestConfigDrivenTagMatcher(t *testing.T) {
	store := NewMockStore()
	cfg := getTagConfig()

	configDrivenTagMatcher(store, "api_handler.go", cfg)

	found := false
	for _, f := range store.Facts {
		if f.Predicate == config.PredicateHasTag && f.Subject == "api_handler.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("configDrivenTagMatcher should add tag for file matching pattern")
	}
}

func TestConfigDrivenTagMatcher_SymbolID(t *testing.T) {
	store := NewMockStore()
	cfg := getTagConfig()

	configDrivenTagMatcher(store, "main.go:main", cfg)

	if len(store.Facts) != 0 {
		t.Error("configDrivenTagMatcher should not add tags for symbol-level IDs")
	}
}

func TestMockStore_StoreInterface(t *testing.T) {
	store := NewMockStore()
	var _ Store = store
}

func TestAnalyzerWithMocks(t *testing.T) {
	storeManager := NewMockStoreManager()
	templateStore := NewMockTemplateStore()

	analyzer := NewAnalyzer(storeManager, templateStore)
	if analyzer == nil {
		t.Fatal("NewAnalyzer with mocks returned nil")
	}
	if analyzer.storeManager != storeManager {
		t.Error("Analyzer.storeManager should be set from constructor")
	}
	if analyzer.templateStore != templateStore {
		t.Error("Analyzer.templateStore should be set from constructor")
	}
}

func TestAnalyzer_NewAnalyzer_Checks(t *testing.T) {
	storeManager := NewMockStoreManager()
	templateStore := NewMockTemplateStore()

	analyzer := NewAnalyzer(storeManager, templateStore)

	if analyzer == nil {
		t.Fatal("NewAnalyzer should not return nil")
	}

	if analyzer.storeManager == nil {
		t.Error("Analyzer.storeManager should be initialized")
	}

	if analyzer.templateStore == nil {
		t.Error("Analyzer.templateStore should be initialized")
	}
}

func TestMockStoreManager_ImplementsInterface(t *testing.T) {
	mgr := NewMockStoreManager()
	var _ StoreManagerInterface = mgr
}

func TestMockTemplateStore_ImplementsInterface(t *testing.T) {
	ts := NewMockTemplateStore()
	var _ TemplateStoreInterface = ts
}
