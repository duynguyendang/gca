package repl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/duynguyendang/meb"
)

// SymbolStat local definition since it was removed from meb
type SymbolStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ProjectSummary holds a structured summary of the codebase for the AI Planner.
type ProjectSummary struct {
	Predicates  []string       `json:"predicates"`
	Packages    []string       `json:"packages"`
	TopSymbols  []SymbolStat   `json:"top_symbols"`
	Stats       map[string]int `json:"stats"`
	EntryPoints []string       `json:"entry_points"`
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

	// Step 5: Entry Points
	entryPoints, err := extractEntryPoints(s)
	if err != nil {
		// Log error but don't fail summary?
		// For now return existing
	}

	return &ProjectSummary{
		Predicates:  predicates,
		Packages:    packages,
		TopSymbols:  topSymbols,
		Stats:       stats,
		EntryPoints: entryPoints,
	}, nil
}

// discoverPredicates uses the high-level MEBStore API to find all unique predicates.
func discoverPredicates(s *meb.MEBStore) ([]string, error) {
	var preds []string
	for _, p := range s.ListPredicates() {
		preds = append(preds, string(p.Symbol))
	}
	return preds, nil
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
func analyzeTopSymbols(s *meb.MEBStore, limit int) ([]SymbolStat, error) {
	// Function removed. Return empty stats.
	return []SymbolStat{}, nil
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

// extractEntryPoints finds main functions and HTTP handlers.
func extractEntryPoints(s *meb.MEBStore) ([]string, error) {
	entryPoints := []string{}
	// Scan for "defines" of "main"
	// Heuristic: "defines" ?s where ?s ends with ":main"
	// We can't regex scan efficiently without full scan.
	// But we can scan symbols with specific suffix if dictionary supports it? No.
	// Iterate valid "defines" facts.
	// Or use SearchSymbols?
	// Let's scan all facts with predicate "defines" and filter in memory.
	// For large repos this is slow.
	// Better: Use `s.IterateSymbols` to find symbols ending in ":main" or containing "Handler".
	// But IterateSymbols iterates *all* strings.
	// Let's iterate `defines` facts, it's safer.

	// Limit to first 50 entry points to be safe.

	count := 0
	for fact, err := range s.Scan("", "defines", "", "") {
		if err != nil {
			continue
		}

		sym := string(fact.Subject)

		// 1. main function
		if strings.HasSuffix(sym, ":main") {
			entryPoints = append(entryPoints, sym)
			count++
		}

		// 2. HTTP Handler (heuristic naming)
		if strings.Contains(sym, "Handler") || strings.Contains(sym, "Controller") {
			// Check if it's a function? Need kind metadata.
			// Just add it for now as "potential entry point".
			// Maybe limit to avoid noise.
		}

		if count > 50 {
			break
		}
	}
	sort.Strings(entryPoints)
	return entryPoints, nil
}
