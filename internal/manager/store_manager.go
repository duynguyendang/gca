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
	"github.com/duynguyendang/gca/pkg/ephemeral"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/telemetry"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	lru "github.com/hashicorp/golang-lru/v2"
)

// Session is an alias for ephemeral.Session for convenience.
type Session = ephemeral.Session

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

// StoreType specifies which logical store to access (Source vs Analytical).
type StoreType string

const (
	// StoreTypeSource is the primary store for immutable AST facts.
	// TopicID: GlobalTopicID() — high bit clear.
	StoreTypeSource StoreType = "SOURCE"

	// StoreTypeAnalytical is the secondary store for derived insights.
	// TopicID: AnalyticalTopicID() — high bit set.
	StoreTypeAnalytical StoreType = "ANALYTICAL"

	// StoreTypeFederated executes queries across both stores.
	StoreTypeFederated StoreType = "FEDERATED"
)

// TopicID constants for store partitioning.
const (
	topicIDMask           = 0xFFFFFF // 24-bit mask
	topicIDHighBit        = 0x800000 // high bit set = Analytical partition
	topicIDAnalyticalBit  = 0x800000 // alias for clarity
	topicIDGlobalMask     = 0x7FFFFF // high bit clear = Source/Global partition
)

// Memory constants.
const (
	MemoryProfileDefault MemoryProfile = "default"
	MemoryProfileLow     MemoryProfile = "low"
	MaxOpenStores                     = 10
	ProjectListTTL                    = 1 * time.Minute
	DefaultMaxFacts                   = 5_000_000 // 5M facts retention limit
	WindowMaxFacts                    = 500_000   // 500K facts window limit
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
	ephemeral     *ephemeral.EphemeralStore // RAM-only sessions for PR diffs/incidents
}

// NewStoreManager creates a new StoreManager.
func NewStoreManager(baseDir string, profile MemoryProfile, readOnly bool) *StoreManager {
	sm := &StoreManager{
		baseDir:       baseDir,
		profile:       profile,
		readOnly:      readOnly,
		telemetrySink: telemetry.NewLoggerSink(),
		ephemeral:     ephemeral.NewEphemeralStore(0), // default 30-min TTL
	}

	// Create LRU cache with eviction callback to close stores
	// The eviction callback closes stores when they're evicted from the LRU.
	// Since the cache is only accessed via GetStore() which holds sm.mu,
	// and eviction happens when the cache is at capacity, the callback
	// running means no active operation is in progress on that specific store.
	cache, _ := lru.NewWithEvict[string, *meb.MEBStore](MaxOpenStores, func(key string, value *meb.MEBStore) {
		sm.mu.Lock()
		defer sm.mu.Unlock()
		_ = value.Close()
	})
	sm.projects = cache

	return sm
}

// SetEphemeralStore sets the EphemeralStore (for testing or custom configuration).
func (sm *StoreManager) SetEphemeralStore(es *ephemeral.EphemeralStore) {
	sm.ephemeral = es
}

// NewEphemeralSession creates a new RAM-based session for transient facts.
func (sm *StoreManager) NewEphemeralSession(projectID string) (*Session, error) {
	return sm.ephemeral.NewSession(projectID)
}

// GetEphemeralSession retrieves an ephemeral session by ID.
func (sm *StoreManager) GetEphemeralSession(id string) (*Session, error) {
	return sm.ephemeral.GetSession(id)
}

