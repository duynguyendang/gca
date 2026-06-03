package server

import (
	"net/http"
	"testing"
)

func TestHandleWhoCalls(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid symbol returns callers", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/who-calls?project="+testProjectID+"&symbol=handleUser", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("with depth parameter", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/who-calls?project="+testProjectID+"&symbol=handleUser&depth=3", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing symbol returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/who-calls?project="+testProjectID, "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/who-calls?symbol=handleUser", "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("nonexistent symbol returns empty or 404", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/who-calls?project="+testProjectID+"&symbol=nonexistent", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}

func TestHandleWhatCalls(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid symbol returns callees", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/what-calls?project="+testProjectID+"&symbol=handleUser", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("with depth parameter", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/what-calls?project="+testProjectID+"&symbol=handleUser&depth=2", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing symbol returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/what-calls?project="+testProjectID, "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleCheckReachability(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("reachable symbols", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/reachable?project="+testProjectID+"&from=handleUser&to=AuthService", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing from returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/reachable?project="+testProjectID+"&to=AuthService", "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing to returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/reachable?project="+testProjectID+"&from=handleUser", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleDetectCycles(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns cycle detection results", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/cycles?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleFindLCA(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("finds LCA of two symbols", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/lca?project="+testProjectID+"&a=handleUser&b=handleOrder", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("missing a returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/lca?project="+testProjectID+"&b=handleOrder", "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing b returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/lca?project="+testProjectID+"&a=handleUser", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleEnrichCalledBy(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("enriches called_by predicates", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/enrich-called-by?project="+testProjectID, "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}
