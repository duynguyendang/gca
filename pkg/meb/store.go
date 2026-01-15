// Package meb implements a high-performance, memory-efficient bidirectional graph store
// using BadgerDB and dictionary encoding. It supports quad-based facts (Subject-Predicate-Object-Graph)
// with dual indexing (SPO and OPS) for efficient bidirectional traversal.
//
// Features:
//   - Dictionary encoding for memory efficiency
//   - Dual indices (SPO/OPS) for bidirectional graph traversal
//   - Atomic operations with transaction pooling
//   - Batched operations for high throughput
//   - Vector search integration
//   - Multi-tenancy via graph contexts
//
// Example usage:
//
//	cfg := &store.Config{DataDir: "./data", DictDir: "./dict"}
//	s, err := meb.NewMEBStore(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer s.Close()
//
//	// Add facts
//	fact := meb.NewFact("Alice", "knows", "Bob")
//	s.AddFact(fact)
//
//	// Query facts
//	for f, err := range s.Scan("Alice", "", "", "") {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    fmt.Printf("%s\n", f.String())
//	}
package meb

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/duynguyendang/gca/pkg/meb/dict"
	"github.com/duynguyendang/gca/pkg/meb/keys"
	"github.com/duynguyendang/gca/pkg/meb/predicates"
	"github.com/duynguyendang/gca/pkg/meb/store"
	"github.com/duynguyendang/gca/pkg/meb/vector"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/mangle/ast"
)

// MEBStore implements both factstore.FactStore and store.KnowledgeStore interfaces.
// It uses BadgerDB for persistent storage and dictionary encoding for efficient operations.
type MEBStore struct {
	db     *badger.DB
	dictDB *badger.DB // Separate DB for dictionary
	dict   dict.Dictionary

	// Predicate tables
	predicates map[ast.PredicateSym]*predicates.PredicateTable

	// Configuration
	config *store.Config

	// Mutex for predicate table registration
	mu *sync.RWMutex

	// numFacts tracks the total number of facts in RAM.
	// We use atomic.Uint64 for lock-free thread safety.
	// This value is persisted to disk only on graceful shutdown.
	numFacts atomic.Uint64

	// Vector registry for MRL vector search
	vectors *vector.VectorRegistry

	// Active transaction for batched operations (nil if not in batch)
	txn *badger.Txn
	// Flag to indicate if txn is owned by this instance (true) or inherited (false)
	// Actually, if txn is set in struct, it's borrowed.
	// But we need to know if we should commit/discard.
	// Simpler: if txn != nil, we are inside ExecuteBatch, so we use it and DO NOT commit/discard.
}

// loadStats reads the counter from disk into RAM.
func (m *MEBStore) loadStats() error {
	return m.withReadTxn(func(txn *badger.Txn) error {
		item, err := txn.Get(keys.KeyFactCount)
		if err == badger.ErrKeyNotFound {
			m.numFacts.Store(0)
			return nil
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			if len(val) >= 8 {
				count := binary.BigEndian.Uint64(val)
				m.numFacts.Store(count)
			}
			return nil
		})
	})
}

// saveStats writes the RAM counter to disk.
func (m *MEBStore) saveStats() error {
	if m.config.ReadOnly {
		return nil
	}
	return m.withWriteTxn(func(txn *badger.Txn) error {
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, m.numFacts.Load())
		return txn.Set(keys.KeyFactCount, buf)
	})
}

