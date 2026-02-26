package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func TestServer_MultiProject(t *testing.T) {
	// Setup temp directory for projects
	tmpDir, err := os.MkdirTemp("", "gca-server-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create project directories
	projects := []string{"projA", "projB"}
	for _, p := range projects {
		pDir := filepath.Join(tmpDir, p)
		if err := os.Mkdir(pDir, 0755); err != nil {
			t.Fatalf("Failed to create project dir %v", err)
		}
		// Create metadata.json for projA
		if p == "projA" {
			meta := `{"name": "Project A", "description": "Test Project A"}`
			if err := os.WriteFile(filepath.Join(pDir, "metadata.json"), []byte(meta), 0644); err != nil {
				t.Fatalf("Failed to write metadata: %v", err)
			}
			if err := os.WriteFile(filepath.Join(pDir, "metadata.json"), []byte(meta), 0644); err != nil {
				t.Fatalf("Failed to write metadata: %v", err)
			}
		}
		// Initialize DB (Open and Close to create manifest)
		cfg := store.DefaultConfig(pDir)
		db, err := meb.NewMEBStore(cfg)
		if err != nil {
			t.Fatalf("Failed to initialize DB: %v", err)
		}
		db.Close()
	}

	// Initialize Manager
	mgr := manager.NewStoreManager(tmpDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	// Initialize Server
	s := NewServer(mgr, tmpDir, "")

	// Test GET /v1/projects
	t.Run("ListProjects", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/v1/projects", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", w.Code)
		}

		var resp []manager.ProjectMetadata
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if len(resp) != 2 {
			t.Errorf("Expected 2 projects, got %d", len(resp))
		}

		// Verify metadata
		for _, p := range resp {
			if p.ID == "projA" {
				if p.Name != "Project A" {
					t.Errorf("Expected Project A name to be 'Project A', got '%s'", p.Name)
				}
			}
		}
	})

	// Test GET /v1/query (Lazy Loading)
	// Note: The store will be empty, so queries won't find anything, but we check for successful execution vs "Project not found".
	t.Run("Query_LazyLoad", func(t *testing.T) {
		// Valid project
		body := strings.NewReader(`{"query": "triples(?S, ?P, ?O)"}`)
		req, _ := http.NewRequest("POST", "/v1/query?project=projA", body)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// Expect 200 OK (empty result is fine)
		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for valid project, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Invalid project
		body = strings.NewReader(`{"query": "triples(?S, ?P, ?O)"}`)
		req, _ = http.NewRequest("POST", "/v1/query?project=invalid", body)
		w = httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found for invalid project, got %d", w.Code)
		}
	})
}
