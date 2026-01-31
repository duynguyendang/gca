package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
	lru "github.com/hashicorp/golang-lru/v2"
)

// ProjectMetadata represents the project information exposed by the API.
type ProjectMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MemoryProfile defines the memory optimization strategy
type MemoryProfile string

const (
	MemoryProfileDefault MemoryProfile = "default"
	MemoryProfileLow     MemoryProfile = "low"
	MaxOpenStores                      = 10
	ProjectListTTL                     = 1 * time.Minute
)

// StoreManager manages multiple MEBStore instances.
type StoreManager struct {
	baseDir       string
	projects      *lru.Cache[string, *meb.MEBStore]
	mu            sync.RWMutex
	profile       MemoryProfile
	readOnly      bool
	cachedList    []ProjectMetadata
	lastListBuild time.Time
}

// NewStoreManager creates a new StoreManager.
func NewStoreManager(baseDir string, profile MemoryProfile, readOnly bool) *StoreManager {
	// Create LRU cache with eviction callback to close stores
	cache, _ := lru.NewWithEvict[string, *meb.MEBStore](MaxOpenStores, func(key string, value *meb.MEBStore) {
		_ = value.Close()
	})

	return &StoreManager{
		baseDir:  baseDir,
		projects: cache,
		profile:  profile,
		readOnly: readOnly,
	}
}

// GetStore retrieves a store by project ID, opening it if necessary.
func (sm *StoreManager) GetStore(projectID string) (*meb.MEBStore, error) {
	// Fast path: check if exists in LRU
	// lru.Get updates recency
	if s, ok := sm.projects.Get(projectID); ok {
		return s, nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check under lock
	if s, ok := sm.projects.Get(projectID); ok {
		return s, nil
	}

	projectDir := filepath.Join(sm.baseDir, projectID)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	// Open in ReadOnly mode if configured
	cfg := store.DefaultConfig(projectDir)
	cfg.ReadOnly = sm.readOnly
	// If ReadOnly, we don't need BypassLockGuard typically, but for safety in server mode:
	// cfg.BypassLockGuard = sm.readOnly
	// Actually, BypassLockGuard is for multiple processes.
	// If we are strictly ReadOnly, we might fail if a lock exists (e.g. ingestion running).
	// Let's keep BypassLockGuard true if we want to read even if locked?
	// No, standard ReadOnly usually respects locks or bypasses.
	// Let's rely on standard config, but set ReadOnly.
	if !sm.readOnly {
		cfg.ReadOnly = false
		cfg.BypassLockGuard = true
	} else {
		// In ReadOnly mode, we often want to bypass the lock to inspect running DBs
		cfg.BypassLockGuard = true
	}

	// Apply Memory Profile
	if sm.profile == MemoryProfileLow {
		cfg.BlockCacheSize = 64 << 20 // 64 MB
		cfg.IndexCacheSize = 64 << 20 // 64 MB
		cfg.Profile = "Cloud-Run-LowMem"
	} else {
		cfg.BlockCacheSize = 128 << 20 // 128 MB (Still small)
		cfg.IndexCacheSize = 128 << 20 // 128 MB
		cfg.Profile = "Cloud-Run-LowMem"
	}

	s, err := meb.Open(projectDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open store for project %s: %w", projectID, err)
	}

	sm.projects.Add(projectID, s)
	return s, nil
}

// ListProjects returns a list of available projects.
func (sm *StoreManager) ListProjects() ([]ProjectMetadata, error) {
	sm.mu.RLock()
	if time.Since(sm.lastListBuild) < ProjectListTTL && sm.cachedList != nil {
		// Return copy to be safe
		list := make([]ProjectMetadata, len(sm.cachedList))
		copy(list, sm.cachedList)
		sm.mu.RUnlock()
		return list, nil
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check
	if time.Since(sm.lastListBuild) < ProjectListTTL && sm.cachedList != nil {
		list := make([]ProjectMetadata, len(sm.cachedList))
		copy(list, sm.cachedList)
		return list, nil
	}

	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		return nil, err
	}

	var projects []ProjectMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			id := entry.Name()
			meta := ProjectMetadata{
				ID:   id,
				Name: id, // Default name is directory name
			}

			// Try to read metadata.json
			metaPath := filepath.Join(sm.baseDir, id, "metadata.json")
			if data, err := os.ReadFile(metaPath); err == nil {
				var jsonMeta ProjectMetadata
				if err := json.Unmarshal(data, &jsonMeta); err == nil {
					if jsonMeta.Name != "" {
						meta.Name = jsonMeta.Name
					}
					meta.Description = jsonMeta.Description
				}
			}
			projects = append(projects, meta)
		}
	}

	sm.cachedList = projects
	sm.lastListBuild = time.Now()

	return projects, nil
}

// CloseAll closes all open stores.
func (sm *StoreManager) CloseAll() {
	sm.projects.Purge()
}
