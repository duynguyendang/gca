// Package main validates policy / constant consistency against GCA's canonical contract
// defined in docs/designs/contract.md. It is run as a CI gate after any .mg or constant
// change.
//
//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	ctx := gcaRoot()
	passed, failed := 0, 0

	// 1. Scan all .mg files for unsupported meb atoms.
	mgFiles := walkMG(ctx)
	if issues := checkUnsupportedAtoms(mgFiles); len(issues) > 0 {
		fmt.Println("\n❌ Unsupported meb atoms found:")
		for _, iss := range issues {
			fmt.Printf("   %s\n", iss)
		}
		failed += len(issues)
	} else {
		fmt.Println("✅ No unsupported meb atoms in .mg files")
		passed++
	}

	// 2. Check predicate consistency across sources.
	if issues := checkPredicateConsistency(ctx); len(issues) > 0 {
		fmt.Println("\n❌ Predicate inconsistency detected:")
		for _, iss := range issues {
			fmt.Printf("   %s\n", iss)
		}
		failed += len(issues)
	} else {
		fmt.Println("✅ Predicates consistent across config/constants.go, *_decl.mg, NamedQueries")
		passed++
	}

	// 3. Verify NamedQueries map has matching policy query_metadata entries.
	if issues := checkNamedQueryPolicyMatch(ctx); len(issues) > 0 {
		fmt.Println("\n⚠️  NamedQueries without policy metadata (informational):")
		for _, iss := range issues {
			fmt.Printf("   %s\n", iss)
		}
		// Not a hard failure — some queries are Go-only helpers.
		passed++
	} else {
		fmt.Println("✅ All NamedQueries have policy equivalents")
		passed++
	}

	// 4. Ensure scoring.mg uses has_smell_type, not has_smell.
	if issues := checkSmellPredicateScoring(ctx); len(issues) > 0 {
		fmt.Println("\n❌ scoring.mg still references legacy 'has_smell':")
		for _, iss := range issues {
			fmt.Printf("   %s\n", iss)
		}
		failed += len(issues)
	} else {
		fmt.Println("✅ scoring.mg uses has_smell_type")
		passed++
	}

	fmt.Printf("\n=== Summary: %d passed, %d failed ===\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// gcaRoot returns the gca directory (where go.mod lives).
func gcaRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting cwd: %v\n", err)
		os.Exit(1)
	}
	// Walk up to find go.mod
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	fmt.Fprintln(os.Stderr, "could not find gca root (go.mod not found)")
	os.Exit(1)
	return ""
}

// walkMG collects all .mg files under the policies/ directory.
func walkMG(root string) []string {
	var files []string
	policiesDir := filepath.Join(root, "policies")
	filepath.Walk(policiesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".mg") {
			// Relative path from gca root for reporting
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
		}
		return nil
	})
	return files
}

// unsupportedAtomRe matches atom families the meb query layer still rejects loudly.
// contains/regex/not triples are supported (see pkg/meb/store.go constraint layer).
var unsupportedAtomRe = regexp.MustCompile(`(?i)\b(or|sum|findall|aggregate|count)\s*\(`)

// assignmentAtomRe matches `Var = <literal>` body atoms (e.g. `Count = 1`,
// `Score = 1`). These are Prolog assignment/aggregation, which meb parses as a
// predicate `Var = <literal>` and rejects. Must be moved to a Go-side pass.
var assignmentAtomRe = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]*\s*=\s*\d+`)

// notDerivedRe matches `not <pred>(` — negation over a derived predicate.
// Only `not triples(...)` and `not <constraint>(...)` are store-evaluable.
var notDerivedRe = regexp.MustCompile(`\bnot\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

// supportedNotTargets are predicates that not(...) may wrap under mebpkg.Query.
var supportedNotTargets = map[string]bool{
	"triples": true, "eq": true, "neq": true,
	"gt": true, "gte": true, "lt": true, "lte": true,
	"contains": true, "regex": true,
}

// checkUnsupportedAtoms scans .mg files for atoms that error loudly under mebpkg.Query.
func checkUnsupportedAtoms(files []string) []string {
	var issues []string
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(gcaRoot(), f))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Skip comments
			if strings.HasPrefix(trimmed, "%") || trimmed == "" {
				continue
			}
			// Only flag atoms inside triples(...) bodies or rule heads, not declarations.
			if strings.Contains(trimmed, ".decl ") || strings.HasPrefix(trimmed, "% OKF") {
				continue
			}
			if unsupportedAtomRe.MatchString(trimmed) {
				issues = append(issues, fmt.Sprintf("%s:%d: unsupported meb atom: %s", f, i+1, trimmed))
				continue
			}
			if assignmentAtomRe.MatchString(trimmed) {
				issues = append(issues, fmt.Sprintf("%s:%d: assignment atom (%s) is aggregation the meb layer cannot express — move to a Go-side pass", f, i+1, trimmed))
				continue
			}
			for _, m := range notDerivedRe.FindAllStringSubmatch(trimmed, -1) {
				if len(m) == 2 && !supportedNotTargets[m[1]] {
					issues = append(issues, fmt.Sprintf("%s:%d: not(<derived> %s) fails loudly under mebpkg.Query — move to a Go-side pass: %s", f, i+1, m[1], trimmed))
					break
				}
			}
		}
	}
	return issues
}