// NewMEBStore creates a new MEBStore with the given configuration.
func NewMEBStore(cfg *store.Config) (*MEBStore, error) {
	slog.Info("initializing MEB store",
		"dataDir", cfg.DataDir,
		"inMemory", cfg.InMemory,
		"blockCacheSize", cfg.BlockCacheSize,
		"indexCacheSize", cfg.IndexCacheSize,
		"numDictShards", cfg.NumDictShards,
	)

	// Validate configuration before proceeding
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Open BadgerDB for Facts
	db, err := store.OpenBadgerDB(cfg)
	if err != nil {
		slog.Error("failed to open BadgerDB", "error", err)
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	slog.Info("BadgerDB (Facts) opened successfully")

	// Open BadgerDB for Dictionary
	// Create a modified config for Dictionary DB
	dictCfg := *cfg // Copy config
	dictCfg.DataDir = cfg.DictDir
	dictCfg.SyncWrites = true // Enforce strict persistence for Dictionary

	dictDB, err := store.OpenBadgerDB(&dictCfg)
	if err != nil {
		db.Close()
		slog.Error("failed to open Dictionary BadgerDB", "error", err)
		return nil, fmt.Errorf("failed to open Dictionary BadgerDB: %w", err)
	}
	slog.Info("BadgerDB (Dictionary) opened successfully")

	// Create dictionary encoder (sharded if configured)
	var dictEncoder dict.Dictionary
	if cfg.NumDictShards > 0 {
		slog.Info("creating sharded dictionary encoder", "shards", cfg.NumDictShards, "lruCacheSize", cfg.LRUCacheSize)
		dictEncoder, err = dict.NewShardedEncoder(dictDB, cfg.LRUCacheSize, cfg.NumDictShards)
		if err != nil {
			dictDB.Close()
			db.Close()
			return nil, fmt.Errorf("failed to create sharded dictionary encoder: %w", err)
		}
	} else {
		slog.Info("creating single-threaded dictionary encoder", "lruCacheSize", cfg.LRUCacheSize)
		dictEncoder, err = dict.NewEncoder(dictDB, cfg.LRUCacheSize)
		if err != nil {
			dictDB.Close()
			db.Close()
			return nil, fmt.Errorf("failed to create dictionary encoder: %w", err)
		}
	}

	m := &MEBStore{
		db:         db,
		dictDB:     dictDB,
		dict:       dictEncoder,
		predicates: make(map[ast.PredicateSym]*predicates.PredicateTable),
		config:     cfg,
		mu:         &sync.RWMutex{},
		vectors:    vector.NewRegistry(db),
	}

	// Load vector snapshot from disk
	if err := m.vectors.LoadSnapshot(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load vector snapshot: %w", err)
	}

	// Load fact count stats from disk
	if err := m.loadStats(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load stats: %w", err)
	}

	// Register default predicates (triples)
	m.registerDefaultPredicates()

	slog.Info("MEB store initialized successfully", "factCount", m.numFacts.Load())
	return m, nil
}

// registerDefaultPredicates registers the built-in predicates.
func (m *MEBStore) registerDefaultPredicates() {
	// Register "triples" predicate for subject-predicate-object relationships
	triplesPred := ast.PredicateSym{Symbol: "triples", Arity: 3}
	m.predicates[triplesPred] = predicates.NewPredicateTable(m.db, m.dict, triplesPred, keys.SPOPrefix)
}

// SetMetadata writes a metadata pair to the store.
func (m *MEBStore) SetMetadata(key, value string) error {
	return m.withWriteTxn(func(txn *badger.Txn) error {
		// Use a specific prefix for metadata, e.g., "meta:"
		// We can use a simple prefix convention in the key itself or a separate keyspace.
		// For simplicity/compatibility, let's just prefix the key string.
		// But wait, are we using existing keys package?
		// keys package handles binary keys. Metadata usually implies string keys in a separate namespace.
		// Let's assume we store them as raw keys with a prefix "meta:".
		fullKey := []byte("meta:" + key)
		return txn.Set(fullKey, []byte(value))
	})
}

// GetMetadata retrieves a metadata value.
func (m *MEBStore) GetMetadata(key string) (string, error) {
	var value []byte
	err := m.withReadTxn(func(txn *badger.Txn) error {
		fullKey := []byte("meta:" + key)
		item, err := txn.Get(fullKey)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			value = append([]byte(nil), val...)
			return nil
		})
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return "", nil // Return empty string if not found
		}
		return "", err
	}
	return string(value), nil
}

