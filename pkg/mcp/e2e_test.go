package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	mebpkg "github.com/duynguyendang/meb"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// newSeededManager builds a manager with a "testproj" that has source+analytical facts.
func newSeededManager(t *testing.T, readOnly bool) *manager.StoreManager {
	t.Helper()
	dataDir := t.TempDir()
	projDir := filepath.Join(dataDir, "testproj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(`{"name":"Test"}`), 0o644))

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, readOnly)

	// Read-only managers can't open stores for writing; tests that need data
	// pass readOnly=false and seed below.
	if readOnly {
		return mgr
	}

	src, err := mgr.GetSourceStore("testproj")
	require.NoError(t, err)
	require.NoError(t, src.AddFactBatch([]mebpkg.Fact{
		{Subject: "file:a.go", Predicate: config.PredicateDefines, Object: "funcA"},
		{Subject: "file:a.go", Predicate: config.PredicateCalls, Object: "funcB"},
	}))

	an, err := mgr.GetAnalyticalStore("testproj")
	require.NoError(t, err)
	require.NoError(t, an.AddFactBatch([]mebpkg.Fact{
		{Subject: "file:a.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "file:a.go", Predicate: "has_smell_severity", Object: "high"},
	}))
	return mgr
}

// newInProcessClient builds a client over an in-process MCP server and initializes it.
func newInProcessClient(t *testing.T, readOnly bool) (*mcpclient.Client, context.CancelFunc) {
	t.Helper()
	mgr := newSeededManager(t, readOnly)
	ms := New(Options{Manager: mgr})
	client, err := mcpclient.NewInProcessClient(ms)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "gca-test", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	})
	require.NoError(t, err)
	return client, func() {
		cancel()
		client.Close()
	}
}

func TestE2EInProcessListToolsGated(t *testing.T) {
	client, closeFn := newInProcessClient(t, false)
	defer closeFn()
	res, err := client.ListTools(context.Background(), mcp.ListToolsRequest{})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	require.True(t, names["okf_ingest"])
	require.True(t, names["ingest_incremental"])
	require.True(t, names["scan_facts"])
}

func TestE2EInProcessListToolsReadOnly(t *testing.T) {
	client, closeFn := newInProcessClient(t, true)
	defer closeFn()
	res, err := client.ListTools(context.Background(), mcp.ListToolsRequest{})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	require.False(t, names["okf_ingest"])
	require.False(t, names["ingest_incremental"])
	require.True(t, names["scan_facts"])
	require.True(t, names["okf_export"])
}

func TestE2EInProcessCallToolAndErrorEnvelope(t *testing.T) {
	client, closeFn := newInProcessClient(t, false)
	defer closeFn()
	ctx := context.Background()

	res, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "scan_facts", Arguments: map[string]any{
			"project":   "testproj",
			"predicate": config.PredicateCalls,
		}},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	body := resultText(t, res)
	require.Contains(t, body, "funcB")

	// Error path: unknown project must carry a project_not_found envelope.
	errRes, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "scan_facts", Arguments: map[string]any{
			"project":   "nope",
			"predicate": config.PredicateCalls,
		}},
	})
	require.NoError(t, err)
	env := decodeErr(t, errRes)
	require.Equal(t, ErrCodeProjectNotFound, env.Code)
}

func TestE2EInProcessReadSmellsResource(t *testing.T) {
	client, closeFn := newInProcessClient(t, false)
	defer closeFn()
	ctx := context.Background()

	res, err := client.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "gca://projects/testproj/smells"},
	})
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)
	text, ok := res.Contents[0].(mcp.TextResourceContents)
	require.True(t, ok)
	require.Contains(t, text.Text, "god_file")
}

func TestE2EHTTP(t *testing.T) {
	mgr := newSeededManager(t, false)
	handler := NewHTTPServer(mgr, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client, err := mcpclient.NewStreamableHttpClient(srv.URL)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer client.Close()

	_, err = client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "gca-test", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	})
	require.NoError(t, err)

	res, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	require.Greater(t, len(res.Tools), 0)

	callRes, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "scan_facts", Arguments: map[string]any{
			"project":   "testproj",
			"predicate": config.PredicateDefines,
		}},
	})
	require.NoError(t, err)
	require.False(t, callRes.IsError)
	require.Contains(t, resultText(t, callRes), "funcA")
}

func TestE2EHTTPReadOnlyEnvelope(t *testing.T) {
	mgr := newSeededManager(t, true)
	handler := NewHTTPServer(mgr, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client, err := mcpclient.NewStreamableHttpClient(srv.URL)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer client.Close()

	_, err = client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "gca-test", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	})
	require.NoError(t, err)

	// Read-only server must not expose provisioning tools.
	res, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	require.False(t, names["okf_ingest"])
	require.False(t, names["ingest_incremental"])
	require.True(t, names["scan_facts"])
}

func TestE2EPaginationAcrossTransports(t *testing.T) {
	// Seed 5 define facts to paginate.
	dataDir := t.TempDir()
	projDir := filepath.Join(dataDir, "testproj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(`{"name":"Test"}`), 0o644))
	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	src, err := mgr.GetSourceStore("testproj")
	require.NoError(t, err)
	var facts []mebpkg.Fact
	for i := 0; i < 5; i++ {
		facts = append(facts, mebpkg.Fact{
			Subject:   "file:f.go",
			Predicate: config.PredicateDefines,
			Object:    fmt.Sprintf("sym%d", i),
		})
	}
	require.NoError(t, src.AddFactBatch(facts))

	ms := New(Options{Manager: mgr})
	client, err := mcpclient.NewInProcessClient(ms)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer client.Close()
	_, err = client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "gca-test", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	})
	require.NoError(t, err)

	fetch := func(cursor string) ([]string, string) {
		args := map[string]any{"project": "testproj", "predicate": config.PredicateDefines, "limit": float64(2)}
		if cursor != "" {
			args["cursor"] = cursor
		}
		res, err := client.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "scan_facts", Arguments: args}})
		require.NoError(t, err)
		var page struct {
			Facts      []string `json:"facts"`
			NextCursor string   `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal([]byte(resultText(t, res)), &page))
		return page.Facts, page.NextCursor
	}

	f1, cur1 := fetch("")
	require.Len(t, f1, 2)
	f2, cur2 := fetch(cur1)
	require.Len(t, f2, 2)
	f3, cur3 := fetch(cur2)
	require.Len(t, f3, 1)
	require.Empty(t, cur3)
}
