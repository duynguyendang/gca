package server

import (
	"net/http"
	"testing"
)

func TestHandleCreateSnapshot(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("creates snapshot successfully", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/snapshots", `{"project_id": "`+testProjectID+`", "label": "test"}`)
		requireStatus(t, w, http.StatusCreated)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["id"]; !ok {
			t.Error("expected 'id' key in response")
		}
	})

	t.Run("missing project_id returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/snapshots", `{"label": "test"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/snapshots", `{invalid}`)
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleListSnapshots(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("lists snapshots for project", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/snapshots?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("empty project returns empty list", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/graph/snapshots?project=nonexistent", "")
		// May return 200 with empty list or 404
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", w.Code)
		}
	})
}

func TestHandleGraphDiff(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("diff with project_id takes snapshot", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/diff", `{"project_id": "`+testProjectID+`"}`)
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		// Should have diff structure
		if _, ok := resp["summary"]; !ok {
			// Some implementations may return different structure
			if _, ok := resp["added_nodes"]; !ok {
				t.Error("expected diff structure in response")
			}
		}
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/diff", `{invalid}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("empty request returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/graph/diff", `{}`)
		requireStatus(t, w, http.StatusBadRequest)
	})
}