// ResolveID converts a numeric ID back to its string representation.
func (m *MEBStore) ResolveID(id uint64) (string, bool) {
	val, err := m.dict.GetString(id)
	if err != nil {
		return "", false
	}
	return val, true
}

// LookupID finds the ID for a given string.
func (m *MEBStore) LookupID(val string) (uint64, bool) {
	id, err := m.dict.GetID(val)
	if err != nil {
		return 0, false
	}
	return id, true
}

// ExecuteBatch executes a function within a single transaction.
func (m *MEBStore) ExecuteBatch(fn func(store *MEBStore) error) error {
	// Start a new update transaction
	return m.db.Update(func(txn *badger.Txn) error {
		// Create a shallow copy of the store
		batchedStore := *m
		// Inject the transaction
		batchedStore.txn = txn
		// Execute the callback with the batched store
		return fn(&batchedStore)
	})
}

// newTxn creates a new read-only transaction.
func (m *MEBStore) newTxn() *badger.Txn {
	return m.db.NewTransaction(false)
}

// releaseTxn discards the transaction.
func (m *MEBStore) releaseTxn(txn *badger.Txn) {
	txn.Discard()
}

// Reset clears the store by deleting all data.
func (m *MEBStore) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Info("resetting store", "factCount", m.numFacts.Load())

	// Clear all data
	err := m.db.DropAll()
	if err != nil {
		slog.Error("failed to drop all data", "error", err)
		return fmt.Errorf("failed to reset store: %w", err)
	}

	// Reset fact count (atomic operation)
	m.numFacts.Store(0)

	slog.Info("store reset complete")
	return nil
}

// Close closes the store and releases resources.
func (m *MEBStore) Close() error {
	slog.Info("closing store", "factCount", m.numFacts.Load())

	// Save vector snapshot before closing
	if !m.config.ReadOnly {
		if err := m.vectors.SaveSnapshot(); err != nil {
			slog.Error("failed to save vector snapshot", "error", err)
			return fmt.Errorf("failed to save vector snapshot: %w", err)
		}
	}

	// Save fact count stats to disk
	if err := m.saveStats(); err != nil {
		slog.Error("failed to save stats", "error", err)
		return fmt.Errorf("failed to save stats: %w", err)
	}

	// Wait for vector operations to complete
	if err := m.vectors.Close(); err != nil {
		slog.Error("failed to close vectors", "error", err)
		return err
	}

	// Close dictionary
	if err := m.dict.Close(); err != nil {
		slog.Error("failed to close dictionary", "error", err)
		return err
	}

	// Close Dictionary BadgerDB
	if err := m.dictDB.Close(); err != nil {
		slog.Error("failed to close dictionary database", "error", err)
		// We still try to close the main DB even if this fails
	}

	// Close BadgerDB
	if err := m.db.Close(); err != nil {
		slog.Error("failed to close database", "error", err)
		return err
	}

	slog.Info("store closed successfully")
	return nil
}

// Count returns the total number of facts in the store.
// This is a zero-cost atomic read from memory.
func (m *MEBStore) Count() uint64 {
	return m.numFacts.Load()
}

