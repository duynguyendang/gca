package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/config"
	mebpkg "github.com/duynguyendang/meb"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// verify_mcp launches the gca `mcp` subprocess over stdio and verifies the
// MCP hardening contract end to end:
//
//  1. Read-only by default: okf_ingest / ingest_incremental are NOT exposed.
//  2. Error envelope: unknown project returns {error, code: project_not_found}.
//  3. Pagination: scan_facts limit/cursor pages + next_cursor.
//  4. Resources: gca://projects/{project}/smells returns JSON.
//
// Usage: go run ./devtools/verify_mcp [path-to-gca-binary]
func main() {
	bin := "./gca"
	if len(os.Args) > 1 {
		bin = os.Args[1]
	}
	if _, err := os.Stat(bin); err != nil {
		log.Fatalf("gca binary not found at %s: %v", bin, err)
	}

	dataDir := seedDataDir()
	defer os.RemoveAll(dataDir)

	client, err := mcpclient.NewStdioMCPClient(bin, nil, "mcp", dataDir)
	if err != nil {
		log.Fatalf("failed to start stdio client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "gca-verify", Version: "1.0.0"},
			Capabilities:    mcp.ClientCapabilities{},
		},
	}); err != nil {
		log.Fatalf("initialize failed: %v", err)
	}

	// 1. Tool list must be gated for read-only servers.
	tools, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatalf("list tools failed: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if names["okf_ingest"] || names["ingest_incremental"] {
		log.Fatalf("FAIL: read-only server must not expose provisioning tools")
	}
	if !names["scan_facts"] || !names["okf_export"] {
		log.Fatalf("FAIL: expected read tools missing: scan_facts=%v okf_export=%v", names["scan_facts"], names["okf_export"])
	}
	fmt.Printf("PASS: %d tools, provisioning tools gated in read-only mode\n", len(tools.Tools))

	// 2. Error envelope for unknown project.
	errRes, err := client.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "scan_facts",
		Arguments: map[string]any{"project": "nope", "predicate": config.PredicateDefines},
	}})
	if err != nil {
		log.Fatalf("call scan_facts failed: %v", err)
	}
	if !errRes.IsError {
		log.Fatalf("FAIL: expected error for unknown project")
	}
	var env struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal([]byte(resultText(errRes)), &env); err != nil {
		log.Fatalf("FAIL: error envelope is not JSON: %v", err)
	}
	if env.Code != "project_not_found" {
		log.Fatalf("FAIL: expected code project_not_found, got %q", env.Code)
	}
	fmt.Printf("PASS: error envelope code=%s\n", env.Code)

	// 3. Pagination over scan_facts.
	fetchPage := func(cursor string) (int, string) {
		args := map[string]any{"project": "testproj", "predicate": config.PredicateDefines, "limit": float64(2)}
		if cursor != "" {
			args["cursor"] = cursor
		}
		res, err := client.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
			Name: "scan_facts", Arguments: args,
		}})
		if err != nil {
			log.Fatalf("call scan_facts failed: %v", err)
		}
		if res.IsError {
			log.Fatalf("FAIL: scan_facts error: %s", resultText(res))
		}
		var page struct {
			Facts      []string `json:"facts"`
			NextCursor string   `json:"next_cursor"`
		}
		if err := json.Unmarshal([]byte(resultText(res)), &page); err != nil {
			log.Fatalf("FAIL: scan_facts result not JSON: %v", err)
		}
		return len(page.Facts), page.NextCursor
	}

	c1, cur1 := fetchPage("")
	if c1 != 2 || cur1 == "" {
		log.Fatalf("FAIL: page1 count=%d next=%q, want 2 + cursor", c1, cur1)
	}
	c2, cur2 := fetchPage(cur1)
	if c2 != 2 || cur2 == "" {
		log.Fatalf("FAIL: page2 count=%d next=%q, want 2 + cursor", c2, cur2)
	}
	c3, cur3 := fetchPage(cur2)
	if c3 != 1 || cur3 != "" {
		log.Fatalf("FAIL: page3 count=%d next=%q, want 1 + empty", c3, cur3)
	}
	fmt.Println("PASS: scan_facts pagination (2/2/1 pages)")

	// 4. Smells resource.
	rr, err := client.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "gca://projects/testproj/smells"},
	})
	if err != nil {
		log.Fatalf("read smells resource failed: %v", err)
	}
	if len(rr.Contents) != 1 {
		log.Fatalf("FAIL: expected 1 resource content, got %d", len(rr.Contents))
	}
	text, ok := rr.Contents[0].(mcp.TextResourceContents)
	if !ok {
		log.Fatalf("FAIL: resource content is not text")
	}
	if !json.Valid([]byte(text.Text)) {
		log.Fatalf("FAIL: smells resource is not valid JSON")
	}
	fmt.Println("PASS: smells resource returns JSON")

	fmt.Println("ALL CHECKS PASSED")
}

// seedDataDir creates a temp data dir with a seeded test project and returns it.
func seedDataDir() string {
	dir, err := os.MkdirTemp("", "gca-verify-mcp-*")
	if err != nil {
		log.Fatalf("mkdirtemp: %v", err)
	}
	projDir := filepath.Join(dir, "testproj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		log.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(`{"name":"Test"}`), 0o644); err != nil {
		log.Fatalf("write metadata: %v", err)
	}

	mgr := manager.NewStoreManager(dir, manager.MemoryProfileDefault, false)
	src, err := mgr.GetSourceStore("testproj")
	if err != nil {
		log.Fatalf("open source store: %v", err)
	}
	var facts []mebpkg.Fact
	for i := 0; i < 5; i++ {
		facts = append(facts, mebpkg.Fact{
			Subject:   "file:f.go",
			Predicate: config.PredicateDefines,
			Object:    fmt.Sprintf("sym%d", i),
		})
	}
	if err := src.AddFactBatch(facts); err != nil {
		log.Fatalf("seed source facts: %v", err)
	}
	an, err := mgr.GetAnalyticalStore("testproj")
	if err != nil {
		log.Fatalf("open analytical store: %v", err)
	}
	if err := an.AddFactBatch([]mebpkg.Fact{
		{Subject: "file:a.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "file:a.go", Predicate: "has_smell_severity", Object: "high"},
	}); err != nil {
		log.Fatalf("seed analytical facts: %v", err)
	}
	mgr.CloseAll()
	return dir
}

// resultText extracts the text from a CallToolResult (mirrors the test helper).
func resultText(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if text, ok := res.Content[0].(mcp.TextContent); ok {
		return text.Text
	}
	return ""
}
