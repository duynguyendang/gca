package ingest

import (
	"context"

	"github.com/duynguyendang/gca/pkg/compliance"
	"github.com/duynguyendang/gca/pkg/logger"
)

// runComplianceMatch performs the F4 offline dependency vulnerability scan:
// collects the SBOM inventory from the source store and writes
// has_vulnerability / vuln_severity / vuln_summary facts to the analytical
// store for packages present in the offline advisory snapshot. It runs after
// smells so the vulnerable_dependency smell template can fire in the same
// cycle. Missing snapshots are treated as a no-op (no network egress at runtime).
func (a *Analyzer) runComplianceMatch(ctx context.Context, projectID string) {
	source, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		logger.Warn("Compliance: source store unavailable", "error", err)
		return
	}
	analytical, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		logger.Warn("Compliance: analytical store unavailable", "error", err)
		return
	}

	snap, err := compliance.LoadSnapshot(compliance.DefaultSnapshotPath)
	if err != nil {
		logger.Warn("Compliance: no advisory snapshot, skipping", "path", compliance.DefaultSnapshotPath, "error", err)
		return
	}

	if _, err := compliance.Match(ctx, source, analytical, snap); err != nil {
		logger.Warn("Compliance matching failed", "error", err)
		return
	}
	logger.Info("Compliance scan complete", "project", projectID)
}
