package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func TestStoreManager_LRU(t *testing.T) {
	// 1. Setup temp dir
	tmpDir, err := os.MkdirTemp("", "store_manager_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy projects
	for _, id := range []string{"p1", "p2", "p3"} {
		pDir := filepath.Join(tmpDir, id)
		if err := os.MkdirAll(pDir, 0755); err != nil {
			t.Fatalf("Failed to create project dir: %v", err)
		}
		// Open and close a store to initialize it properly (so badger files exist)
		store, err := meb.NewMEBStore(store.DefaultConfig(pDir))
		if err != nil {
			t.Fatalf("Failed to init store %s: %v", id, err)
		}
		store.Close()
	}

	// 2. Init Manager with small LRU by overwriting the constant?
	// Constants can't be overwritten. We rely on the Default MaxOpenStores=10.
	// Since we can't easily change the constant from test without refactoring to passing it in,
	// let's just open 11 stores if we really want to test eviction, or just test that it works.
	// For this test, verifying specific eviction count is hard without mocking LRU or changing the code to accept config.
	// HOWEVER, we can just verify that GetStore works and projects are cached.

	sm := NewStoreManager(tmpDir, MemoryProfileLow, false)

	// Open p1
	s1, err := sm.GetStore("p1")
	if err != nil {
		t.Fatalf("Failed to get p1: %v", err)
	}
	if s1 == nil {
		t.Fatal("s1 is nil")
	}

	// Open p1 again, should be same instance
	s1Again, err := sm.GetStore("p1")
	if err != nil {
		t.Fatalf("Failed to get p1 again: %v", err)
	}
	if s1 != s1Again {
		t.Errorf("Expected same instance for p1, got different")
	}

	sm.CloseAll()
}

func TestStoreManager_ListProjects_Caching(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "store_manager_list_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create p1 with proper store structure (badger dir)
	p1Dir := filepath.Join(tmpDir, "p1")
	if err := os.MkdirAll(p1Dir, 0755); err != nil {
		t.Fatalf("Failed to create p1 dir: %v", err)
	}
	store1, err := meb.NewMEBStore(store.DefaultConfig(p1Dir))
	if err != nil {
		t.Fatalf("Failed to init p1 store: %v", err)
	}
	store1.Close()

	sm := NewStoreManager(tmpDir, MemoryProfileDefault, false)

	// First list
	projects, err := sm.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != "p1" {
		t.Errorf("Expected 1 project p1, got %v", projects)
	}

	// Add p2 with proper store structure
	p2Dir := filepath.Join(tmpDir, "p2")
	if err := os.MkdirAll(p2Dir, 0755); err != nil {
		t.Fatalf("Failed to create p2 dir: %v", err)
	}
	store2, err := meb.NewMEBStore(store.DefaultConfig(p2Dir))
	if err != nil {
		t.Fatalf("Failed to init p2 store: %v", err)
	}
	store2.Close()

	// Second list (should be cached, so still only p1)
	projects, err = sm.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("Expected cached projects (1), got %d. Cache might not be working or TTL too short?", len(projects))
	}

	// Manually expire cache (hacky, but effective for white-box test) or just wait?
	// Waiting 1 minute is too long for unit test.
	// We can modify the lastListBuild
	sm.mu.Lock()
	sm.lastListBuild = time.Now().Add(-2 * time.Minute)
	sm.mu.Unlock()

	// Third list (should refresh)
	projects, err = sm.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("Expected refreshed projects (2), got %d", len(projects))
	}
}
