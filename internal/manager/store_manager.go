package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ephemeral"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/telemetry"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	"github.com/duynguyendang/meb/vector"
	"github.com/dgraph-io/badger/v4"
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

// Index types for vector search acceleration.
const (
	IndexTypeBrute = "brute" // default: exact brute-force search
	IndexTypeIVFPQ = "ivfpq" // IVF-PQ approximate nearest neighbor
	IndexTypeHNSW  = "hnsw"  // HNSW graph-based approximate nearest neighbor
)

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
	topicIDMask          = 0xFFFFFF // 24-bit mask
	topicIDHighBit       = 0x800000 // high bit set = Analytical partition
	topicIDAnalyticalBit = 0x800000 // alias for clarity
	topicIDGlobalMask    = 0x7FFFFF // high bit clear = Source/Global partition
)

// Memory constants.
const (
	MemoryProfileDefault MemoryProfile = "default"
	MemoryProfileLow     MemoryProfile = "low"
	MaxOpenStores                      = 10
	ProjectListTTL                     = 1 * time.Minute
	DefaultMaxFacts                    = 5_000_000 // 5M facts retention limit
	WindowMaxFacts                     = 500_000   // 500K facts window limit
)

// StoreManager manages multiple MEBStore instances.
type StoreManager struct {
	baseDir       string
	projects      *lru.Cache[string, *meb.MEBStore]
	mu            sync.Mutex // Protects all access to projects cache
	profile       MemoryProfile
	readOnly      bool
	embeddingDim  int    // Vector dimension for MEB's vector registry (0 = MEB default 1536)
	indexType     string // Vector index type: brute, ivfpq, hnsw
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

// SetIndexType sets the vector search index type. Must be called before
// the first GetStore() call. Accepted values: "brute" (default), "ivfpq", "hnsw".
func (sm *StoreManager) SetIndexType(indexType string) {
	switch indexType {
	case "", IndexTypeBrute:
		sm.indexType = IndexTypeBrute
	case IndexTypeIVFPQ, IndexTypeHNSW:
		sm.indexType = indexType
	default:
		log.Printf("WARNING: unknown MEB_INDEX %q, falling back to brute", indexType)
		sm.indexType = IndexTypeBrute
	}
}

// SetEphemeralStore sets the EphemeralStore (for testing or custom configuration).
func (sm *StoreManager) SetEphemeralStore(es *ephemeral.EphemeralStore) {
	sm.ephemeral = es
}

// SetVectorFullDim sets the embedding vector dimension for MEB's vector registry.
// This must match the output dimension of the configured embedding model.
// Setting to 0 leaves MEB's default (1536). Call before first GetStore() call.
func (sm *StoreManager) SetVectorFullDim(dim int) {
	sm.embeddingDim = dim
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

	projectDir := getActualProjectDir(sm.baseDir, projectID)

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	// Open in ReadOnly mode if configured
	cfg := store.DefaultConfig(projectDir)
	cfg.ReadOnly = sm.readOnly
	cfg.SyncWrites = true

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

	// Set vector dimension to match embedding model output.
	// Must be set before NewMEBStore — mismatches cause all vector Adds to fail.
	if sm.embeddingDim > 0 {
		cfg.VectorFullDim = sm.embeddingDim
	}

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		// Handle meb v0.4 snapshot format migration: if the snapshot vector
		// size doesn't match (clean break from v0.3), delete the old snapshot
		// keys and retry, so a fresh snapshot is built from current vectors.
		if strings.Contains(err.Error(), "snapshot vector size mismatch") {
			log.Printf("Detected stale vector snapshot for project %s, clearing and retrying...", projectID)
			if clearErr := clearVectorSnapshot(projectDir); clearErr != nil {
				return nil, fmt.Errorf("failed to open store for project %s: %w (also failed to clear snapshot: %v)", projectID, err, clearErr)
			}
			s, err = meb.NewMEBStore(cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to open store for project %s after snapshot clear: %w", projectID, err)
			}
			log.Printf("Vector snapshot cleared and store reopened successfully for project %s", projectID)
		} else {
			return nil, fmt.Errorf("failed to open store for project %s: %w", projectID, err)
		}
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

	// Enable vector index if configured
	switch sm.indexType {
	case IndexTypeIVFPQ:
		if err := s.EnableIVFPQ(vector.DefaultIVFPQConfig()); err != nil {
			return nil, fmt.Errorf("failed to enable IVF-PQ for project %s: %w", projectID, err)
		}
		log.Printf("IVF-PQ index enabled for project %s", projectID)
	case IndexTypeHNSW:
		if err := s.EnableHNSW(vector.DefaultHNSWConfig()); err != nil {
			return nil, fmt.Errorf("failed to enable HNSW for project %s: %w", projectID, err)
		}
		log.Printf("HNSW index enabled for project %s", projectID)
	default:
		// brute (exact search) — no additional setup needed
	}

	sm.projects.Add(projectID, s)
	return s, nil
}

// TrainIndex trains the vector index (IVF-PQ only; HNSW trains incrementally).
// Called after ingestion or batch vector additions.
func (sm *StoreManager) TrainIndex(s *meb.MEBStore, topicID uint32) error {
	switch sm.indexType {
	case IndexTypeIVFPQ:
		if err := s.TrainIVFPQ(topicID); err != nil {
			return fmt.Errorf("IVF-PQ training failed: %w", err)
		}
		log.Printf("IVF-PQ training completed for topic %d", topicID)
	}
	return nil
}

// hasBadgerDir checks if a directory contains a badger database subdirectory.
func hasBadgerDir(dir string) bool {
	badgerPath := filepath.Join(dir, "badger")
	info, err := os.Stat(badgerPath)
	return err == nil && info.IsDir()
}

// findNestedBadgerDir looks for a nested directory containing a badger database.
// This handles the case where project name equals source folder name, creating
// a structure like: baseDir/projectName/projectName/badger
func findNestedBadgerDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			nestedPath := filepath.Join(dir, entry.Name(), "badger")
			if info, err := os.Stat(nestedPath); err == nil && info.IsDir() {
				return filepath.Join(dir, entry.Name())
			}
		}
	}
	return ""
}

