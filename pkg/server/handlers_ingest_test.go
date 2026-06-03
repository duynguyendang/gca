package server

import (
	"net/http"
	"testing"
)

func TestHandleIncrementalIngest(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/ingest/incremental", `{invalid}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing project_id returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/ingest/incremental", `{"source_dir": "/tmp/test"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing source_dir returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/ingest/incremental", `{"project_id": "`+testProjectID+`"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleTestGenerateEndpoints(t *testing.T) {
	srv, _, cleanup := setupTestServer(t, testServerConfig{
		OODAResponse: "generated test code here",
	})
	defer cleanup()

	t.Run("test/generate with valid request", func(t *testing.T) {
		body := `{"target": "handleUser", "query": "generate tests", "depth": 3}`
		w := doJSONRequest(srv, "POST", "/api/v1/projects/"+testProjectID+"/test/generate", body)
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["answer"]; !ok {
			t.Error("expected 'answer' key in response")
		}
	})

	t.Run("test/generate with invalid JSON returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/projects/"+testProjectID+"/test/generate", `{invalid}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("test/generate-all with valid request", func(t *testing.T) {
		body := `{"depth": 3}`
		w := doJSONRequest(srv, "POST", "/api/v1/projects/"+testProjectID+"/test/generate-all", body)
		requireStatus(t, w, http.StatusOK)

		var resp TestGenerateAllResponse
		requireJSON(t, w, &resp)
		if resp.Total < 0 {
			t.Error("expected non-negative total")
		}
	})
}
