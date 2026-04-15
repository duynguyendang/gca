package meb

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/datalog"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/keys"
	"github.com/duynguyendang/meb/query"
)

// QueryCache provides TTL-based caching for query results
type QueryCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
	enabled bool
}

type cacheEntry struct {
	results   []map[string]any
	expiresAt time.Time
	createdAt time.Time
}

func NewQueryCache(ttl time.Duration, maxSize int, enabled bool) *QueryCache {
	cache := &QueryCache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
		enabled: enabled,
	}
	if enabled {
		go cache.cleanupExpired()
	}
	return cache
}

func (c *QueryCache) get(key string) ([]map[string]any, bool) {
	if !c.enabled {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.results, true
}

func (c *QueryCache) set(key string, results []map[string]any) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}
	c.entries[key] = &cacheEntry{
		results:   results,
		expiresAt: time.Now().Add(c.ttl),
		createdAt: time.Now(),
	}
}

func (c *QueryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for key, entry := range c.entries {
		if first || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
			first = false
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func (c *QueryCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.expiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

func (c *QueryCache) hashKey(query string) string {
	h := sha256.Sum256([]byte(query))
	return fmt.Sprintf("%x", h[:8])
}

var globalQueryCache = NewQueryCache(config.QueryCacheTTL, config.QueryCacheMaxSize, config.QueryCacheEnabled)

type Store struct {
	*meb.MEBStore
}

func NewStore(db *meb.MEBStore) *Store {
	return &Store{db}
}

func Query(ctx context.Context, store *meb.MEBStore, q string) ([]map[string]any, error) {
	return QueryWithLimit(ctx, store, q, config.QueryResultLimit)
}

func QueryWithLimit(ctx context.Context, store *meb.MEBStore, q string, limit int) ([]map[string]any, error) {
	cacheKey := globalQueryCache.hashKey(q)
	if cached, ok := globalQueryCache.get(cacheKey); ok {
		if len(cached) > limit {
			return cached[:limit], nil
		}
		return cached, nil
	}

	atoms, err := datalog.Parse(q)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	if len(atoms) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	triplesAtoms := make([]datalog.Atom, 0, len(atoms))
	constraintAtoms := make([]datalog.Atom, 0)

	for _, atom := range atoms {
		if atom.Predicate == "triples" {
			triplesAtoms = append(triplesAtoms, atom)
		} else {
			constraintAtoms = append(constraintAtoms, atom)
		}
	}

	if len(triplesAtoms) == 0 {
		return nil, fmt.Errorf("query must contain at least one triples atom")
	}

	var results []map[string]any

	if len(triplesAtoms) == 1 {
		results = executeSingleAtomQuery(ctx, store, triplesAtoms[0], limit)
	} else {
		results = executeLFTJQuery(ctx, store, triplesAtoms, limit)
		if len(results) == 0 && len(triplesAtoms) > 1 {
			log.Printf("LFTJ engine returned no results for multi-atom query, falling back to sequential join")
			results = executeSequentialJoinQuery(ctx, store, triplesAtoms, limit)
		}
	}

	results = applyConstraints(results, constraintAtoms)

	if len(results) > limit {
		results = results[:limit]
	}

	globalQueryCache.set(cacheKey, results)

	return results, nil
}

func (s *Store) Query(ctx context.Context, q string) ([]map[string]any, error) {
	return Query(ctx, s.MEBStore, q)
}

// scanFacts scans facts using meb's ScanContext with proper SPO index support.
func scanFacts(ctx context.Context, store *meb.MEBStore, subj, pred, obj string) <-chan struct {
	Fact meb.Fact
	Err  error
} {
	ch := make(chan struct {
		Fact meb.Fact
		Err  error
	}, 1)

	go func() {
		defer close(ch)
		for fact, err := range store.ScanContext(ctx, subj, pred, obj) {
			ch <- struct {
				Fact meb.Fact
				Err  error
			}{Fact: fact, Err: err}
		}
	}()

	return ch
}

func executeSingleAtomQuery(ctx context.Context, store *meb.MEBStore, atom datalog.Atom, limit int) []map[string]any {
	var results []map[string]any

	subj := resolveArg(atom.Args[0])
	pred := resolveArg(atom.Args[1])
	obj := resolveArg(atom.Args[2])

	subjIsVar := isVariable(atom.Args[0])
	predIsVar := isVariable(atom.Args[1])
	objIsVar := isVariable(atom.Args[2])

	for item := range scanFacts(ctx, store, subj, pred, obj) {
		if item.Err != nil {
			continue
		}
		fact := item.Fact

		result := make(map[string]any)
		if subjIsVar {
			result[atom.Args[0]] = fact.Subject
		}
		if predIsVar {
			result[atom.Args[1]] = fact.Predicate
		}
		if objIsVar {
			result[atom.Args[2]] = fact.Object
		}

		if len(result) > 0 {
			results = append(results, result)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results
}

func executeLFTJQuery(ctx context.Context, store *meb.MEBStore, atoms []datalog.Atom, limit int) []map[string]any {
	var results []map[string]any

	relations, resultVars, err := buildLFTJRelations(store, atoms)
	if err != nil {
		return results
	}
	if len(relations) == 0 {
		return results
	}

	boundVars := make(map[string]uint64)

	engine := store.LFTJEngine()
	if engine == nil {
		return results
	}

	var mu sync.Mutex

	for joinResult, err := range engine.Execute(ctx, relations, boundVars, resultVars) {
		if err != nil {
			continue
		}

		row := make(map[string]any)
		for varName, dictID := range joinResult {
			if dictID == 0 {
				continue
			}

			localID := keys.UnpackLocalID(dictID)
			strVal, err := store.ResolveID(localID)
			if err != nil {
				continue
			}
			row[varName] = strVal
		}

		if len(row) > 0 {
			mu.Lock()
			results = append(results, row)
			if limit > 0 && len(results) >= limit {
				mu.Unlock()
				break
			}
			mu.Unlock()
		}
	}

	return results
}

func executeSequentialJoinQuery(ctx context.Context, store *meb.MEBStore, atoms []datalog.Atom, limit int) []map[string]any {
	var results []map[string]any

	firstAtom := atoms[0]
	subj := resolveArg(firstAtom.Args[0])
	pred := resolveArg(firstAtom.Args[1])
	obj := resolveArg(firstAtom.Args[2])

	for item := range scanFacts(ctx, store, subj, pred, obj) {
		if item.Err != nil {
			continue
		}
		fact := item.Fact

		row := make(map[string]any)
		if isVariable(firstAtom.Args[0]) {
			row[firstAtom.Args[0]] = fact.Subject
		}
		if isVariable(firstAtom.Args[1]) {
			row[firstAtom.Args[1]] = fact.Predicate
		}
		if isVariable(firstAtom.Args[2]) {
			row[firstAtom.Args[2]] = fact.Object
		}

		for _, atom := range atoms[1:] {
			resolvedArgs := make([]string, 3)
			for i, arg := range atom.Args[:3] {
				if isVariable(arg) {
					if val, ok := row[arg]; ok {
						resolvedArgs[i] = fmt.Sprintf("%v", val)
					}
				} else {
					resolvedArgs[i] = resolveArg(arg)
				}
			}

			found := false
			for item := range scanFacts(ctx, store, resolvedArgs[0], resolvedArgs[1], resolvedArgs[2]) {
				if item.Err != nil {
					continue
				}
				f := item.Fact
				if isVariable(atom.Args[0]) {
					row[atom.Args[0]] = f.Subject
				}
				if isVariable(atom.Args[1]) {
					row[atom.Args[1]] = f.Predicate
				}
				if isVariable(atom.Args[2]) {
					row[atom.Args[2]] = f.Object
				}
				found = true
				break
			}
			if !found {
				goto nextFact
			}
		}

		if len(row) > 0 {
			results = append(results, row)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	nextFact:
	}

	return results
}

func buildLFTJRelations(store *meb.MEBStore, atoms []datalog.Atom) ([]query.RelationPattern, []string, error) {
	relations := make([]query.RelationPattern, 0, len(atoms))
	resultVarsSet := make(map[string]bool)
	topicID := store.TopicID()

	for _, atom := range atoms {
		if atom.Predicate != "triples" || len(atom.Args) < 3 {
			continue
		}

		boundPositions := make(map[int]uint64)
		variablePositions := make(map[int]string)
		skipAtom := false

		for argIdx, arg := range atom.Args[:3] {
			if isVariable(arg) {
				varName := arg
				variablePositions[argIdx] = varName
				resultVarsSet[varName] = true
			} else {
				strVal := resolveArg(arg)
				dictID, found := store.LookupID(strVal)
				if !found {
					log.Printf("Warning: dictionary lookup failed for %q in atom %v, skipping atom", strVal, atom)
					skipAtom = true
					break
				}
				packedID := keys.PackID(topicID, dictID)
				boundPositions[argIdx] = packedID
			}
		}

		if skipAtom {
			continue
		}

		relations = append(relations, query.RelationPattern{
			Prefix:            keys.TripleSPOPrefix,
			BoundPositions:    boundPositions,
			VariablePositions: variablePositions,
		})
	}

	resultVars := make([]string, 0, len(resultVarsSet))
	for v := range resultVarsSet {
		resultVars = append(resultVars, v)
	}

	return relations, resultVars, nil
}

func applyConstraints(results []map[string]any, constraintAtoms []datalog.Atom) []map[string]any {
	if len(constraintAtoms) == 0 {
		return results
	}

	filtered := make([]map[string]any, 0, len(results))

	for _, result := range results {
		if matchesConstraints(result, constraintAtoms) {
			filtered = append(filtered, result)
		}
	}

	return filtered
}

func matchesConstraints(result map[string]any, constraints []datalog.Atom) bool {
	for _, atom := range constraints {
		switch atom.Predicate {
		case "neq", "!=":
			if len(atom.Args) >= 2 {
				varName := atom.Args[0]
				constraintVal := atom.Args[1]
				if val, ok := result[varName]; ok {
					if fmt.Sprintf("%v", val) == constraintVal {
						return false
					}
				}
			}
		case "eq", "=":
			if len(atom.Args) >= 2 {
				varName := atom.Args[0]
				constraintVal := atom.Args[1]
				if val, ok := result[varName]; ok {
					if fmt.Sprintf("%v", val) != constraintVal {
						return false
					}
				}
			}
		}
	}
	return true
}

func isVariable(arg string) bool {
	return len(arg) > 0 && (arg[0] == '?' || (arg[0] >= 'A' && arg[0] <= 'Z'))
}

func resolveArg(arg string) string {
	if len(arg) >= 2 && arg[0] == '"' && arg[len(arg)-1] == '"' {
		return arg[1 : len(arg)-1]
	}
	if len(arg) >= 1 && arg[0] == '?' {
		return ""
	}
	if len(arg) > 0 && arg[0] >= 'A' && arg[0] <= 'Z' {
		return ""
	}
	return arg
}
