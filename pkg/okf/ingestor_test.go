package okf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	"github.com/stretchr/testify/require"
)

// memoryStoreAccessor supplies in-memory source/analytical stores.
type memoryStoreAccessor struct {
	source     *meb.MEBStore
	analytical *meb.MEBStore
}

func (a *memoryStoreAccessor) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	return a.source, nil
}

func (a *memoryStoreAccessor) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return a.analytical, nil
}

func newMemoryStores(t *testing.T) *memoryStoreAccessor {
	t.Helper()
	mk := func() *meb.MEBStore {
		cfg := store.DefaultConfig("")
		cfg.InMemory = true
		s, err := meb.NewMEBStore(cfg)
		require.NoError(t, err)
		return s
	}
	return &memoryStoreAccessor{source: mk(), analytical: mk()}
}

func writeBundle(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
}

func TestIngestBasic(t *testing.T) {
	sa := newMemoryStores(t)
	bundle := t.TempDir()
	writeBundle(t, bundle, map[string]string{
		"index.md": "---\nokf_version: 0.1.0\n---\n# Root\n",
		"concepts/orders.md": `---
type: table
title: Orders
description: Orders dataset
tags: [sales]
---
# Orders

See [items](/concepts/items.md).
`,
		"concepts/items.md": "---\ntype: table\ntitle: Items\ndescription: Items dataset\n---\n# Items\n",
	})

	report, err := Ingest(context.Background(), sa, t.TempDir(), IngestOptions{
		ProjectID: "acme",
		BundleDir: bundle,
	})
	require.NoError(t, err)
	require.Equal(t, 2, report.Concepts)
	require.Equal(t, 1, report.Links)
	require.True(t, report.Conformant)

	// Verify a concept fact landed in the source store.
	conceptID := ConceptID("acme", "concepts/orders.md")
	found := false
	for fact := range sa.source.Scan("", "okf_title", "") {
		if fact.Subject == conceptID && fact.Object == "Orders" {
			found = true
		}
	}
	require.True(t, found, "expected okf_title fact for orders concept")

	// Cross-link resolved to a concept ID.
	linkFound := false
	for fact := range sa.source.Scan("", "okf_link", "") {
		if fact.Subject == conceptID && fact.Object == ConceptID("acme", "concepts/items.md") {
			linkFound = true
		}
	}
	require.True(t, linkFound, "expected okf_link to items concept")
}

func TestIngestConformanceError(t *testing.T) {
	sa := newMemoryStores(t)
	bundle := t.TempDir()
	writeBundle(t, bundle, map[string]string{
		"good.md":       "---\ntype: table\ntitle: Good\n---\n# Good\n",
		"bad.md":        "# No frontmatter type\n",
		"badfront.md":   "---\nnot: yaml: [\n---\nbody",
	})

	report, err := Ingest(context.Background(), sa, t.TempDir(), IngestOptions{
		ProjectID: "acme",
		BundleDir: bundle,
	})
	require.NoError(t, err)
	require.False(t, report.Conformant)
	require.GreaterOrEqual(t, len(report.Errors), 1)
	require.Equal(t, 1, report.Concepts) // only good.md parsed
}

func TestIngestReingestDedup(t *testing.T) {
	sa := newMemoryStores(t)
	bundle := t.TempDir()
	body := "---\ntype: table\ntitle: Orders\ndescription: Orders\n---\n# Orders\n"
	writeBundle(t, bundle, map[string]string{"concepts/orders.md": body})

	_, err := Ingest(context.Background(), sa, t.TempDir(), IngestOptions{
		ProjectID: "acme",
		BundleDir: bundle,
	})
	require.NoError(t, err)

	countFacts := func(pred string) int {
		n := 0
		for fact := range sa.source.Scan("", pred, "") {
			if fact.Subject != "" {
				n++
			}
		}
		return n
	}
	first := countFacts("okf_title")

	// Re-ingest identical bundle — no duplicate facts.
	_, err = Ingest(context.Background(), sa, t.TempDir(), IngestOptions{
		ProjectID: "acme",
		BundleDir: bundle,
	})
	require.NoError(t, err)
	require.Equal(t, first, countFacts("okf_title"), "re-ingest should not duplicate facts")
}

func TestIngestDryRun(t *testing.T) {
	sa := newMemoryStores(t)
	bundle := t.TempDir()
	writeBundle(t, bundle, map[string]string{
		"concepts/orders.md": "---\ntype: table\ntitle: Orders\n---\n# Orders\n",
	})

	report, err := Ingest(context.Background(), sa, t.TempDir(), IngestOptions{
		ProjectID: "acme",
		BundleDir: bundle,
		DryRun:    true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, report.Concepts)

	// Dry run must not write any facts.
	n := 0
	for fact := range sa.source.Scan("", "okf_title", "") {
		if fact.Subject != "" {
			n++
		}
	}
	require.Equal(t, 0, n)
}

func TestIngestMissingBundle(t *testing.T) {
	sa := newMemoryStores(t)
	_, err := Ingest(context.Background(), sa, t.TempDir(), IngestOptions{
		ProjectID: "acme",
		BundleDir: "/nonexistent/bundle",
	})
	require.Error(t, err)
}