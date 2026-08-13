package ingest

import (
	"context"
	"regexp"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// dbCallPattern matches callee symbols that suggest a database or long-running
// operation that normally returns an error which must be handled.
var dbCallPattern = regexp.MustCompile(`(?i)query|exec|transaction|commit|rollback`)

// errHandlingPattern matches symbols whose presence in a file suggests it
// handles errors from its callees (error returns, validation, sanitizers).
var errHandlingPattern = regexp.MustCompile(`(?i)error|check|validate|handle`)

// detectSecuritySmells runs graph-based security smell detection as a Go-side
// pass and writes has_smell_type facts to the Analytical Store.
//
// Rationale: the smell template engine executes via mebpkg.Query over a single
// store with row-level constraints. The `calls` predicate is symbol-level, so
// "public API file reaches a database file" and "file calls a DB routine without
// handling errors" require joins (defines × calls × defines) and graph negation
// that cannot be expressed in that model — mirroring detectDeadCode
// (analyzer_deadcode.go). See docs/designs/contract.md §5.
func (a *Analyzer) detectSecuritySmells(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return err
	}
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return err
	}

	// --- Pass 1: index defines (file -> symbol, symbol -> file) and tag files ---
	fileSyms := make(map[string][]string)
	symFile := make(map[string]string)
	fileTags := make(map[string]map[string]bool)

	tagConfig := &config.ProjectTagConfig{Rules: config.DefaultTagRules()}

	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateDefines, "") {
		if fact.Subject == "" {
			continue // scanner sentinel
		}
		sym, ok := fact.Object.(string)
		if !ok || sym == "" {
			continue
		}
		if _, seen := symFile[sym]; !seen {
			symFile[sym] = fact.Subject
		}
		fileSyms[fact.Subject] = append(fileSyms[fact.Subject], sym)

		if _, ok := fileTags[fact.Subject]; !ok {
			fileTags[fact.Subject] = make(map[string]bool)
			for _, tag := range tagConfig.MatchingTags(fact.Subject) {
				fileTags[fact.Subject][tag] = true
			}
		}
	}

	// --- Pass 2: index symbol-level call edges ---
	callEdges := make(map[string][]string)
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateCalls, "") {
		if fact.Subject == "" {
			continue
		}
		if callee, ok := fact.Object.(string); ok && callee != "" {
			callEdges[fact.Subject] = append(callEdges[fact.Subject], callee)
		}
	}

	// --- Pass 3: emit smells ---
	emitted := make(map[string]bool) // subject|smellType — avoid duplicates

	// smell_unsanitized_db_access: a public_api file defines a symbol that calls
	// a symbol defined by a database file (direct symbol-level edge).
	for file, syms := range fileSyms {
		if !fileTags[file][config.TagPublicAPI] {
			continue
		}
		for _, sym := range syms {
			for _, callee := range callEdges[sym] {
				calleeFile := symFile[callee]
				if calleeFile != "" && calleeFile != file && fileTags[calleeFile][config.TagDatabase] {
					if err := emitSecuritySmell(analyticalStore, emitted, file, "unsanitized_db_access", "high"); err != nil {
						logger.Warn("Failed to write unsanitized_db_access fact", "file", file, "error", err)
					}
					break
				}
			}
		}
	}

	// smell_missing_error_check: file calls a DB-ish symbol but has no
	// error-handling symbol or callee anywhere in the file.
	fileHasErrHandling := make(map[string]bool)
	markErrHandling := func(file, sym string) {
		if errHandlingPattern.MatchString(sym) {
			fileHasErrHandling[file] = true
		}
	}
	for file, syms := range fileSyms {
		for _, sym := range syms {
			markErrHandling(file, sym)
			for _, callee := range callEdges[sym] {
				markErrHandling(file, callee)
			}
		}
	}
	for file, syms := range fileSyms {
		if fileHasErrHandling[file] {
			continue
		}
		hasDBLikeCall := false
		for _, sym := range syms {
			for _, callee := range callEdges[sym] {
				if dbCallPattern.MatchString(callee) {
					hasDBLikeCall = true
					break
				}
			}
			if hasDBLikeCall {
				break
			}
		}
		if hasDBLikeCall {
			if err := emitSecuritySmell(analyticalStore, emitted, file, "missing_error_check", "high"); err != nil {
				logger.Warn("Failed to write missing_error_check fact", "file", file, "error", err)
			}
		}
	}

	logger.Info("Security smell detection complete", "facts", len(emitted), "project", projectID)
	return nil
}

// emitSecuritySmell writes the standard smell fact trio, deduplicated per subject+type.
func emitSecuritySmell(store *meb.MEBStore, emitted map[string]bool, subject, smellType, severity string) error {
	key := subject + "|" + smellType
	if emitted[key] {
		return nil
	}
	emitted[key] = true

	facts := []meb.Fact{
		{Subject: subject, Predicate: "has_smell_type", Object: smellType},
		{Subject: subject, Predicate: "has_smell_category", Object: "security"},
		{Subject: subject, Predicate: "has_smell_severity", Object: severity},
	}
	for _, f := range facts {
		if err := store.AddFact(f); err != nil {
			return err
		}
	}
	return nil
}
