package repl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/duynguyendang/gca/pkg/meb"
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

// discoverPredicates uses the high-level MEBStore API to find all unique predicates.
func discoverPredicates(s *meb.MEBStore) ([]string, error) {
	return s.GetAllPredicates()
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
		parts := strings.Split(string(filePath), "/")
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

// analyzeTopSymbols retrieves the top N most frequent symbols using MEBStore API.
func analyzeTopSymbols(s *meb.MEBStore, limit int) ([]string, error) {
	// Common stdlib types to exclude
	stdlibTypes := map[string]bool{
		"int": true, "int64": true, "int32": true, "int16": true, "int8": true,
		"uint": true, "uint64": true, "uint32": true, "uint16": true, "uint8": true,
		"float64": true, "float32": true, "string": true, "bool": true,
		"byte": true, "rune": true, "error": true, "interface{}": true,
		"map": true, "slice": true, "chan": true, "func": true,
		"true": true, "false": true, "nil": true,
	}

	filter := func(sym string) bool {
		return !stdlibTypes[sym] && sym != ""
	}

	return s.GetTopSymbols(limit, filter)
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
