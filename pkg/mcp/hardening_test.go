package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/registry"
	mebpkg "github.com/duynguyendang/meb"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// errBody is the JSON envelope returned on tool errors.
type errBody struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details string `json:"details"`
}

func decodeErr(t *testing.T, res *mcp.CallToolResult) errBody {
	t.Helper()
	require.True(t, res.IsError)
	var body errBody
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &body))
	return body
}

func TestErrorEnvelopeProjectNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	res := callTool(t, srv.handleScanFacts, "nope", nil)
	body := decodeErr(t, res)
	require.Equal(t, ErrCodeProjectNotFound, body.Code)
	require.Contains(t, body.Error, "not found")
}

func TestErrorEnvelopeArgumentRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	res := callTool(t, srv.handleDatalogQuery, "", map[string]any{"query": `triples(S, "calls", O)`})
	body := decodeErr(t, res)
	require.Equal(t, ErrCodeInvalidArgument, body.Code)
	require.Contains(t, body.Error, "argument required")
}

func TestErrorEnvelopeReadOnly(t *testing.T) {
	dataDir := t.TempDir()
	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, true)
	srv := &Server{mgr: mgr, smellReg: registry.NewSmellRegistry(mgr)}
	res := callTool(t, srv.handleOKFIngest, "testproj", map[string]any{"bundle_dir": "/tmp"})
	body := decodeErr(t, res)
	require.Equal(t, ErrCodeReadOnly, body.Code)
}

func TestErrorEnvelopeUnavailable(t *testing.T) {
	srv, _ := newTestServer(t)
	res := callTool(t, srv.handleSemanticSearch, "testproj", map[string]any{"query": "payment"})
	body := decodeErr(t, res)
	require.Equal(t, ErrCodeUnavailable, body.Code)
}

func TestScanFactsPagination(t *testing.T) {
	srv, _ := newTestServer(t)
	var facts []mebpkg.Fact
	for i := 0; i < 5; i++ {
		facts = append(facts, mebpkg.Fact{
			Subject:   "file:f.go",
			Predicate: config.PredicateDefines,
			Object:    fmt.Sprintf("sym%d", i),
		})
	}
	srv.seedSource(t, "testproj", facts)

	fetch := func(cursor string) ([]string, int, string) {
		args := map[string]any{"predicate": config.PredicateDefines, "limit": float64(2)}
		if cursor != "" {
			args["cursor"] = cursor
		}
		res := callTool(t, srv.handleScanFacts, "testproj", args)
		var page struct {
			Facts      []string `json:"facts"`
			Count      int      `json:"count"`
			NextCursor string   `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &page))
		return page.Facts, page.Count, page.NextCursor
	}

	f1, c1, cur1 := fetch("")
	require.Equal(t, 2, c1)
	require.Len(t, f1, 2)
	require.NotEmpty(t, cur1)

	f2, c2, cur2 := fetch(cur1)
	require.Equal(t, 2, c2)
	require.Len(t, f2, 2)
	require.NotEmpty(t, cur2)

	f3, c3, cur3 := fetch(cur2)
	require.Equal(t, 1, c3)
	require.Len(t, f3, 1)
	require.Empty(t, cur3)
}

func TestDatalogQueryPagination(t *testing.T) {
	srv, _ := newTestServer(t)
	var facts []mebpkg.Fact
	for i := 0; i < 5; i++ {
		facts = append(facts, mebpkg.Fact{
			Subject:   fmt.Sprintf("a%d", i),
			Predicate: config.PredicateCalls,
			Object:    "b",
		})
	}
	srv.seedSource(t, "testproj", facts)

	fetch := func(cursor string) (int, string) {
		args := map[string]any{"query": `triples(S, "calls", O)`, "limit": float64(2)}
		if cursor != "" {
			args["cursor"] = cursor
		}
		res := callTool(t, srv.handleDatalogQuery, "testproj", args)
		var page struct {
			Results    []map[string]any `json:"results"`
			Count      int              `json:"count"`
			NextCursor string           `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &page))
		return page.Count, page.NextCursor
	}

	c1, cur1 := fetch("")
	require.Equal(t, 2, c1)
	require.NotEmpty(t, cur1)

	c2, cur2 := fetch(cur1)
	require.Equal(t, 2, c2)
	require.NotEmpty(t, cur2)

	c3, cur3 := fetch(cur2)
	require.Equal(t, 1, c3)
	require.Empty(t, cur3)
}

func TestListSmellsFilters(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.seedAnalytical(t, "testproj", []mebpkg.Fact{
		{Subject: "file:a.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "file:a.go", Predicate: "has_smell_severity", Object: "high"},
		{Subject: "file:b.go", Predicate: "has_smell_type", Object: "missing_error_check"},
		{Subject: "file:b.go", Predicate: "has_smell_severity", Object: "medium"},
	})

	res := callTool(t, srv.handleListSmells, "testproj", map[string]any{"severity": "high"})
	var high []smellEntry
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &high))
	require.Len(t, high, 1)
	require.Equal(t, "god_file", high[0].Type)

	res = callTool(t, srv.handleListSmells, "testproj", map[string]any{"type": "missing_error_check"})
	var typed []smellEntry
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &typed))
	require.Len(t, typed, 1)
	require.Equal(t, "missing_error_check", typed[0].Type)
	require.Equal(t, "medium", typed[0].Severity)

	res = callTool(t, srv.handleListSmells, "testproj", map[string]any{"severity": "high", "type": "missing_error_check"})
	var none []smellEntry
	require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &none))
	require.Empty(t, none)
}

func TestReadOnlyRegistersNoProvisioningTools(t *testing.T) {
	_, mgr := newTestServer(t)
	require.False(t, mgr.ReadOnly())
	ms := New(Options{Manager: mgr})
	require.NotNil(t, ms.GetTool("okf_ingest"))
	require.NotNil(t, ms.GetTool("ingest_incremental"))

	// A read-only manager must not expose the write tools.
	dataDir := t.TempDir()
	roMgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, true)
	ms2 := New(Options{Manager: roMgr})
	require.Nil(t, ms2.GetTool("okf_ingest"))
	require.Nil(t, ms2.GetTool("ingest_incremental"))
	require.NotNil(t, ms2.GetTool("okf_export"))
	require.NotNil(t, ms2.GetTool("scan_facts"))
}

func TestSmellsResource(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.seedAnalytical(t, "testproj", []mebpkg.Fact{
		{Subject: "file:a.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "file:a.go", Predicate: "has_smell_severity", Object: "high"},
	})
	req := mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{URI: "gca://projects/testproj/smells"}}
	req.Params.Arguments = map[string]any{"project": []string{"testproj"}}
	contents, err := srv.handleSmellsResource(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, contents, 1)
	text := contents[0].(mcp.TextResourceContents)
	require.Contains(t, text.Text, "god_file")
}

func TestHealthResource(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.seedAnalytical(t, "testproj", []mebpkg.Fact{
		{Subject: "file:a.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "file:a.go", Predicate: "has_hub_score", Object: "5"},
	})
	req := mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{URI: "gca://projects/testproj/health"}}
	req.Params.Arguments = map[string]any{"project": []string{"testproj"}}
	contents, err := srv.handleHealthResource(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, contents, 1)
	text := contents[0].(mcp.TextResourceContents)
	require.Contains(t, text.Text, "overall_score")
}
