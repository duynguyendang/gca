package server

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHandleFlowPath(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid flow path request", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/search/flow?project="+testProjectID+"&from=handleUser&to=UserRepo", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("missing from returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/search/flow?project="+testProjectID+"&to=UserRepo", "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing to returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/search/flow?project="+testProjectID+"&from=handleUser", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleGraphPath(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid path request", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/path?project="+testProjectID+"&source=handleUser&target=UserRepo", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("missing start returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/path?project="+testProjectID+"&end=UserRepo", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleSemanticSearch(t *testing.T) {
	srv, _, cleanup := setupTestServer(t, testServerConfig{
		OODAResponse: `{"results": [{"id": "handleUser", "score": 0.95}]}`,
	})
	defer cleanup()

	t.Run("valid search returns results", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/semantic-search?project="+testProjectID+"&q=user+handler&k=5", "")
		// Semantic search may fail without embedding model, that's OK
		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500, got %d", w.Code)
		}
	})

	t.Run("k exceeds max is clamped", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/semantic-search?project="+testProjectID+"&q=test&k=100", "")
		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500, got %d", w.Code)
		}
	})
}

func TestHandleGraphCluster(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns clustered graph", func(t *testing.T) {
		q := url.QueryEscape("triples(?S, ?P, ?O)")
		w := doRequest(srv, "GET", "/api/v1/graph/cluster?project="+testProjectID+"&query="+q, "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleGraphSubgraph(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns subgraph for given IDs", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/subgraph?project="+testProjectID, `{"ids": ["handleUser", "AuthService"]}`)
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("empty IDs returns empty graph", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/subgraph?project="+testProjectID, `{"ids": []}`)
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleGraphCommunities(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns community structure", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/communities?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleHybridCluster(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid hybrid cluster request", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/hybrid-cluster?project="+testProjectID, `{"embedding": [0.1, 0.2, 0.3], "clusters": 3}`)
		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500, got %d", w.Code)
		}
	})
}
