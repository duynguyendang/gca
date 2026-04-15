package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/telemetry"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	lru "github.com/hashicorp/golang-lru/v2"
)

// ProjectMetadata represents the project information exposed by the API.
type ProjectMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version,omitempty"`
}

// CurrentSchemaVersion is the current version of the knowledge schema.
// Bump this when breaking changes require re-ingestion.
const CurrentSchemaVersion = "2.0"

// MemoryProfile defines the memory optimization strategy
type MemoryProfile string

const (
	MemoryProfileDefault MemoryProfile = "default"
	MemoryProfileLow     MemoryProfile = "low"
	MaxOpenStores                      = 10
	ProjectListTTL                     = 1 * time.Minute
	DefaultMaxFacts                    = 5_000_000 // 5M facts retention limit
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
	telemetrySink meb.TelemetrySink
}

// NewStoreManager creates a new StoreManager.
func NewStoreManager(baseDir string, profile MemoryProfile, readOnly bool) *StoreManager {
	// Create LRU cache with eviction callback to close stores
	// Note: All access to this cache must be protected by StoreManager.mu
	cache, _ := lru.NewWithEvict[string, *meb.MEBStore](MaxOpenStores, func(key string, value *meb.MEBStore) {
		_ = value.Close()
	})

	return &StoreManager{
		baseDir:       baseDir,
		projects:      cache,
		profile:       profile,
		readOnly:      readOnly,
		telemetrySink: telemetry.NewLoggerSink(),
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

	// Enable auto-GC for long-running server mode
	cfg.EnableAutoGC = !sm.readOnly
	cfg.GCRatio = 0.5
	cfg.Verbose = false

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open store for project %s: %w", projectID, err)
	}

	// Set TopicID for project-scoped queries
	// Uses a hash of the project name to generate a unique 24-bit topic ID
	// This must be set before any query operations to ensure correct data filtering
	topicID := hashToTopicID(projectID)
	s.SetTopicID(topicID)

	// Register telemetry sink
	s.RegisterTelemetrySink(sm.telemetrySink)
	log.Printf("Registered telemetry sink for project %s (topicID=%d)", projectID, topicID)

	// Set retention policy to prevent unbounded growth
	if err := s.SetRetention(DefaultMaxFacts); err != nil {
		return nil, fmt.Errorf("failed to set retention for project %s: %w", projectID, err)
	}

	sm.projects.Add(projectID, s)
	return s, nil
}

// ListProjects returns a list of available projects.
func (sm *StoreManager) ListProjects() ([]ProjectMetadata, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if time.Since(sm.lastListBuild) < ProjectListTTL && sm.cachedList != nil {
		list := make([]ProjectMetadata, len(sm.cachedList))
		copy(list, sm.cachedList)
		return list, nil
	}

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
				Name: id,
			}

			metaPath := filepath.Join(sm.baseDir, id, "metadata.json")
			if data, err := os.ReadFile(metaPath); err == nil {
				var jsonMeta ProjectMetadata
				if err := json.Unmarshal(data, &jsonMeta); err == nil {
					if jsonMeta.Name != "" {
						meta.Name = jsonMeta.Name
					}
					meta.Description = jsonMeta.Description
					meta.Version = jsonMeta.Version
				}
			}
			projects = append(projects, meta)
		}
	}

	sm.cachedList = projects
	sm.lastListBuild = time.Now()

	list := make([]ProjectMetadata, len(projects))
	copy(list, projects)
	return list, nil
}

// CloseAll closes all open stores.
func (sm *StoreManager) CloseAll() {
	sm.projects.Purge()
}

// NeedsMigration checks if a project needs to be re-ingested for schema updates.
// It returns true if the project lacks has_name triples (new requirement for symbol resolution).
func (sm *StoreManager) NeedsMigration(projectID string) (bool, string, error) {
	store, err := sm.GetStore(projectID)
	if err != nil {
		return false, "", err
	}

	return CheckStoreNeedsMigration(store)
}

// CheckStoreNeedsMigration checks if a store lacks has_name triples.
func CheckStoreNeedsMigration(s *meb.MEBStore) (bool, string, error) {
	ctx := context.Background()
	count := 0
	for range s.FindSubjectsByObject(ctx, config.PredicateHasName, "") {
		count++
		if count > 0 {
			break // Found at least one, no migration needed
		}
	}

	if count == 0 {
		return true, "no has_name triples found - re-ingestion required", nil
	}
	return false, "", nil
}

// GetProjectMetadata returns metadata for a project.
func (sm *StoreManager) GetProjectMetadata(projectID string) (*ProjectMetadata, error) {
	metaPath := filepath.Join(sm.baseDir, projectID, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata for %s: %w", projectID, err)
	}

	var meta ProjectMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata for %s: %w", projectID, err)
	}

	return &meta, nil
}

// SetProjectVersion updates the version in metadata.json.
func (sm *StoreManager) SetProjectVersion(projectID, version string) error {
	metaPath := filepath.Join(sm.baseDir, projectID, "metadata.json")

	var meta ProjectMetadata
	if data, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(data, &meta)
	}

	meta.Version = version

	newData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return os.WriteFile(metaPath, newData, 0644)
}

// hashToTopicID generates a deterministic 24-bit topic ID from a project name.
func hashToTopicID(name string) uint32 {
	if name == "" {
		return 1
	}
	var h uint32 = 2166136261 // FNV-1a offset basis
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619 // FNV-1a prime
	}
	return (h & 0xFFFFFF) | 1 // ensure non-zero (0 is reserved)
}
