package entity

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/nlp/types"
)

var (
	// goFileRegex matches any path ending in .go, including paths with dots,
	// backslashes (Windows-style), hyphens, and slashes.
	goFileRegex = regexp.MustCompile(`[a-zA-Z0-9_./\\/-]+\.go\b`)
	// serviceSymbolRegex matches CamelCase identifiers ending with common service suffixes.
	serviceSymbolRegex = regexp.MustCompile(`[A-Z][a-zA-Z0-9]+Service`)
	// pkgPathRegex matches slash-separated two-segment package paths (e.g. auth/service).
	pkgPathRegex = regexp.MustCompile(`[a-z][a-zA-Z0-9]+/[a-z][a-zA-Z0-9]+`)
	// goSymbolFileRegex matches simpler .go file paths (no dots/hyphens/backslashes in path segments).
	goSymbolFileRegex = regexp.MustCompile(`[a-zA-Z0-9_/]+\.go\b`)
)

type EntityStore interface {
	LookupID(key string) (uint64, bool)
	ScanFacts(subject, predicate, object string) [][3]string
	GetContentByKey(key string) ([]byte, error)
}

type Resolver struct {
	store    EntityStore
	cache    map[string][]*types.Entity
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
	cacheTime map[string]time.Time
	searchCache    map[string][]*types.Entity
	searchCacheMu  sync.RWMutex
	searchCacheTTL time.Duration
	searchCacheTime map[string]time.Time
}

func NewResolver(store EntityStore) *Resolver {
	return &Resolver{
		store:     store,
		cache:     make(map[string][]*types.Entity),
		cacheTime: make(map[string]time.Time),
		cacheTTL:  5 * time.Minute,
		searchCache:     make(map[string][]*types.Entity),
		searchCacheTime: make(map[string]time.Time),
		searchCacheTTL:  10 * time.Minute,
	}
}

type ResolveOptions struct {
	MaxResults int
	Strict     bool
}

func DefaultResolveOptions() *ResolveOptions {
	return &ResolveOptions{
		MaxResults: 10,
		Strict:     false,
	}
}

func (r *Resolver) Resolve(input string, ctx []*types.Entity) ([]*types.Entity, error) {
	if input == "" {
		return nil, nil
	}

	r.cacheMu.RLock()
	if ents, ok := r.cache[input]; ok && time.Since(r.cacheTime[input]) < r.cacheTTL {
		r.cacheMu.RUnlock()
		return ents, nil
	}
	r.cacheMu.RUnlock()

	var results []*types.Entity

	if sym, found := r.ResolveSymbol(input); found {
		results = append(results, sym)
	}

	if file, found := r.ResolveFile(input); found {
		results = append(results, file)
	}

	if pkg, found := r.ResolvePackage(input); found {
		results = append(results, pkg)
	}

	patterns := []struct {
		pattern *regexp.Regexp
		resolve func(string) *types.Entity
	}{
		{
			goSymbolFileRegex,
			func(s string) *types.Entity { e, _ := r.ResolveFile(s); return e },
		},
		{
			serviceSymbolRegex,
			func(s string) *types.Entity { e, _ := r.ResolveSymbol(s); return e },
		},
		{
			pkgPathRegex,
			func(s string) *types.Entity { e, _ := r.ResolvePackage(s); return e },
		},
	}

	for _, p := range patterns {
		if match := p.pattern.FindString(input); match != "" {
			if e := p.resolve(match); e != nil {
				results = append(results, e)
			}
		}
	}

	for _, e := range ctx {
		if strings.EqualFold(e.Name, input) {
			results = append(results, e)
		}
	}

	if len(results) == 0 {
		syms := r.searchSymbols(input)
		for i := range syms {
			results = append(results, syms[i])
			if len(results) >= 10 {
				break
			}
		}
	}

	r.cacheMu.Lock()
	r.cache[input] = results
	r.cacheTime[input] = time.Now()
	r.cacheMu.Unlock()

	return results, nil
}

func (r *Resolver) ResolveSymbol(name string) (*types.Entity, bool) {
	if r.store == nil || name == "" {
		return nil, false
	}

	name = strings.Trim(name, "\"' ")

	if _, exists := r.store.LookupID(name); exists {
		return &types.Entity{
			ID:         name,
			Name:       name,
			EntityType: "symbol",
			Source:     "store",
		}, true
	}

	syms := r.searchSymbols(name)
	if len(syms) > 0 {
		return syms[0], true
	}

	return nil, false
}

func (r *Resolver) ResolveFile(path string) (*types.Entity, bool) {
	if path == "" {
		return nil, false
	}

	path = strings.Trim(path, "\"' ")

	if goFileRegex.MatchString(path) {
		return &types.Entity{
			ID:         path,
			Name:       path,
			EntityType: "file",
			Source:     "pattern",
		}, true
	}

	return nil, false
}

func (r *Resolver) ResolvePackage(name string) (*types.Entity, bool) {
	if name == "" {
		return nil, false
	}

	name = strings.Trim(name, "\"' ")

	if strings.Contains(name, "/") && !strings.HasSuffix(name, ".go") {
		return &types.Entity{
			ID:         name,
			Name:       name,
			EntityType: "package",
			Source:     "pattern",
		}, true
	}

	return nil, false
}

func (r *Resolver) searchSymbols(query string) []*types.Entity {
	if r.store == nil {
		return nil
	}

	r.searchCacheMu.RLock()
	if cached, ok := r.searchCache[query]; ok && time.Since(r.searchCacheTime[query]) < r.searchCacheTTL {
		r.searchCacheMu.RUnlock()
		return cached
	}
	r.searchCacheMu.RUnlock()

	var results []*types.Entity
	upperQuery := strings.ToUpper(query)
	lowerQuery := strings.ToLower(query)

	facts := r.store.ScanFacts("", config.PredicateDefines, "")
	for _, fact := range facts {
		symID := fact[2]

		symName := common.ExtractSymbolName(symID)
		symNameUpper := strings.ToUpper(symName)
		symNameLower := strings.ToLower(symName)

		if strings.Contains(symNameUpper, upperQuery) ||
			strings.Contains(symNameLower, lowerQuery) ||
			symNameUpper == upperQuery ||
			symNameLower == lowerQuery {
			results = append(results, &types.Entity{
				ID:         symID,
				Name:       symName,
				EntityType: "symbol",
				Source:     "search",
			})
			if len(results) >= 10 {
				break
			}
		}
	}

	r.searchCacheMu.Lock()
	r.searchCache[query] = results
	r.searchCacheTime[query] = time.Now()
	r.searchCacheMu.Unlock()

	return results
}

