package server

import (
	"net/http"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns ok status", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/health", "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if resp["status"] != "ok" {
			t.Errorf("expected status 'ok', got %v", resp["status"])
		}
	})
}

func TestReadyCheck(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns ready status", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/ready", "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if resp["status"] != "ready" {
			t.Errorf("expected status 'ready', got %v", resp["status"])
		}
		if _, ok := resp["projects"]; !ok {
			t.Error("expected 'projects' key in response")
		}
	})
}

func TestMetricsHandler(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns metrics", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/metrics", "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if resp["endpoint"] != "/api/metrics" {
			t.Errorf("expected endpoint '/api/metrics', got %v", resp["endpoint"])
		}
	})
}

func TestHandleRoutes(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("returns detected routes", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/routes?project="+testProjectID, "")
		requireStatus(t, w, http.StatusOK)

		var resp map[string]interface{}
		requireJSON(t, w, &resp)
		if _, ok := resp["routes"]; !ok {
			t.Error("expected 'routes' key in response")
		}
	})

	t.Run("auto-detects project when none specified", func(t *testing.T) {
		w := doRequest(srv, "GET", "/api/v1/routes", "")
		requireStatus(t, w, http.StatusOK)
	})
}
