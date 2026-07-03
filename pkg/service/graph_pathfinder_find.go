package service

import (
	"context"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

// findFileForSymbolByStore looks up the file that defines a symbol using MEB store.
// It handles both qualified symbols (e.g., "main.go:main") and unqualified
// symbols (e.g., "fmt.Println" or just "Println") by querying has_name and defines predicates.
func findFileForSymbolByStore(ctx context.Context, store *meb.MEBStore, target string) string {
	// If target already has file prefix (format "file:symbol"), extract it
	if strings.Contains(target, ":") && isValidFilePath(strings.SplitN(target, ":", 2)[0]) {
		return strings.SplitN(target, ":", 2)[0]
	}

	// Try direct lookup via defines predicate (O(1) via OPS index)
	// Query: find subjects where defines(subject, target)
	for fact, err := range store.ScanContext(ctx, "", config.PredicateDefines, target) {
		if err != nil {
			continue
		}
		if obj, ok := fact.Object.(string); ok && obj == target {
			return fact.Subject
		}
	}

	// Try by short name using has_name predicate
	shortName := target
	if strings.Contains(target, ".") {
		parts := strings.Split(target, ".")
		shortName = parts[len(parts)-1]
	}

	// Find all symbols with this short name
	var candidates []string
	for subject := range store.FindSubjectsByObject(ctx, config.PredicateHasName, shortName) {
		candidates = append(candidates, subject)
	}

	if len(candidates) == 0 {
		return ""
	}

	// Find best candidate - prefer same package, shortest match
	bestFile := ""
	bestScore := -1
	for _, sym := range candidates {
		file := findFileForSymbolByStore(ctx, store, sym)
		if file == "" {
			continue
		}
		score := 0
		if strings.Contains(sym, shortName) {
			score++
		}
		if len(sym) < 50 {
			score++
		}
		if score > bestScore {
			bestScore = score
			bestFile = file
		}
	}

	return bestFile
}

// findFileForSymbol looks up the file that defines a symbol.
// NOTE: This function is kept for backward compatibility but should use findFileForSymbolByStore for better performance.
func findFileForSymbol(target string, symbolToFile map[string]string) string {
	// Direct lookup first
	if file, exists := symbolToFile[target]; exists {
		return file
	}

	// Try to find by suffix - e.g., target="fmt.Println" -> look for "*/fmt.Println"
	// or target="Println" -> look for "*/Println"
	for sym, file := range symbolToFile {
		if strings.HasSuffix(sym, ":"+target) || sym == target {
			return file
		}
	}

	// Try stripping package prefix - e.g., "fmt.Println" -> "Println"
	parts := strings.Split(target, ".")
	if len(parts) > 1 {
		lastPart := parts[len(parts)-1]
		for sym, file := range symbolToFile {
			if strings.HasSuffix(sym, ":"+lastPart) {
				return file
			}
		}
	}

	return ""
}

// findProjectFileForImport converts an import path to a project file path.
// For example, "github.com/firebase/genkit/go/core" might map to "genkit-go/core"
// if the project is named "genkit-go".
func findProjectFileForImport(importPath, projectID string) string {
	// If the import starts with the project ID, convert it
	if strings.HasPrefix(importPath, projectID) {
		return strings.TrimPrefix(importPath, projectID+"/")
	}

	// Try common patterns
	// e.g., "github.com/firebase/genkit/go/core" -> "genkit-go/core" for projectID "genkit-go"
	parts := strings.Split(importPath, "/")
	if len(parts) >= 3 {
		// Try to find the project in the import path
		// "github.com/firebase/genkit/go/core" -> look for "genkit" or projectID
		for i, part := range parts {
			if part == projectID || strings.Contains(importPath, projectID) {
				// Reconstruct with just the path after the project
				remaining := strings.Join(parts[i+1:], "/")
				if remaining != "" {
					return remaining
				}
			}
		}
	}

	// Return the import path as-is (might be external)
	return ""
}

// isValidFilePath checks if a string looks like a valid source file path
func isValidFilePath(path string) bool {
	if path == "" {
		return false
	}
	for _, ext := range config.SourceFileExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
