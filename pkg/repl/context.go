package repl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/keys"
)

// ProjectSummary holds a structured summary of the codebase for the AI Planner.
type ProjectSummary struct {
	Predicates []string       `json:"predicates"`
	Packages   []string       `json:"packages"`
	TopSymbols []string       `json:"top_symbols"`
	Stats      map[string]int `json:"stats"`
}

// GenerateProjectSummary scans the database and generates a structured context summary.
// This provides the AI Planner with actual project structure and symbols to prevent hallucinations.
func GenerateProjectSummary(s *meb.MEBStore) (*ProjectSummary, error) {
	// Step 1: Schema Discovery
	predicates, err := discoverPredicates(s)
	if err != nil {
		return nil, fmt.Errorf("schema discovery failed: %w", err)
	}

	// Step 2: Project Tree Generation
	packages, err := extractPackages(s)
	if err != nil {
		return nil, fmt.Errorf("package extraction failed: %w", err)
	}

	// Step 3: Symbol Frequency Analysis
	topSymbols, err := analyzeTopSymbols(s, 50)
	if err != nil {
		return nil, fmt.Errorf("symbol analysis failed: %w", err)
	}

	// Step 4: System Statistics
	stats := gatherStats(s, len(predicates), len(packages), len(topSymbols))

	return &ProjectSummary{
		Predicates: predicates,
		Packages:   packages,
		TopSymbols: topSymbols,
		Stats:      stats,
	}, nil
}

// discoverPredicates iterates through the database to find all unique predicates.
// It scans the PSO index to efficiently collect predicate IDs.
func discoverPredicates(s *meb.MEBStore) ([]string, error) {
	predicateSet := make(map[string]bool)

	// Use a read transaction to scan the PSO index
	err := s.DB().View(func(txn *badger.Txn) error {
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
			predStr, err := s.Dict().GetString(pID)
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

// extractPackages scans for "defines" predicate facts and extracts unique directory paths.
// This builds a virtual file tree from the facts.
func extractPackages(s *meb.MEBStore) ([]string, error) {
	packageSet := make(map[string]bool)

	// Scan for "defines" facts
	for fact, err := range s.Scan("", "defines", "", "") {
		if err != nil {
			// Skip errors (e.g., predicate not found)
			continue
		}

		// Extract the file path from the Subject
		filePath := fact.Subject

		// Split by "/" and extract directory parts
		parts := strings.Split(filePath, "/")
		if len(parts) > 1 {
			// Build package path by removing the filename
			// For "mangle/pkg/parser/file.go", we want "mangle/pkg/parser"
			pkgPath := strings.Join(parts[:len(parts)-1], "/")
			if pkgPath != "" {
				packageSet[pkgPath] = true
			}
		}
	}

	// Convert set to sorted slice
	packages := make([]string, 0, len(packageSet))
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	return packages, nil
}

// analyzeTopSymbols counts the frequency of subjects and objects in the database,
// then returns the top N most frequent symbols (excluding common stdlib types).
func analyzeTopSymbols(s *meb.MEBStore, limit int) ([]string, error) {
	symbolFreq := make(map[string]int)

	// Common stdlib types to exclude
	stdlibTypes := map[string]bool{
		"int": true, "int64": true, "int32": true, "int16": true, "int8": true,
		"uint": true, "uint64": true, "uint32": true, "uint16": true, "uint8": true,
		"float64": true, "float32": true, "string": true, "bool": true,
		"byte": true, "rune": true, "error": true, "interface{}": true,
		"map": true, "slice": true, "chan": true, "func": true,
		"true": true, "false": true, "nil": true,
	}

	// Scan all facts and count subject/object occurrences
	err := s.DB().View(func(txn *badger.Txn) error {
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
			subjectStr, err := s.Dict().GetString(sID)
			if err == nil && !stdlibTypes[subjectStr] && subjectStr != "" {
				symbolFreq[subjectStr]++
			}

			objectStr, err := s.Dict().GetString(oID)
			if err == nil && !stdlibTypes[objectStr] && objectStr != "" {
				symbolFreq[objectStr]++
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

// gatherStats computes high-level system statistics.
func gatherStats(s *meb.MEBStore, uniquePredicates, uniquePackages, topSymbolsCount int) map[string]int {
	factCount := int(s.Count())

	stats := map[string]int{
		"total_facts":       factCount,
		"unique_predicates": uniquePredicates,
		"unique_packages":   uniquePackages,
		"top_symbols_count": topSymbolsCount,
	}

	// Compute fact density (facts per package)
	if uniquePackages > 0 {
		stats["facts_per_package"] = factCount / uniquePackages
	}

	return stats
}
