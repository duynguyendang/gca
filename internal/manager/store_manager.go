package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
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
	mu            sync.Mutex // Protects all access to projects cache
	profile       MemoryProfile
	readOnly      bool
	cachedList    []ProjectMetadata
	lastListBuild time.Time
}

// NewStoreManager creates a new StoreManager.
func NewStoreManager(baseDir string, profile MemoryProfile, readOnly bool) *StoreManager {
	// Create LRU cache with eviction callback to close stores
	// Note: All access to this cache must be protected by StoreManager.mu
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if exists in LRU (under lock for thread safety)
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

	// Apply Memory Profile
	if sm.profile == MemoryProfileLow {
		cfg.BlockCacheSize = 64 << 20 // 64 MB
		cfg.IndexCacheSize = 64 << 20 // 64 MB
		cfg.Profile = "Safe-Serving"
	} else {
		cfg.BlockCacheSize = 128 << 20 // 128 MB (Still small)
		cfg.IndexCacheSize = 128 << 20 // 128 MB
		cfg.Profile = "Safe-Serving"
	}

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open store for project %s: %w", projectID, err)
	}

	sm.projects.Add(projectID, s)
	return s, nil
}

// ListProjects returns a list of available projects.
func (sm *StoreManager) ListProjects() ([]ProjectMetadata, error) {
	sm.mu.Lock()
	if time.Since(sm.lastListBuild) < ProjectListTTL && sm.cachedList != nil {
		// Return copy to be safe
		list := make([]ProjectMetadata, len(sm.cachedList))
		copy(list, sm.cachedList)
		sm.mu.Unlock()
		return list, nil
	}
	sm.mu.Unlock()

	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir error on baseDir '%s': %v", sm.baseDir, err)
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

	sm.mu.Lock()
	sm.cachedList = projects
	sm.lastListBuild = time.Now()
	sm.mu.Unlock()

	return projects, nil
}

// CloseAll closes all open stores.
func (sm *StoreManager) CloseAll() {
	sm.projects.Purge()
}