// getActualProjectDir resolves the actual project directory, handling nested cases.
func getActualProjectDir(baseDir, projectID string) string {
	projectDir := filepath.Join(baseDir, projectID)

	// Check if this project directory itself has badger (normal case)
	if hasBadgerDir(projectDir) {
		// Check if there's a nested directory with the same name (strong indicator of nested structure)
		if isNestedProjectDir(projectDir) {
			nestedPath := filepath.Join(projectDir, projectID)
			if hasBadgerDir(nestedPath) {
				return nestedPath
			}
		}
		return projectDir
	}

	// No badger at projectDir level, look for nested
	nested := findNestedBadgerDir(projectDir)
	if nested != "" {
		return nested
	}

	// Fallback to original projectDir even without badger check
	// This allows error messages to report the correct path
	return projectDir
}

// isNestedProjectDir checks if a project directory contains a subdirectory with the same name.
// This happens when: ./gca ingest /tmp/smell-test ./data/smell-test
// resulting in ./data/smell-test/smell-test/ structure where the project dir name equals subdir name.
func isNestedProjectDir(projectDir string) bool {
	parentName := filepath.Base(projectDir)
	nestedPath := filepath.Join(projectDir, parentName)
	info, err := os.Stat(nestedPath)
	return err == nil && info.IsDir()
}

// isRetryableError checks if an error indicates a transient failure that may succeed on retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	retryableErrors := []string{
		"database is locked",
		"checkpoint",
		"read-only transaction",
		"resource temporarily unavailable",
		"connection refused",
		"timeout",
	}
	for _, pattern := range retryableErrors {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// GetStoreWithRetry retrieves a store with exponential backoff retry on transient failures
func (sm *StoreManager) GetStoreWithRetry(projectID string, maxRetries int) (*meb.MEBStore, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		store, err := sm.GetStore(projectID)
		if err == nil {
			return store, nil
		}
		lastErr = err

		if !isRetryableError(err) {
			return nil, err
		}

		if attempt < maxRetries {
			backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
			if backoff > 2*time.Second {
				backoff = 2 * time.Second
			}
			log.Printf("Retryable error for project %s (attempt %d/%d): %v, retrying in %v",
				projectID, attempt+1, maxRetries+1, err, backoff)
			time.Sleep(backoff)
		}
	}
	return nil, lastErr
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

			// Skip directories that are just nested project structures
			// (when project name equals folder name, like smell-test/smell-test)
			actualDir := getActualProjectDir(sm.baseDir, id)
			if !hasBadgerDir(actualDir) {
				continue
			}

			meta := ProjectMetadata{
				ID:   id,
				Name: id,
			}

			metaPath := filepath.Join(actualDir, "metadata.json")
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
	h := common.FNV1aHash(name)
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
	Source     *meb.MEBStore
	Analytical *meb.MEBStore
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

// clearVectorSnapshot deletes the stale TQ vector snapshot keys from a
// Badger DB, so that meb can rebuild a fresh snapshot on next open.
// This is needed after a meb v0.3→v0.4 upgrade where the snapshot format
// changed (clean break).
func clearVectorSnapshot(projectDir string) error {
	badgerPath := filepath.Join(projectDir, "badger")
	opts := badger.DefaultOptions(badgerPath).
		WithReadOnly(false).
		WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		return fmt.Errorf("open badger for snapshot cleanup: %w", err)
	}
	defer db.Close()

	// Collect all keys with "sys:tq:" prefix (snapshot meta + vector chunks)
	var keys [][]byte
	err = db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("sys:tq:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := make([]byte, len(it.Item().Key()))
			copy(key, it.Item().Key())
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan snapshot keys: %w", err)
	}

	if len(keys) == 0 {
		return nil // nothing to clear
	}

	err = db.Update(func(txn *badger.Txn) error {
		for _, k := range keys {
			if err := txn.Delete(k); err != nil {
				return fmt.Errorf("delete snapshot key %s: %w", string(k), err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete snapshot keys: %w", err)
	}

	return nil
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
