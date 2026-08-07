package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
)

// newTestIngestServer builds a server whose manager owns its own store (so the
// okf/ingest handler can open the project's store without a directory lock).
func newTestIngestServer(t *testing.T, readOnly bool) *Server {
	t.Helper()
	dataDir := t.TempDir()
	projDir := filepath.Join(dataDir, testProjectID)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "metadata.json"), []byte(`{"name":"Test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, readOnly)
	return NewServerWithAIService(mgr, dataDir, &testMockAIService{})
}

func TestHandleOKFIngest(t *testing.T) {
	srv := newTestIngestServer(t, false)
	defer srv.manager.CloseAll()

	t.Run("rejects when bundle_dir missing", func(t *testing.T) {
		w := doRequest(srv, "POST", "/api/v1/okf/ingest", `{"project_id":"testproj"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("rejects relative bundle_dir", func(t *testing.T) {
		w := doRequest(srv, "POST", "/api/v1/okf/ingest", `{"project_id":"testproj","bundle_dir":"relative/path"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("rejects nonexistent bundle_dir", func(t *testing.T) {
		w := doRequest(srv, "POST", "/api/v1/okf/ingest", `{"project_id":"testproj","bundle_dir":"/nonexistent/bundle"}`)
		requireStatus(t, w, http.StatusBadRequest)
	})

	t.Run("ingests a valid bundle", func(t *testing.T) {
		bundle := t.TempDir()
		conceptsDir := filepath.Join(bundle, "concepts")
		if err := os.MkdirAll(conceptsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(conceptsDir, "orders.md"), []byte("---\ntype: table\ntitle: Orders\ndescription: Orders dataset\n---\n# Orders\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		body := `{"project_id":"testproj","bundle_dir":"` + bundle + `"}`
		w := doRequest(srv, "POST", "/api/v1/okf/ingest", body)
		requireStatus(t, w, http.StatusOK)

		var resp struct {
			Concepts   int  `json:"concepts"`
			Conformant bool `json:"conformant"`
		}
		requireJSON(t, w, &resp)
		if resp.Concepts != 1 {
			t.Errorf("expected 1 concept, got %d", resp.Concepts)
		}
		if !resp.Conformant {
			t.Error("expected conformant=true")
		}
	})
}

func TestHandleOKFIngestReadOnly(t *testing.T) {
	srv := newTestIngestServer(t, true)
	defer srv.manager.CloseAll()

	w := doRequest(srv, "POST", "/api/v1/okf/ingest", `{"project_id":"testproj","bundle_dir":"/tmp"}`)
	requireStatus(t, w, http.StatusConflict)
}