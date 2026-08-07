package ingest

import (
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func newSchemaVersionStore(t *testing.T) (*meb.MEBStore, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "gca-schema-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("NewMEBStore failed: %v", err)
	}
	cleanup := func() {
		s.Close()
		os.RemoveAll(tmpDir)
	}
	return s, cleanup
}

func TestSchemaVersionRoundTrip(t *testing.T) {
	s, cleanup := newSchemaVersionStore(t)
	defer cleanup()

	if got := GetSchemaVersion(s); got != "" {
		t.Fatalf("expected empty schema version on fresh store, got %q", got)
	}

	SaveSchemaVersion(s, "1.5")
	if got := GetSchemaVersion(s); got != "1.5" {
		t.Fatalf("expected stored schema version 1.5, got %q", got)
	}

	// Overwrite with a newer version.
	SaveSchemaVersion(s, config.SchemaVersion)
	if got := GetSchemaVersion(s); got != config.SchemaVersion {
		t.Fatalf("expected schema version %q, got %q", config.SchemaVersion, got)
	}
}

func TestLastCommitSHARoundTrip(t *testing.T) {
	s, cleanup := newSchemaVersionStore(t)
	defer cleanup()

	if got := GetLastCommitSHA(s); got != "" {
		t.Fatalf("expected empty last commit SHA on fresh store, got %q", got)
	}

	SaveLastCommitSHA(s, "deadbeef")
	if got := GetLastCommitSHA(s); got != "deadbeef" {
		t.Fatalf("expected stored SHA deadbeef, got %q", got)
	}
}
