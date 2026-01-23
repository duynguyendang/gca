package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
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
)

// StoreManager manages multiple MEBStore instances.
type StoreManager struct {
	baseDir  string
	projects map[string]*meb.MEBStore
	mu       sync.RWMutex
	profile  MemoryProfile
}

// NewStoreManager creates a new StoreManager.
func NewStoreManager(baseDir string, profile MemoryProfile) *StoreManager {
	return &StoreManager{
		baseDir:  baseDir,
		projects: make(map[string]*meb.MEBStore),
		profile:  profile,
	}
}

// GetStore retrieves a store by project ID, opening it if necessary.
func (sm *StoreManager) GetStore(projectID string) (*meb.MEBStore, error) {
	sm.mu.RLock()
	s, ok := sm.projects[projectID]
	sm.mu.RUnlock()
	if ok {
		return s, nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check
	if s, ok := sm.projects[projectID]; ok {
		return s, nil
	}

	projectDir := filepath.Join(sm.baseDir, projectID)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	// Open in ReadOnly mode with BypassLockGuard
	// Open in ReadOnly mode with BypassLockGuard
	cfg := store.DefaultConfig(projectDir)
	cfg.ReadOnly = true
	cfg.BypassLockGuard = true

	// Apply Memory Profile
	if sm.profile == MemoryProfileLow {
		cfg.BlockCacheSize = 64 << 20  // 64 MB
		cfg.IndexCacheSize = 128 << 20 // 128 MB
		cfg.Profile = "Cloud-Run-LowMem"
	} else {
		cfg.BlockCacheSize = 256 << 20 // 256 MB
		cfg.IndexCacheSize = 256 << 20 // 256 MB
		cfg.Profile = "Safe-Serving"
	}

	s, err := meb.Open(projectDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open store for project %s: %w", projectID, err)
	}

	sm.projects[projectID] = s
	return s, nil
}

// ListProjects returns a list of available projects.
func (sm *StoreManager) ListProjects() ([]ProjectMetadata, error) {
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
	return projects, nil
}

// CloseAll closes all open stores.
func (sm *StoreManager) CloseAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, s := range sm.projects {
		_ = s.Close()
	}
	sm.projects = make(map[string]*meb.MEBStore)
}
