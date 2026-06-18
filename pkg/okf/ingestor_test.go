package okf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

// mockStoreAccessor implements StoreAccessor with two in-memory stores.
type mockStoreAccessor struct {
	source     *meb.MEBStore
	analytical *meb.MEBStore
}

func (m *mockStoreAccessor) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	return m.source, nil
}

func (m *mockStoreAccessor) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return m.analytical, nil
}

func newTestStores(t *testing.T) *mockStoreAccessor {
	t.Helper()
	cfg := &store.Config{
		InMemory:       true,
		BlockCacheSize: 8 << 20, // 8MB for tests
		IndexCacheSize: 2 << 20, // 2MB for tests
		LRUCacheSize:   10000,
	}
	src, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatalf("create source store: %v", err)
	}
	anal, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatalf("create analytical store: %v", err)
	}
	return &mockStoreAccessor{source: src, analytical: anal}
}

func TestIngest_SampleBundle(t *testing.T) {
	sa := newTestStores(t)
	ctx := context.Background()

	bundleDir := filepath.Join("devtools", "fixtures", "okf_sample")
	if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
		t.Skip("test fixture not found, skipping")
	}

	report, err := Ingest(ctx, sa, t.TempDir(), IngestOptions{
		ProjectID: "test",
		BundleDir: bundleDir,
	})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if report.Concepts != 3 {
		t.Errorf("concepts = %d, want 3", report.Concepts)
	}
	if !report.Conformant {
		t.Error("expected conformant=true")
	}
	if len(report.Errors) > 0 {
		for _, e := range report.Errors {
			t.Logf("error: %s", e)
		}
	}

	// Verify okf_concept facts exist
	conceptCount := 0
	for fact := range sa.source.ScanContext(ctx, "", string(PredOKFConcept), "") {
		conceptCount++
		if fact.Subject == "" {
			t.Error("empty subject for okf_concept")
		}
	}
	if conceptCount != 3 {
		t.Errorf("okf_concept facts = %d, want 3", conceptCount)
	}

	// Verify okf_link facts exist (orders.md links to customers.md and sales.md)
	linkCount := 0
	for fact := range sa.source.ScanContext(ctx, "", string(PredOKFLink), "") {
		linkCount++
		if fact.Subject == "" {
			t.Error("empty subject for okf_link")
		}
	}
	if linkCount < 2 {
		t.Errorf("okf_link facts = %d, want at least 2", linkCount)
	}

	// Verify has_role = okf_concept
	roleCount := 0
	for fact := range sa.source.ScanContext(ctx, "", "has_role", config.RoleOKFConcept) {
		roleCount++
		_ = fact
	}
	if roleCount != 3 {
		t.Errorf("has_role(okf_concept) = %d, want 3", roleCount)
	}

	// Verify okf_version from root index.md
	for fact := range sa.source.ScanContext(ctx, OKFVersionSubject, string(PredOKFVersion), "") {
		if v, ok := fact.Object.(string); ok {
			if v != "0.1" {
				t.Errorf("okf_version = %q, want %q", v, "0.1")
			}
		}
	}
}

func TestIngest_EmptyBundle(t *testing.T) {
	sa := newTestStores(t)
	ctx := context.Background()
	tmpDir := t.TempDir()
	// Create an empty bundle directory
	os.MkdirAll(filepath.Join(tmpDir, "empty"), 0o755)

	report, err := Ingest(ctx, sa, tmpDir, IngestOptions{
		ProjectID: "test",
		BundleDir: filepath.Join(tmpDir, "empty"),
	})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if report.Concepts != 0 {
		t.Errorf("concepts = %d, want 0", report.Concepts)
	}
}

func TestIngest_NonConformantFile(t *testing.T) {
	sa := newTestStores(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Write a file missing 'type'
	os.WriteFile(filepath.Join(tmpDir, "bad.md"), []byte(`---
title: Bad File
---
body
`), 0o644)

	// Write a valid file
	os.WriteFile(filepath.Join(tmpDir, "good.md"), []byte(`---
type: Playbook
title: Good
---
body
`), 0o644)

	report, err := Ingest(ctx, sa, tmpDir, IngestOptions{
		ProjectID: "test",
		BundleDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if report.Conformant {
		t.Error("expected conformant=false due to bad.md")
	}
	if report.Concepts != 1 {
		t.Errorf("concepts = %d, want 1 (only good.md)", report.Concepts)
	}
	if len(report.Errors) != 1 {
		t.Errorf("errors = %d, want 1", len(report.Errors))
	}
}

func TestIngest_DryRun(t *testing.T) {
	sa := newTestStores(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(`---
type: Playbook
title: Test
---
body
`), 0o644)

	report, err := Ingest(ctx, sa, tmpDir, IngestOptions{
		ProjectID: "test",
		BundleDir: tmpDir,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if report.Concepts != 1 {
		t.Errorf("concepts = %d, want 1", report.Concepts)
	}

	// Verify no facts were written (dry run)
	count := 0
	for range sa.source.ScanContext(ctx, "", "", "") {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 facts in dry run, got %d", count)
	}
}

func TestIngest_ContentHashDedup(t *testing.T) {
	sa := newTestStores(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(`---
type: Playbook
title: Test
---
body
`), 0o644)

	// First ingest
	r1, err := Ingest(ctx, sa, tmpDir, IngestOptions{ProjectID: "p", BundleDir: tmpDir})
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	// Second ingest with same content — should still succeed
	r2, err := Ingest(ctx, sa, tmpDir, IngestOptions{ProjectID: "p", BundleDir: tmpDir})
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}

	// Both should report 1 concept
	if r1.Concepts != 1 || r2.Concepts != 1 {
		t.Errorf("r1.concepts=%d, r2.concepts=%d, want 1", r1.Concepts, r2.Concepts)
	}
}

func TestIngest_BridgesToAnalyticalStore(t *testing.T) {
	sa := newTestStores(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create a source store fact that a code-path link can resolve against
	// Simulate: src/auth.go defines LoginHandler
	sa.source.AddFact(meb.Fact{Subject: "src/auth.go", Predicate: "defines", Object: "src/auth.go:LoginHandler"})
	sa.source.AddFact(meb.Fact{Subject: "src/auth.go:LoginHandler", Predicate: "has_name", Object: "LoginHandler"})
	sa.source.AddFact(meb.Fact{Subject: "src/auth.go:LoginHandler", Predicate: "has_kind", Object: "func"})
	sa.source.AddFact(meb.Fact{Subject: "src/auth.go", Predicate: "has_kind", Object: "file"})

	// Write an OKF concept with a code-path link
	os.WriteFile(filepath.Join(tmpDir, "auth.md"), []byte(`---
type: Playbook
title: Auth Runbook
description: Login flow documentation
---

See [LoginHandler](src/auth.go#LoginHandler) for the implementation.
`), 0o644)

	report, err := Ingest(ctx, sa, tmpDir, IngestOptions{ProjectID: "p", BundleDir: tmpDir})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if report.Bridges != 1 {
		t.Errorf("bridges = %d, want 1", report.Bridges)
	}

	// Verify bridges_to fact in Analytical Store
	bridgeCount := 0
	for fact := range sa.analytical.ScanContext(ctx, "", string(PredBridgesTo), "") {
		bridgeCount++
		if fact.Subject == "" {
			t.Error("empty subject for bridges_to")
		}
	}
	if bridgeCount != 1 {
		t.Errorf("bridges_to facts = %d, want 1", bridgeCount)
	}
}
