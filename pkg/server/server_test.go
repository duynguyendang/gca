package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
	"github.com/stretchr/testify/assert"
)

func setupTestStore(t *testing.T) *meb.MEBStore {
	tmpDir, err := os.MkdirTemp("", "meb_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := store.DefaultConfig(tmpDir)
	cfg.InMemory = false // Use disk for simplicity of setup/cleanup or true? Default is fine.
	// Actually, Default might use disk. Let's force in-memory if supported or just tmpDir.
	// MEBStore uses Badger, which needs disk usually unless InMem is set.
	// store.DefaultConfig just sets dir.

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func TestHealthCheck(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()
	srv := NewServer(s, "")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuery(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()
	srv := NewServer(s, "")

	// Insert some dummy data
	fact := meb.NewFact("start_node", "connects_to", "end_node")
	s.AddFact(fact)

	w := httptest.NewRecorder()
	body := `{"query": "triples(S, P, O)"}`
	req, _ := http.NewRequest("POST", "/v1/query", strings.NewReader(body))
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var graph GraphResponse
	json.Unmarshal(w.Body.Bytes(), &graph)

	// Should have at least 2 nodes and 1 link
	assert.GreaterOrEqual(t, len(graph.Nodes), 2)
	assert.GreaterOrEqual(t, len(graph.Links), 1)
}

func TestSource(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	// Create a dummy source file
	tmpSource, err := os.MkdirTemp("", "source_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpSource) })

	fileName := "test.go"
	content := "line1\nline2\nline3\nline4\nline5"
	err = os.WriteFile(filepath.Join(tmpSource, fileName), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Index the file ID in store so LookUpID works
	// We need to inject "test.go" into the dictionary.
	// MEBStore doesn't expose dictionary directly for writing without facts?
	// But AddFact does it.
	s.AddFact(meb.NewFact(fileName, "type", "file"))

	srv := NewServer(s, tmpSource)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/source?id="+fileName+"&start=2&end=4", nil)
	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "line2\nline3\nline4", w.Body.String())
}
