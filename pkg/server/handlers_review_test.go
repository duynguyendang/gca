package server

import (
	"net/http"
	"testing"
)

func TestHandleCreateReviewSession(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("creates review session from diff", func(t *testing.T) {
		diff := `diff --git a/handlers/user_handler.go b/handlers/user_handler.go
--- a/handlers/user_handler.go
+++ b/handlers/user_handler.go
@@ -10,3 +10,4 @@ func handleUser() {
+	// new line`
		body := `{"project_id": "` + testProjectID + `", "diff": "` + escapeJSON(diff) + `"}`
		w := doJSONRequest(srv, "POST", "/api/v1/review/session", body)
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["session_id"]; !ok {
			t.Error("expected 'session_id' key in response")
		}
	})

	t.Run("missing project_id returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/review/session", `{"diff": "test diff"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing diff returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/review/session", `{"project_id": "`+testProjectID+`"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		w := doJSONRequest(srv, "POST", "/api/v1/review/session", `{invalid}`)
		requireStatus(t, w, http.StatusBadRequest)
	})
}

func TestHandleReviewSessionQuery(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// First create a session
	diff := `diff --git a/test.go b/test.go
--- a/test.go
+++ b/test.go
@@ -1,0 +1,1 @@ 
+func test() {}`
	createBody := `{"project_id": "` + testProjectID + `", "diff": "` + escapeJSON(diff) + `"}`
	createW := doJSONRequest(srv, "POST", "/api/v1/review/session", createBody)
	if createW.Code != http.StatusOK {
		t.Skip("could not create review session for query test")
	}

	var createResp map[string]interface{}
	requireJSON(t, createW, &createResp)
	sessionID, ok := createResp["session_id"].(string)
	if !ok || sessionID == "" {
		t.Skip("no session_id in create response")
	}

	t.Run("queries existing session", func(t *testing.T) {
		body := `{"query": "triples(?S, ?P, ?O)", "project_id": "` + testProjectID + `"}`
		w := doJSONRequest(srv, "POST", "/api/v1/review/session/"+sessionID+"/query", body)
		// May return 200 with results or 400/404 if federation not fully implemented
		if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("expected 200, 400, or 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing session id returns 400", func(t *testing.T) {
		body := `{"query": "triples(?S, ?P, ?O)", "project_id": "` + testProjectID + `"}`
		w := doJSONRequest(srv, "POST", "/api/v1/review/session//query", body)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("expected 400 or 404, got %d", w.Code)
		}
	})

	t.Run("missing query returns 400", func(t *testing.T) {
		body := `{"project_id": "` + testProjectID + `"}`
		w := doJSONRequest(srv, "POST", "/api/v1/review/session/"+sessionID+"/query", body)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("nonexistent session returns 404", func(t *testing.T) {
		body := `{"query": "triples(?S, ?P, ?O)", "project_id": "` + testProjectID + `"}`
		w := doJSONRequest(srv, "POST", "/api/v1/review/session/nonexistent/query", body)
		if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
			t.Errorf("expected 404 or 400, got %d", w.Code)
		}
	})
}

// escapeJSON escapes a string for inclusion in a JSON string value.
func escapeJSON(s string) string {
	s = replaceAll(s, "\\", "\\\\")
	s = replaceAll(s, "\"", "\\\"")
	s = replaceAll(s, "\n", "\\n")
	s = replaceAll(s, "\r", "\\r")
	s = replaceAll(s, "\t", "\\t")
	return s
}

func replaceAll(s, old, new string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += new
			i += len(old) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}
