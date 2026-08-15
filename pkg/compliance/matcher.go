package compliance

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// Compliance predicates written to the Analytical Store by the matcher.
// They are cleared and recomputed each analysis cycle like other analytical
// facts (see Analyzer.clearAnalyticalData).
const (
	PredicateHasVulnerability = "has_vulnerability" // (Package, AdvisoryID)
	PredicateVulnSeverity     = "vuln_severity"     // (AdvisoryID, Severity)
	PredicateVulnSummary      = "vuln_summary"      // (AdvisoryID, Summary)
)

// Vulnerability is one matched vulnerability fact.
type Vulnerability struct {
	Package      string `json:"package"`
	AdvisoryID   string `json:"advisory_id"`
	Severity     string `json:"severity"`
	Summary      string `json:"summary"`
	SnapshotDate string `json:"snapshot_date"`
}

// MatchResult summarizes a matching pass.
type MatchResult struct {
	MatchedPackages int             `json:"matched_packages"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	SnapshotDate    string          `json:"snapshot_date"`
}

// Match writes has_vulnerability / vuln_severity / vuln_summary facts for every
// dependency in the inventory that appears in the offline advisory snapshot.
// It reads package facts from the analytical store only after the inventory is
// fully resolved, so the analytical partition can be cleared independently.
func Match(ctx context.Context, source, analytical *meb.MEBStore, snap *Snapshot) (*MatchResult, error) {
	if snap == nil {
		return &MatchResult{}, nil
	}

	inv, err := CollectInventory(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("collect inventory: %w", err)
	}

	result := &MatchResult{SnapshotDate: snap.SnapshotDate()}
	for _, dep := range inv.Dependencies {
		advisories := snap.Lookup(dep.Name)
		if len(advisories) == 0 {
			continue
		}
		result.MatchedPackages++
		for _, adv := range advisories {
			if err := analytical.AddFact(meb.Fact{Subject: dep.Name, Predicate: PredicateHasVulnerability, Object: adv.ID}); err != nil {
				logger.Warn("failed to write has_vulnerability fact", "package", dep.Name, "advisory", adv.ID, "error", err)
				continue
			}
			if err := analytical.AddFact(meb.Fact{Subject: adv.ID, Predicate: PredicateVulnSeverity, Object: adv.Severity}); err != nil {
				logger.Warn("failed to write vuln_severity fact", "advisory", adv.ID, "error", err)
				continue
			}
			if adv.Summary != "" {
				if err := analytical.AddFact(meb.Fact{Subject: adv.ID, Predicate: PredicateVulnSummary, Object: adv.Summary}); err != nil {
					logger.Warn("failed to write vuln_summary fact", "advisory", adv.ID, "error", err)
					continue
				}
			}
			result.Vulnerabilities = append(result.Vulnerabilities, Vulnerability{
				Package:      dep.Name,
				AdvisoryID:   adv.ID,
				Severity:     adv.Severity,
				Summary:      adv.Summary,
				SnapshotDate: snap.SnapshotDate(),
			})
		}
	}

	logger.Info("Compliance matching complete",
		"matched_packages", result.MatchedPackages,
		"vulnerabilities", len(result.Vulnerabilities))
	return result, nil
}
