package okf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExportRoundTrip(t *testing.T) {
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

	ctx := context.Background()
	_, err := Ingest(ctx, sa, t.TempDir(), IngestOptions{ProjectID: "acme", BundleDir: bundle})
	require.NoError(t, err)

	// Export to a fresh directory.
	out := t.TempDir()
	report, err := Export(ctx, sa, ExportOptions{ProjectID: "acme", OutputDir: out})
	require.NoError(t, err)
	require.Equal(t, 2, report.Concepts)
	require.Equal(t, 2, report.Files)

	// Exported bundle must re-ingest cleanly into a fresh store set.
	sa2 := newMemoryStores(t)
	report2, err := Ingest(ctx, sa2, t.TempDir(), IngestOptions{ProjectID: "acme", BundleDir: out})
	require.NoError(t, err)
	require.True(t, report2.Conformant)
	require.Equal(t, 2, report2.Concepts, "re-ingest of exported bundle should parse both concepts")
}

func TestExportNoConcepts(t *testing.T) {
	sa := newMemoryStores(t)
	ctx := context.Background()

	// Empty source store — export should not error.
	out := t.TempDir()
	report, err := Export(ctx, sa, ExportOptions{ProjectID: "acme", OutputDir: out})
	require.NoError(t, err)
	require.Equal(t, 0, report.Concepts)
	require.Equal(t, 0, report.Files)

	// Output directory should have been created.
	_, statErr := os.Stat(out)
	require.NoError(t, statErr)
}

func TestConceptSourcePath(t *testing.T) {
	require.Equal(t, "concepts/orders", conceptSourcePath("gca://project/acme/okf/concepts/orders"))
	require.Equal(t, "concepts/orders.md", conceptSourcePath("gca://project/acme/okf/concepts/orders.md"))
	require.Equal(t, "", conceptSourcePath("gca://project/acme/other"))
	require.Equal(t, "", conceptSourcePath("no-marker"))
}

func TestExportOverwritesBundleDir(t *testing.T) {
	sa := newMemoryStores(t)
	bundle := t.TempDir()
	writeBundle(t, bundle, map[string]string{
		"concepts/a.md": "---\ntype: doc\ntitle: A\n---\n# A\n",
	})
	ctx := context.Background()
	_, err := Ingest(ctx, sa, t.TempDir(), IngestOptions{ProjectID: "acme", BundleDir: bundle})
	require.NoError(t, err)

	out := t.TempDir()
	report, err := Export(ctx, sa, ExportOptions{ProjectID: "acme", OutputDir: filepath.Join(out, "nested")})
	require.NoError(t, err)
	require.Equal(t, 1, report.Files)
	_, err = os.Stat(filepath.Join(out, "nested", "concepts", "a.md"))
	require.NoError(t, err)
}