// RecalculateStats forces a full DB scan to fix the fact counter.
// This is an expensive operation that should only be used if the counter
// is suspected to be out of sync (e.g., after an unclean shutdown).
// It scans the SPOG index and updates both the in-memory counter and disk.
func (m *MEBStore) RecalculateStats() (uint64, error) {
	slog.Info("recalculating stats (expensive operation)", "currentCount", m.numFacts.Load())

	var count uint64

	err := m.withReadTxn(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Key-only is much faster
		it := txn.NewIterator(opts)
		defer it.Close()

		// Count only primary SPO keys
		// Note: We use SPOPrefix (0x01) because AddFactBatch currently writes 25-byte Triple keys.
		// Although QuadSPOGPrefix (0x20) is defined, it is not yet used for writing facts.
		prefix := []byte{keys.SPOPrefix}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			// Ensure we are counting valid keys
			if len(item.Key()) == keys.TripleKeySize {
				count++
			}
		}
		return nil
	})

	if err != nil {
		slog.Error("failed to recalculate stats", "error", err)
		return 0, fmt.Errorf("failed to recalculate stats: %w", err)
	}

	// Update RAM counter
	m.numFacts.Store(count)

	// Save to disk
	if err := m.saveStats(); err != nil {
		slog.Error("failed to save recalculated stats", "error", err)
		return 0, fmt.Errorf("failed to save recalculated stats: %w", err)
	}

	slog.Info("stats recalculated successfully", "newCount", count)
	return count, nil
}

// Vectors returns the vector registry for vector search operations.
func (m *MEBStore) Vectors() *vector.VectorRegistry {
	return m.vectors
}

// Find returns a new query builder for neuro-symbolic search.
// Example:
//
//	results, err := store.Find().
//	    SimilarTo(embedding, 0.8).
//	    Where("author", "alice").
//	    Limit(5).
//	    Execute()
func (m *MEBStore) Find() *Builder {
	return NewBuilder(m)
}

// GetAllPredicates returns a sorted list of all unique predicates in the store.
func (m *MEBStore) GetAllPredicates() ([]string, error) {
	predicateSet := make(map[string]bool)

	// Use a read transaction to scan the PSO index
	err := m.withReadTxn(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys
		it := txn.NewIterator(opts)
		defer it.Close()

		// Scan PSO index
		prefix := []byte{keys.PSOPrefix}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			if len(key) != keys.TripleKeySize {
				continue
			}

			// Decode PSO key to get predicate ID
			_, pID, _ := keys.DecodePSOKey(key)

			// Resolve predicate ID to string
			predStr, err := m.dict.GetString(pID)
			if err != nil {
				// Skip unresolvable predicates
				continue
			}

			predicateSet[predStr] = true
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Convert set to sorted slice
	predicates := make([]string, 0, len(predicateSet))
	for pred := range predicateSet {
		predicates = append(predicates, pred)
	}
	sort.Strings(predicates)

	return predicates, nil
}

// GetTopSymbols returns the top N most frequent symbols (subjects/objects).
// An optional filter function can be provided to exclude certain symbols.
func (m *MEBStore) GetTopSymbols(limit int, filter func(string) bool) ([]string, error) {
	symbolFreq := make(map[string]int)

	// Scan all facts and count subject/object occurrences
	err := m.withReadTxn(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		// Scan SPO index
		prefix := []byte{keys.SPOPrefix}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			if len(key) != keys.TripleKeySize {
				continue
			}

			// Decode key to get S, P, O IDs
			sID, _, oID := keys.DecodeSPOKey(key)

			// Resolve IDs to strings
			subjectStr, err := m.dict.GetString(sID)
			if err == nil {
				if filter == nil || filter(subjectStr) {
					symbolFreq[subjectStr]++
				}
			}

			objectStr, err := m.dict.GetString(oID)
			if err == nil {
				if filter == nil || filter(objectStr) {
					symbolFreq[objectStr]++
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by frequency (descending)
	type symbolCount struct {
		symbol string
		count  int
	}

	symbols := make([]symbolCount, 0, len(symbolFreq))
	for symbol, count := range symbolFreq {
		symbols = append(symbols, symbolCount{symbol, count})
	}

	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].count > symbols[j].count
	})

	// Take top N
	topN := limit
	if topN > len(symbols) {
		topN = len(symbols)
	}

	result := make([]string, topN)
	for i := 0; i < topN; i++ {
		result[i] = symbols[i].symbol
	}

	return result, nil
}