// GetStore retrieves a store by project ID, opening it if necessary.
func (sm *StoreManager) GetStore(projectID string) (*meb.MEBStore, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if exists in LRU (under lock for thread safety)
	if s, ok := sm.projects.Get(projectID); ok {
		// Reset to GlobalTopicID to ensure correct partition for source queries
		s.SetTopicID(GlobalTopicID(projectID))
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
	// Uses GlobalTopicID (high bit clear) for default partition access
	// This must be set before any query operations to ensure correct data filtering
	topicID := GlobalTopicID(projectID)
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
	if sm.ephemeral != nil {
		sm.ephemeral.Close()
	}
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

// GlobalTopicID returns the Attention Sink topic ID for a project (permanent storage).
// Uses the base hash with high bit clear for Global partition.
func GlobalTopicID(projectID string) uint32 {
	return hashToTopicID(projectID) & 0x7FFFFF // clear high bit
}

// WindowTopicID returns the Sliding Window topic ID for a project (temporary storage).
// Uses the base hash with high bit set for Window partition.
func WindowTopicID(projectID string) uint32 {
	return hashToTopicID(projectID) | 0x800000 // set high bit
}

// AnalyticalTopicID returns the topic ID for the Analytical Store partition.
// Alias for WindowTopicID — used for derived/insight facts.
func AnalyticalTopicID(projectID string) uint32 {
	return WindowTopicID(projectID)
}

// GetSourceStore returns the store scoped to the Source Store partition (GlobalTopicID).
// The Source Store holds immutable AST facts from Tree-sitter parsing.
// Facts are written once at ingest time; no AI-generated content.
func (sm *StoreManager) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	s, err := sm.GetStore(projectID)
	if err != nil {
		return nil, err
	}
	s.SetTopicID(GlobalTopicID(projectID))
	return s, nil
}

// GetAnalyticalStore returns the store scoped to the Analytical Store partition (AnalyticalTopicID).
// The Analytical Store holds derived insights: KPIs, smells, query templates, diagnostic reports.
// Facts are written asynchronously after Source Store ingest completes.
func (sm *StoreManager) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	s, err := sm.GetStore(projectID)
	if err != nil {
		return nil, err
	}
	s.SetTopicID(AnalyticalTopicID(projectID))
	return s, nil
}

// StorePair holds both store references for federated queries.
type StorePair struct {
	Source      *meb.MEBStore
	Analytical  *meb.MEBStore
}

// GetBothStores returns both stores for federated cross-store queries.
// Caller must close both stores when done.
func (sm *StoreManager) GetBothStores(projectID string) (*StorePair, error) {
	source, err := sm.GetSourceStore(projectID)
	if err != nil {
		return nil, err
	}
	analytical, err := sm.GetAnalyticalStore(projectID)
	if err != nil {
		return nil, err
	}
	return &StorePair{Source: source, Analytical: analytical}, nil
}

// QueryResult holds query results with metadata.
type QueryResult struct {
	Rows      []map[string]string
	Count     int
	StoreType StoreType
}

// QuerySource executes a Datalog query against the Source Store only.
func (sm *StoreManager) QuerySource(ctx context.Context, projectID, query string) (*QueryResult, error) {
	s, err := sm.GetSourceStore(projectID)
	if err != nil {
		return nil, err
	}
	results, err := mebpkg.Query(ctx, s, query)
	if err != nil {
		return nil, err
	}
	return &QueryResult{
		Rows:      rowsToStringMap(results),
		Count:     len(results),
		StoreType: StoreTypeSource,
	}, nil
}

// QueryAnalytical executes a Datalog query against the Analytical Store only.
func (sm *StoreManager) QueryAnalytical(ctx context.Context, projectID, query string) (*QueryResult, error) {
	s, err := sm.GetAnalyticalStore(projectID)
	if err != nil {
		return nil, err
	}
	results, err := mebpkg.Query(ctx, s, query)
	if err != nil {
		return nil, err
	}
	return &QueryResult{
		Rows:      rowsToStringMap(results),
		Count:     len(results),
		StoreType: StoreTypeAnalytical,
	}, nil
}

// QueryFederated executes a Datalog query across both stores, merging results.
// Duplicates are not filtered — caller should dedupe if needed.
func (sm *StoreManager) QueryFederated(ctx context.Context, projectID, query string) (*QueryResult, error) {
	pair, err := sm.GetBothStores(projectID)
	if err != nil {
		return nil, err
	}

	sourceResults, err := mebpkg.Query(ctx, pair.Source, query)
	if err != nil {
		return nil, fmt.Errorf("source query failed: %w", err)
	}

	analyticalResults, err := mebpkg.Query(ctx, pair.Analytical, query)
	if err != nil {
		return nil, fmt.Errorf("analytical query failed: %w", err)
	}

	// Merge results
	merged := append(rowsToStringMap(sourceResults), rowsToStringMap(analyticalResults)...)
	return &QueryResult{
		Rows:      merged,
		Count:     len(merged),
		StoreType: StoreTypeFederated,
	}, nil
}

// rowsToStringMap converts []map[string]any to []map[string]string for API responses.
func rowsToStringMap(rows []map[string]any) []map[string]string {
	result := make([]map[string]string, len(rows))
	for i, row := range rows {
		strRow := make(map[string]string)
		for k, v := range row {
			if v != nil {
				strRow[k] = fmt.Sprintf("%v", v)
			}
		}
		result[i] = strRow
	}
	return result
}
