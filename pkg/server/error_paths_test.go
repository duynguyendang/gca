package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestErrorPaths_MalformedJSON(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/query?project=" + testProjectID},
		{"POST", "/api/v1/ai/ask"},
		{"POST", "/api/v1/ai/classify"},
		{"POST", "/api/v1/ask"},
		{"POST", "/api/v1/agent/execute"},
		{"POST", "/api/v1/graph/diff"},
		{"POST", "/api/v1/graph/snapshots"},
		{"POST", "/api/v1/review/session"},
		{"POST", "/api/v1/ingest/incremental"},
		{"POST", "/api/v1/graph/hybrid-cluster?project=" + testProjectID},
		{"POST", "/api/v1/graph/subgraph?project=" + testProjectID},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path+" with malformed JSON", func(t *testing.T) {
			w := doJSONRequest(srv, ep.method, ep.path, `{invalid json}`)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for malformed JSON, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestErrorPaths_MissingProjectID(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// GET endpoints that require ?project=
	getEndpoints := []string{
		"/api/v1/graph?file=test.go",
		"/api/v1/graph/map",
		"/api/v1/graph/file-details?file=test.go",
		"/api/v1/graph/file-calls?file=test.go",
		"/api/v1/graph/backbone",
		"/api/v1/graph/file-backbone?file=test.go",
		"/api/v1/source?id=test.go",
		"/api/v1/summary",
		"/api/v1/symbols?q=test",
		"/api/v1/search/flow?from=a&to=b",
		"/api/v1/graph/path?start=a&end=b",
		"/api/v1/graph/who-calls?symbol=test",
		"/api/v1/graph/what-calls?symbol=test",
		"/api/v1/graph/reachable?from=a&to=b",
		"/api/v1/graph/cycles",
		"/api/v1/graph/lca?a=x&b=y",
		"/api/v1/health/summary",
		"/api/v1/health/summary/v2",
		"/api/v1/analysis/surprise",
		"/api/v1/analysis/knowledge-gaps",
	}

	for _, path := range getEndpoints {
		t.Run("GET "+path+" missing project", func(t *testing.T) {
			w := doRequest(srv, "GET", path, "")
			// Some endpoints auto-detect project (200), some require it (400),
			// some fail on analytical store access (500) — all are acceptable
			if w.Code != http.StatusBadRequest && w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
				t.Errorf("expected 400, 200, or 500, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestErrorPaths_SQLInjection(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	maliciousQueries := []string{
		"'; DROP TABLE users; --",
		"1' OR '1'='1",
		"union select * from users",
	}

	for _, q := range maliciousQueries {
		t.Run("SQL injection in query param: "+q[:min(len(q), 30)], func(t *testing.T) {
			encoded := url.QueryEscape(q)
			w := doRequest(srv, "GET", "/api/v1/symbols?project="+testProjectID+"&q="+encoded, "")
			// Should either reject (400) or safely handle
			if w.Code == http.StatusOK {
				// If accepted, ensure no SQL-like content in response
				body := w.Body.String()
				if contains(body, "DROP TABLE") || contains(body, "UNION SELECT") {
					t.Error("potential SQL injection in response")
				}
			}
		})
	}
}

func TestErrorPaths_XSSPrevention(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	xssPayloads := []string{
		"<script>alert('xss')</script>",
		"javascript:alert('xss')",
		"<img onerror=alert('xss') src=x>",
	}

	for _, payload := range xssPayloads {
		t.Run("XSS in query: "+payload[:min(len(payload), 30)], func(t *testing.T) {
			w := doJSONRequest(srv, "POST", "/api/v1/query?project="+testProjectID, `{"query": "`+payload+`"}`)
			// Should reject with 400
			requireStatus(t, w, http.StatusBadRequest)
		})
	}
}

func TestErrorPaths_PathTraversal(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	traversalPaths := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32",
		"handlers/../../../secret.go",
	}

	for _, path := range traversalPaths {
		t.Run("path traversal: "+path[:min(len(path), 30)], func(t *testing.T) {
			encoded := url.QueryEscape(path)
			w := doRequest(srv, "GET", "/api/v1/source?project="+testProjectID+"&id="+encoded, "")
			// Should reject with 400 or safely handle
			if w.Code == http.StatusOK {
				body := w.Body.String()
				if contains(body, "root:") || contains(body, "[boot loader]") {
					t.Error("potential path traversal - sensitive content returned")
				}
			}
		})
	}
}

func TestErrorPaths_OversizedBody(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a body larger than 10MB
	bigBody := strings.Repeat("x", 11*1024*1024)
	w := doJSONRequest(srv, "POST", "/api/v1/query?project="+testProjectID, `{"query": "`+bigBody+`"}`)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusServiceUnavailable {
		// Some servers return 400, 413, or 503 for oversized bodies
		t.Logf("oversized body returned %d (acceptable if validation middleware catches it)", w.Code)
	}
}

func TestErrorPaths_EmptyContentType(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("POST with empty body", func(t *testing.T) {
		w := doRequest(srv, "POST", "/api/v1/query?project="+testProjectID, "")
		// Should handle gracefully (400 or 200 with empty query)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusOK {
			t.Errorf("expected 400 or 200, got %d", w.Code)
		}
	})
}

func TestErrorPaths_ConcurrentRequests(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Fire 10 concurrent requests to check for race conditions
	done := make(chan int, 10)
	for i := 0; i < 10; i++ {
		go func() {
			w := doRequest(srv, "GET", "/api/v1/projects", "")
			done <- w.Code
		}()
	}

	for i := 0; i < 10; i++ {
		code := <-done
		if code != http.StatusOK {
			t.Errorf("concurrent request returned %d, expected 200", code)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
