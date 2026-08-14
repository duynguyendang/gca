package server

import (
	"net/http"
	"testing"

	"github.com/duynguyendang/gca/pkg/service/ai"
)

func TestHandleAIAsk(t *testing.T) {
	t.Run("returns AI response with OODA", func(t *testing.T) {
		t.Setenv("USE_OODA_LOOP", "true")
		srv, _, cleanup := setupTestServer(t, testServerConfig{
			OODAResponse: "mock AI analysis result",
		})
		defer cleanup()

		body := `{"project_id": "` + testProjectID + `", "query": "what are the entry points?"}`
		w := doJSONRequest(srv, "POST", "/api/v1/ai/ask", body)
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["answer"]; !ok {
			t.Error("expected 'answer' key in response")
		}
	})

	t.Run("missing project_id returns 400", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t)
		defer cleanup()

		w := doJSONRequest(srv, "POST", "/api/v1/ai/ask", `{"query": "test"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t)
		defer cleanup()

		w := doJSONRequest(srv, "POST", "/api/v1/ai/ask", `{invalid}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("nil AI service returns 503", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t, testServerConfig{
			NoAIService: true,
		})
		defer cleanup()

		body := `{"project_id": "` + testProjectID + `", "query": "test"}`
		w := doJSONRequest(srv, "POST", "/api/v1/ai/ask", body)
		requireStatus(t, w, http.StatusServiceUnavailable)
	})
}

func TestHandleAIClassify(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("classifies intent", func(t *testing.T) {
		body := `{"project_id": "` + testProjectID + `", "query": "find all circular dependencies"}`
		w := doJSONRequest(srv, "POST", "/api/v1/ai/classify", body)
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["intent"]; !ok {
			t.Error("expected 'intent' key in response")
		}
	})

	t.Run("missing project_id returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/ai/classify", `{"query": "test"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing query returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/ai/classify", `{"project_id": "`+testProjectID+`"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleAsk(t *testing.T) {
	t.Run("returns ask response", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t, testServerConfig{
			AskResponse: &ai.AskResponse{
				Answer:     "The entry points are main.go",
				Query:      "triples(?S, has_kind, function)",
				Intent:     "task_insight",
				Confidence: 0.95,
			},
		})
		defer cleanup()

		body := `{"project_id": "` + testProjectID + `", "query": "what are the entry points?"}`
		w := doJSONRequest(srv, "POST", "/api/v1/ask", body)
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["answer"]; !ok {
			t.Error("expected 'answer' key in response")
		}
	})

	t.Run("missing project_id returns 400", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t)
		defer cleanup()

		w := doJSONRequest(srv, "POST", "/api/v1/ask", `{"query": "test"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing query returns 400", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t)
		defer cleanup()

		w := doJSONRequest(srv, "POST", "/api/v1/ask", `{"project_id": "`+testProjectID+`"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("AI error returns 500", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t, testServerConfig{
			AskError: assertErr("AI service error"),
		})
		defer cleanup()

		body := `{"project_id": "` + testProjectID + `", "query": "test"}`
		w := doJSONRequest(srv, "POST", "/api/v1/ask", body)
		requireStatus(t, w, http.StatusInternalServerError)
	})
}

func TestHandleAgentExecute(t *testing.T) {
	t.Run("nil AI service returns 503", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t, testServerConfig{
			NoAIService: true,
		})
		defer cleanup()

		body := `{"project_id": "` + testProjectID + `", "query": "analyze this codebase"}`
		w := doJSONRequest(srv, "POST", "/api/v1/agent/execute", body)
		requireStatus(t, w, http.StatusServiceUnavailable)
	})

	t.Run("missing project_id returns 400", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t)
		defer cleanup()

		w := doJSONRequest(srv, "POST", "/api/v1/agent/execute", `{"query": "test"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing query returns 400", func(t *testing.T) {
		srv, _, cleanup := setupTestServer(t)
		defer cleanup()

		w := doJSONRequest(srv, "POST", "/api/v1/agent/execute", `{"project_id": "`+testProjectID+`"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})
}

// assertErr is a simple error type for testing.
type assertErr string

func (e assertErr) Error() string { return string(e) }
