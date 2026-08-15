// Package compliance implements offline dependency vulnerability scanning (F4):
// an SBOM inventory of a project's imports plus an offline OSV-style advisory
// snapshot. Matching is deterministic and requires no network egress at runtime,
// making it Cloud Run-friendly.
package compliance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Advisory is a single vulnerability record for a package.
type Advisory struct {
	ID               string `json:"id"`
	Summary          string `json:"summary"`
	Severity         string `json:"severity"`
	AffectedVersions string `json:"affected_versions,omitempty"`
}

// Snapshot is the versioned offline advisory file.
type Snapshot struct {
	Generated time.Time             `json:"generated"`
	Packages  map[string][]Advisory `json:"packages"`
}

// DefaultSnapshotPath is the offline advisory file used when none is configured.
// It is meant to be gitignored-ish: refreshed by `gca advisories update`.
const DefaultSnapshotPath = "data/advisories/osv-snapshot.json"

// LoadSnapshot reads a compliance snapshot from disk.
func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", path, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot %s: %w", path, err)
	}
	if snap.Packages == nil {
		snap.Packages = map[string][]Advisory{}
	}
	return &snap, nil
}

// SaveSnapshot writes a snapshot to disk, creating parent directories.
func SaveSnapshot(path string, snap *Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}
	if snap.Generated.IsZero() {
		snap.Generated = time.Now().UTC()
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write snapshot %s: %w", path, err)
	}
	return nil
}

// Lookup returns advisories for a package (canonicalized), or nil.
func (s *Snapshot) Lookup(pkg string) []Advisory {
	return s.Packages[pkg]
}

// SnapshotDate returns the snapshot's generated timestamp as RFC3339 (or "").
func (s *Snapshot) SnapshotDate() string {
	if s.Generated.IsZero() {
		return ""
	}
	return s.Generated.Format(time.RFC3339)
}
