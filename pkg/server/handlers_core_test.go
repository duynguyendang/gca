package server

import (
	"net/http"
	"testing"
)

func TestHandleProjects(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns project list", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/projects", "")
		requireStatus(t, w, http.StatusOK)

		var resp []map[string]interface{}
		requireJSON(t, w, &resp)
		if len(resp) == 0 {
			t.Error("expected at least one project")
		}
	})
}

func TestHandleQuery(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid datalog query", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/query?project="+testProjectID, `{"query": "triples(?S, ?P, ?O)"}`)
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("empty query returns empty graph", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/query?project="+testProjectID, `{"query": ""}`)
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/query", `{"query": "triples(?S, ?P, ?O)"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid JSON body returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/query?project="+testProjectID, `{invalid}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("raw mode returns results", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/query?project="+testProjectID+"&raw=true", `{"query": "triples(?S, ?P, ?O)"}`)
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleSource(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/source?id=handlers/user_handler.go", "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing id returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/source?project="+testProjectID, "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("valid request returns 200", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/source?project="+testProjectID+"&id=handlers/user_handler.go", "")
		// May return 200 with content or 404 if file not on disk
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}

func TestHandleSummary(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid project returns summary", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/summary?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/summary", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandlePredicates(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns predicates for project", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/predicates?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["predicates"]; !ok {
			t.Error("expected 'predicates' key in response")
		}
	})

	t.Run("auto-detects project when none specified", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/predicates", "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleSymbols(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("search returns results", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/symbols?project="+testProjectID+"&q=handle", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("empty query returns 200", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/symbols?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/symbols?q=handle", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleFiles(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns file list", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/files?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("with prefix filter", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/files?project="+testProjectID+"&prefix=handlers", "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleHydrate(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid symbol returns hydrated data", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/hydrate?project="+testProjectID+"&id=handleUser", "")
		// May return 200 with hydrated data or 404 if not found
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("missing id returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/hydrate?project="+testProjectID, "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}
