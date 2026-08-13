package ingest

import (
	"context"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// detectDuplicates finds functions/methods with identical normalized bodies
// (same has_body_hash) and writes has_smell_type=duplicate_code facts.
func (a *Analyzer) detectDuplicates(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return err
	}
	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return err
	}

	// Group symbols by body hash.
	hashToSymbols := make(map[string][]string)
	for fact := range sourceStore.ScanContext(ctx, "", "has_body_hash", "") {
		hash, ok := fact.Object.(string)
		if !ok || hash == "" || fact.Subject == "" {
			continue
		}
		hashToSymbols[hash] = append(hashToSymbols[hash], fact.Subject)
	}

	dupCount := 0
	for hash, syms := range hashToSymbols {
		if len(syms) < 2 {
			continue // need at least 2 functions with same hash
		}

		// Emit duplicate_code smell for each file containing a duplicate function.
		filesSeen := make(map[string]bool)
		for _, sym := range syms {
			// Find the file that defines this symbol.
			for fact := range sourceStore.ScanContext(ctx, "", "defines", sym) {
				file := fact.Subject
				if file == "" {
					continue
				}
				if filesSeen[file] {
					continue
				}
				filesSeen[file] = true

				facts := []meb.Fact{
					{Subject: file, Predicate: "has_smell_type", Object: "duplicate_code"},
					{Subject: file, Predicate: "has_smell_category", Object: "smell"},
					{Subject: file, Predicate: "has_smell_severity", Object: "medium"},
				}
				for _, f := range facts {
					if err := analyticalStore.AddFact(f); err != nil {
						logger.Warn("Failed to write duplicate_code fact", "subject", file, "error", err)
					}
				}
				dupCount++
			}
		}

		// Write duplicate_of facts linking duplicate symbols.
		for i := 0; i < len(syms)-1; i++ {
			for j := i + 1; j < len(syms); j++ {
				fact := meb.Fact{
					Subject:   syms[i],
					Predicate: "has_duplicate_of",
					Object:    syms[j],
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					logger.Warn("Failed to write has_duplicate_of", "subject", syms[i], "error", err)
				}
			}
		}

		logger.Debug("Duplicate group found", "hash", hash, "symbols", len(syms))
	}

	logger.Info("Duplicate detection complete", "duplicate_files", dupCount)
	return nil
}