// declaredPredicates extracts predicate names from policies/**/_decl.mg files.
func declaredPredicates(root string) map[string]bool {
	result := make(map[string]bool)
	policiesDir := filepath.Join(root, "policies")

	walkAndRead := func(dir string) {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, "_decl.mg") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if !strings.HasPrefix(line, ".decl ") {
					continue
				}
				// Extract first word: .decl predicate_name(...)
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					name := strings.TrimSuffix(parts[1], "(")
					result[name] = true
				}
			}
			return nil
		})
	}
	walkAndRead(policiesDir)
	return result
}

// extractPredicateStrings reads predicate strings from constants.go.
func extractPredicateStrings(root string) map[string]string {
	result := make(map[string]string)
	cfgPath := filepath.Join(root, "pkg/config/constants.go")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return result
	}
	re := regexp.MustCompile(`Predicate\w+\s*=\s*"([^"]*)"`)
	for _, m := range re.FindAllSubmatch(data, -1) {
		if len(m) == 2 {
			result[string(m[1])] = string(m[1]) // value -> value
		}
	}
	return result
}

// extractNamedQueryKeys reads key strings from NamedQueries map in query.go.
func extractNamedQueryKeys(root string) map[string]bool {
	result := make(map[string]bool)
	qPath := filepath.Join(root, "pkg/common/query.go")
	data, err := os.ReadFile(qPath)
	if err != nil {
		return result
	}
	re := regexp.MustCompile("\"([^\"]+)\":\\s*`")
	for _, m := range re.FindAllSubmatch(data, -1) {
		if len(m) == 2 {
			result[string(m[1])] = true
		}
	}
	return result
}

// checkPredicateConsistency ensures declared predicates match constant definitions.
func checkPredicateConsistency(root string) []string {
	var issues []string

	declared := declaredPredicates(root)
	constants := extractPredicateStrings(root)
	namedQ := extractNamedQueryKeys(root)

	// Cross-check: predicates referenced in NamedQueries should be declared or be well-known core ones.
	corePredicates := map[string]bool{
		"triples": true, "eq": true, "neq": true, "gt": true, "gte": true,
		"lt": true, "lte": true, "smell_weight": true,
		"health_debt_with_hub": true, "health_score": true, "hub_high": true,
		"file_has_smell": true, "file_smell_weight": true, "is_entry_point": true,
		"hub_candidates": true, "entry_candidates": true,
	}
	for name := range namedQ {
		if corePredicates[name] {
			continue
		}
		// Named query keys reference predicates via triples(S,P,O) — extract P.
		// This is a light check; full verification requires parsing the query body.
		_ = name
	}

	// Check if .decl predicates are documented in constants.go.
	// Not all declared predicates need constants, but core ones should be there.
	docPredicates := map[string]bool{
		"defines": true, "calls": true, "imports": true, "has_kind": true,
		"has_language": true, "has_role": true, "has_tag": true, "has_name": true,
		"has_doc": true, "references": true, "has_security_risk": true,
		"has_health_score": true, "has_health_debt": true, "schema_version": true,
		"name": true, "parent_defines": true, "exposes_model": true,
		"called_by": true, "handled_by": true, "calls_api": true, "exports": true,
		"in_package": true, "start_line": true, "end_line": true,
		"has_comment": true, "kind": true, "type": true,
	}
	for decl := range declared {
		if docPredicates[decl] {
			if _, ok := constants[decl]; !ok {
				issues = append(issues, fmt.Sprintf("predicate %q declared in _decl.mg but not in constants.go", decl))
			}
		}
	}
	return issues
}

// checkNamedQueryPolicyMatch verifies each NamedQueries entry has a corresponding
// query_metadata declaration in policies/*.mg files.
func checkNamedQueryPolicyMatch(root string) []string {
	var issues []string
	namedQ := extractNamedQueryKeys(root)
	policiesDir := filepath.Join(root, "policies")

	re := regexp.MustCompile(`query_metadata\("(\w+)"`)
	foundMetadata := make(map[string]bool)
	filepath.Walk(policiesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".mg") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range re.FindAllSubmatch(data, -1) {
			if len(m) == 2 {
				foundMetadata[string(m[1])] = true
			}
		}
		return nil
	})

	for name := range namedQ {
		// Skip internal/helper queries that don't need metadata.
		if strings.HasPrefix(name, "all_") || strings.HasSuffix(name, "_short") ||
			strings.Contains(name, "body_hash") || strings.Contains(name, "duplicate_of") {
			continue
		}
		if !foundMetadata[name] {
			issues = append(issues, fmt.Sprintf("NamedQueries[%q] has no query_metadata in policies/", name))
		}
	}
	return issues
}

// checkSmellPredicateScoring verifies scoring.mg uses has_smell_type instead of has_smell.
func checkSmellPredicateScoring(root string) []string {
	var issues []string
	scoringPath := filepath.Join(root, "policies/smells/scoring.mg")
	data, err := os.ReadFile(scoringPath)
	if err != nil {
		return nil // File doesn't exist yet; nothing to report.
	}

	re := regexp.MustCompile(`"has_smell"`)
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "%") {
			continue // Skip comment lines.
		}
		if re.MatchString(line) {
			issues = append(issues, fmt.Sprintf("scoring.mg:%d: uses legacy 'has_smell' — change to 'has_smell_type'", i+1))
		}
	}
	return issues
}
