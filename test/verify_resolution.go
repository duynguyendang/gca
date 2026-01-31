package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

// Simplified version of findFilesWithPrefix for verification
func findFilesWithPrefix(store *meb.MEBStore, prefix string) []string {
	var files []string
	seen := make(map[string]bool)

	toSlashed := func(p string) string {
		return strings.ReplaceAll(p, ".", "/")
	}

	for fact, _ := range store.Scan("", "in_package", "", "") {
		filePath := string(fact.Subject)
		pkgName, ok := fact.Object.(string)
		if !ok {
			continue
		}

		matched := false

		// 1. Check if filePath matches prefix directly
		if strings.HasPrefix(filePath, prefix) {
			matched = true
		}

		// 2. Check if internal normalized package matches prefix
		internalPkg := toSlashed(pkgName)

		parts := strings.Split(prefix, "/")
		if len(parts) > 2 {
			suffix := strings.Join(parts[len(parts)-2:], "/")
			if strings.Contains(internalPkg, suffix) {
				matched = true
			}
		} else if len(parts) > 0 {
			suffix := parts[len(parts)-1]
			if strings.HasSuffix(internalPkg, "/"+suffix) || internalPkg == suffix {
				matched = true
			}
		}

		if matched && !seen[filePath] {
			seen[filePath] = true
			files = append(files, filePath)
		}
	}
	return files
}

func main() {
	dataDir := "./data/gca"
	fmt.Printf("Opening %s directly (ReadOnly)...\n", dataDir)

	cfg := store.DefaultConfig(dataDir)
	cfg.ReadOnly = true
	cfg.BypassLockGuard = true

	s, err := meb.Open(dataDir, cfg)
	if err != nil {
		fmt.Printf("Failed to open: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	prefix := "github.com/duynguyendang/gca/pkg/meb"
	files := findFilesWithPrefix(s, prefix)
	fmt.Printf("Files found for prefix '%s': %d\n", prefix, len(files))
	for _, f := range files {
		fmt.Println(" -", f)
	}

	// Check if expected file is found
	found := false
	for _, f := range files {
		if strings.Contains(f, "analysis.go") || strings.Contains(f, "store.go") {
			found = true
			break
		}
	}

	if found {
		fmt.Println("SUCCESS: Found meb files via package resolution")
		os.Exit(0)
	} else {
		fmt.Println("FAILURE: meb files not found")
		os.Exit(1)
	}
}
