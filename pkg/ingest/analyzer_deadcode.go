package ingest

import (
	"context"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// detectDeadCode finds functions/methods that are never called and writes
// has_smell_type=dead_code facts to the Analytical Store.
//
// Rationale: the smell template engine executes via mebpkg.Query →
// datalog.Parse, which supports triples + neq/eq/comparison constraints but
// NOT negation (you cannot express "symbol with no incoming calls"). Dead-code
// therefore cannot be a pure .mg policy; it is computed here as a post-ingest
// pass (mirroring writeDegreeFacts), consistent with how god_file is detected
// via precomputed facts.
func (a *Analyzer) detectDeadCode(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return err
	}
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return err
	}

	// Collect symbols that appear as the OBJECT of a calls fact (i.e. they are
	// called by something). Methods on types are resolved to full IDs, so this
	// matches the defines subject form.
	hasCaller := make(map[string]bool)
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateCalls, "") {
		if obj, ok := fact.Object.(string); ok {
			hasCaller[obj] = true
		}
	}

	// Collect public API / exported symbols to exclude from dead-code.
	public := make(map[string]bool)
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateHasRole, config.RoleAPIHandler) {
		public[fact.Subject] = true
	}
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateExports, "") {
		public[fact.Subject] = true
	}
	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateHasTag, "") {
		public[fact.Subject] = true // conservative: any tagged symbol is intentionally wired
	}

	deadCount := 0
	seen := make(map[string]bool)

	for fact := range sourceStore.ScanContext(ctx, "", config.PredicateDefines, "") {
		sym, ok := fact.Object.(string)
		if !ok || sym == "" {
			continue
		}
		if fact.Subject == "" {
			continue // scanner sentinel
		}
		if seen[sym] {
			continue
		}

		// Only functions and methods.
		if !a.symbolIsFunction(ctx, sourceStore, sym) {
			continue
		}

		// Entry points and intentional symbols are not dead.
		if a.symbolIsEntryPoint(ctx, sourceStore, sym) {
			seen[sym] = true
			continue
		}
		if public[sym] {
			seen[sym] = true
			continue
		}
		// Ignore test helpers.
		if strings.Contains(sym, "_test") {
			seen[sym] = true
			continue
		}

		seen[sym] = true
		if hasCaller[sym] {
			continue
		}

		emitFacts := []meb.Fact{
			{Subject: fact.Subject, Predicate: "has_smell_type", Object: "dead_code"},
			{Subject: fact.Subject, Predicate: "has_smell_category", Object: "smell"},
			{Subject: fact.Subject, Predicate: "has_smell_severity", Object: "low"},
		}
		for _, f := range emitFacts {
			if err := analyticalStore.AddFact(f); err != nil {
				logger.Warn("Failed to write dead_code fact", "subject", fact.Subject, "error", err)
			}
		}
		deadCount++
	}

	logger.Info("Dead-code detection complete", "dead_functions", deadCount)
	return nil
}

// symbolIsFunction reports whether sym has_kind equal to func or method.
func (a *Analyzer) symbolIsFunction(ctx context.Context, store *meb.MEBStore, sym string) bool {
	for fact := range store.ScanContext(ctx, sym, config.PredicateHasKind, "") {
		if fact.Subject == "" {
			continue // skip scanner sentinel
		}
		if k, ok := fact.Object.(string); ok {
			if k == config.SymbolKindFunc || k == config.SymbolKindMethod {
				return true
			}
		}
	}
	return false
}

// symbolIsEntryPoint reports whether sym is flagged as an entry point on any
// file that defines it.
func (a *Analyzer) symbolIsEntryPoint(ctx context.Context, store *meb.MEBStore, sym string) bool {
	for fact := range store.ScanContext(ctx, "", config.PredicateDefines, sym) {
		if fact.Subject == "" {
			continue
		}
		for ep := range store.ScanContext(ctx, fact.Subject, "is_entry_point", "true") {
			if ep.Subject != "" {
				return true
			}
		}
	}
	return false
}