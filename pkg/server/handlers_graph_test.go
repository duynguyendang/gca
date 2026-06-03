package server

import (
	"net/http"
	"testing"
)

func TestHandleGraph(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid file returns graph", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph?project="+testProjectID+"&file=handlers/user_handler.go", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph?file=handlers/user_handler.go", "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing file returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph?project="+testProjectID, "")
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("lazy loading mode", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph?project="+testProjectID+"&file=handlers/user_handler.go&lazy=true", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}

func TestHandleGraphPaginated(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid pagination request", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/paginated?project="+testProjectID+"&query=triples(?S, ?P, ?O)&limit=10", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("with cursor", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/paginated?project="+testProjectID+"&query=triples(?S, ?P, ?O)&limit=5&offset=0", "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/paginated?query=triples(?S, ?P, ?O)", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleGraphManifest(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns manifest for project", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/manifest?project="+testProjectID, "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}

func TestHandleGraphMap(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns project map", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/map?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("missing project returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/map", "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleFileDetails(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid file returns details", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/file-details?project="+testProjectID+"&file=handlers/user_handler.go", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("missing file returns 400", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/file-details?project="+testProjectID, "")
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleFileCalls(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("valid file returns call graph", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/file-calls?project="+testProjectID+"&file=handlers/user_handler.go", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})

	t.Run("with depth parameter", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/file-calls?project="+testProjectID+"&file=handlers/user_handler.go&depth=2", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}

func TestHandleGraphBackbone(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns backbone graph", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/backbone?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleFileBackbone(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns file backbone", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/file-backbone?project="+testProjectID+"&file=handlers/user_handler.go", "")
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}
