package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/duynguyendang/gca/internal/manager"
)

func TestHandleWhoCallsWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("WhoCalls", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/who-calls?project=genkit-go&symbol=index.ts:main", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// May return 200 or 404 depending on whether symbol exists
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleWhatCallsWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("WhatCalls", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/what-calls?project=genkit-go&symbol=index.ts:main", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleGraphMapWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("GetProjectMap", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/map?project=genkit-go", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleGraphBackboneWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("GetBackbone", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/backbone?project=genkit-go", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleDetectCyclesWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("DetectCycles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/cycles?project=genkit-go", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleCheckReachabilityWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("CheckReachability", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/reachable?project=genkit-go&from=a&to=b", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// Should return 200 regardless of reachability result
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleFlowPathWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("GetFlowPath", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/search/flow?project=genkit-go&from=a&to=b", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
			t.Errorf("expected 200 or 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleGraphClusterWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("GetCluster", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/cluster?project=genkit-go&query=triples(A,B,C)", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleGraphCommunitiesWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("DetectCommunities", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/communities?project=genkit-go", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleFindLCAWithRealData(t *testing.T) {
	dataDir := "/mnt/e/gca-v2/gca/data"
	genkitDir := dataDir + "/genkit-go"

	if _, err := os.Stat(genkitDir); os.IsNotExist(err) {
		t.Skip("genkit-go data not found")
	}

	mgr := manager.NewStoreManager(dataDir, manager.MemoryProfileDefault, false)
	defer mgr.CloseAll()

	s := NewServer(mgr, dataDir)

	t.Run("FindLCA", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/graph/lca?project=genkit-go&a=symbol1&b=symbol2", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})
}
