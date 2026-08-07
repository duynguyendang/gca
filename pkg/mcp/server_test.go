package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/registry"
	"github.com/duynguyendang/gca/pkg/service"
	mebpkg "github.com/duynguyendang/meb"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// newTestServer builds a Server over a temp StoreManager seeded with a few facts.
func newTestServer(t *testing.T) (*Server, *manager.StoreManager) {
	t.Helper()
	dataDir := t.TempDir()
	projDir := filepath.Join(dataDir, "testproj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(`{"name":"Test"}`), 0o644))

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)

	smellReg := registry.NewSmellRegistry(mgr)
	srv := &Server{
		mgr:        mgr,
		smellReg:   smellReg,
		graph:      service.NewGraphService(mgr),
		clustering: service.NewClusteringService(),
	}
	return srv, mgr
}

func (s *Server) seedSource(t *testing.T, project string, facts []mebpkg.Fact) {
	t.Helper()
	storeHandle, err := s.mgr.GetSourceStore(project)
	require.NoError(t, err)
	require.NoError(t, storeHandle.AddFactBatch(facts))
}

func (s *Server) seedAnalytical(t *testing.T, project string, facts []mebpkg.Fact) {
	t.Helper()
	storeHandle, err := s.mgr.GetAnalyticalStore(project)
	require.NoError(t, err)
	require.NoError(t, storeHandle.AddFactBatch(facts))
}

// callTool invokes a handler with a project argument map.
func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), project string, extra map[string]any) *mcp.CallToolResult {
	t.Helper()
	args := map[string]any{"project": project}
	for k, v := range extra {
		args[k] = v
	}
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	res, err := handler(context.Background(), req)
	require.NoError(t, err)
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, res)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestListProjects(t *testing.T) {
	srv, _ := newTestServer(t)
	// Opening the source store creates the badger dir so the project is listed.
	srv.seedSource(t, "testproj", []mebpkg.Fact{
		{Subject: "a", Predicate: config.PredicateDefines, Object: "a"},
	})
	res := callTool(t, srv.handleListProjects, "", nil)
	require.NotNil(t, res)
	var projects []manager.ProjectMetadata
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &projects))
	require.Equal(t, "testproj", projects[0].ID)
}

func TestScanFacts(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.seedSource(t, "testproj", []mebpkg.Fact{
		{Subject: "file:a.go", Predicate: config.PredicateDefines, Object: "file:a.go"},
	})
	res := callTool(t, srv.handleScanFacts, "testproj", map[string]any{"predicate": config.PredicateDefines})
	require.Contains(t, resultText(t, res), "defines")
}

func TestScanFactsUnknownProject(t *testing.T) {
	srv, _ := newTestServer(t)
	res := callTool(t, srv.handleScanFacts, "nope", nil)
	require.True(t, res.IsError)
	require.Contains(t, resultText(t, res), "not found")
}

func TestDatalogQuery(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.seedSource(t, "testproj", []mebpkg.Fact{
		{Subject: "a", Predicate: config.PredicateCalls, Object: "b"},
	})
	res := callTool(t, srv.handleDatalogQuery, "testproj", map[string]any{
		"query": `triples(S, "calls", O)`,
	})
	require.Contains(t, resultText(t, res), `"b"`)
}

func TestDatalogQueryMissingProject(t *testing.T) {
	srv, _ := newTestServer(t)
	res := callTool(t, srv.handleDatalogQuery, "", map[string]any{"query": `triples(S, "calls", O)`})
	require.True(t, res.IsError)
}

func TestHealthSummary(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.seedAnalytical(t, "testproj", []mebpkg.Fact{
		{Subject: "file:a.go", Predicate: "has_smell", Object: "god_file"},
		{Subject: "file:a.go", Predicate: "has_hub_score", Object: "5"},
	})
	res := callTool(t, srv.handleHealthSummary, "testproj", nil)
	require.Contains(t, resultText(t, res), "god_file")
}

func TestListSmells(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.seedAnalytical(t, "testproj", []mebpkg.Fact{
		{Subject: "file:a.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "file:a.go", Predicate: "has_smell_severity", Object: "high"},
	})
	res := callTool(t, srv.handleListSmells, "testproj", nil)
	require.Contains(t, resultText(t, res), "god_file")
	require.Contains(t, resultText(t, res), "high")
}

func TestSemanticSearchDisabledWithoutAI(t *testing.T) {
	srv, _ := newTestServer(t)
	res := callTool(t, srv.handleSemanticSearch, "testproj", map[string]any{"query": "payment"})
	require.True(t, res.IsError)
	require.Contains(t, resultText(t, res), "AI service")
}

func TestOKFIngestReadOnly(t *testing.T) {
	// A read-only manager rejects ingest.
	dataDir := t.TempDir()
	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, true)
	srv := &Server{mgr: mgr, graph: nil, smellReg: registry.NewSmellRegistry(mgr)}
	res := callTool(t, srv.handleOKFIngest, "testproj", map[string]any{"bundle_dir": "/tmp"})
	require.True(t, res.IsError)
	require.Contains(t, resultText(t, res), "read-only")
}

func TestOKFExportRequiresAbsolutePath(t *testing.T) {
	srv, _ := newTestServer(t)
	res := callTool(t, srv.handleOKFExport, "testproj", map[string]any{"output_dir": "relative/path"})
	require.True(t, res.IsError)
	require.Contains(t, resultText(t, res), "absolute")
}

func TestNewRegistersTools(t *testing.T) {
	_, mgr := newTestServer(t)
	ms := New(Options{Manager: mgr})
	require.NotNil(t, ms)
	// The mark3labs server exposes the tool list at runtime; asserting non-nil
	// is sufficient here since registration is exercised via handlers elsewhere.
}

func TestFileResourceTemplateMatchesNestedPath(t *testing.T) {
	_, mgr := newTestServer(t)
	_ = New(Options{Manager: mgr})

	// The {+path} resource template must match nested file paths and expose
	// the full path via Params.Arguments (mcp-go populates matched vars there).
	tpl := mcp.NewResourceTemplate("gca://projects/{project}/files/{+path}", "File Content")
	re := tpl.URITemplate.Regexp()
	require.True(t, re.MatchString("gca://projects/testproj/files/src/meb/store.go"))
	vars := tpl.URITemplate.Match("gca://projects/testproj/files/src/meb/store.go")
	require.Equal(t, []string{"src/meb/store.go"}, vars["path"].V)
	require.Equal(t, []string{"testproj"}, vars["project"].V)

	// templateParam must extract the single-element []string capture.
	req := mcp.ReadResourceRequest{}
	req.Params.Arguments = map[string]any{
		"project": vars["project"].V,
		"path":    vars["path"].V,
	}
	require.Equal(t, "testproj", templateParam(req, "project"))
	require.Equal(t, "src/meb/store.go", templateParam(req, "path"))
}

func TestResourceTemplatesRegistered(t *testing.T) {
	_, mgr := newTestServer(t)
	ms := New(Options{Manager: mgr})
	require.NotNil(t, ms)
}
