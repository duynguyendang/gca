package server

import (
	"net/http"
	"testing"
)

func TestHandleHealthSummary(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns health summary for project", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/health/summary?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["overall_score"]; !ok {
			t.Error("expected 'overall_score' key in response")
		}
	})

	t.Run("auto-detects project when none specified", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/health/summary", "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleHealthSummaryV2(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns v2 health summary", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/health/summary/v2?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)
	})

	t.Run("auto-detects project when none specified", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/health/summary/v2", "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleSurpriseAnalysis(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns surprise analysis", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/analysis/surprise?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["edges"]; !ok {
			t.Error("expected 'edges' key in response")
		}
	})

	t.Run("auto-detects project when none specified", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/analysis/surprise", "")
		requireStatus(t, w, http.StatusOK)
	})
}

func TestHandleKnowledgeGaps(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns knowledge gaps", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/analysis/knowledge-gaps?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["isolated_nodes"]; !ok {
			t.Error("expected 'isolated_nodes' key in response")
		}
	})

	t.Run("auto-detects project when none specified", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/analysis/knowledge-gaps", "")
		requireStatus(t, w, http.StatusOK)
	})
}
