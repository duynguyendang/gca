package okf

import (
	"strings"
	"testing"
)

func TestConceptID(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		bundlePath string
		want       string
	}{
		{"simple", "acme", "tables/orders.md", "gca://project/acme/okf/tables/orders"},
		{"leading dot slash", "acme", "./foo/bar.md", "gca://project/acme/okf/foo/bar"},
		{"root file", "acme", "overview.md", "gca://project/acme/okf/overview"},
		{"mixed case preserved", "acme", "Tables/Orders.md", "gca://project/acme/okf/Tables/Orders"},
		{"windows separators", "acme", "tables\\orders.md", "gca://project/acme/okf/tables/orders"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConceptID(tt.projectID, tt.bundlePath)
			if got != tt.want {
				t.Errorf("ConceptID(%q, %q) = %q, want %q", tt.projectID, tt.bundlePath, got, tt.want)
			}
		})
	}
}

func TestIsReservedFile(t *testing.T) {
	if !IsReservedFile("index.md") {
		t.Error("expected index.md to be reserved")
	}
	if !IsReservedFile("log.md") {
		t.Error("expected log.md to be reserved")
	}
	if IsReservedFile("orders.md") {
		t.Error("expected orders.md to NOT be reserved")
	}
	if !IsReservedFile("tables/index.md") {
		t.Error("expected tables/index.md to be reserved (any level)")
	}
}

func TestHashContent(t *testing.T) {
	h1 := HashContent([]byte("hello"))
	h2 := HashContent([]byte("hello"))
	h3 := HashContent([]byte("world"))
	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
	if len(h1) != 64 { // sha256 hex
		t.Errorf("expected 64 hex chars, got %d", len(h1))
	}
}

func TestIsWellKnownExtensionKey(t *testing.T) {
	if !IsWellKnownExtensionKey("gca_in_degree") {
		t.Error("expected gca_in_degree to be well-known")
	}
	if !IsWellKnownExtensionKey("gca_centrality") {
		t.Error("expected gca_centrality to be well-known")
	}
	if IsWellKnownExtensionKey("some_custom_key") {
		t.Error("expected some_custom_key to NOT be well-known")
	}
}

func TestParseConcept_Basic(t *testing.T) {
	raw := []byte(`---
type: BigQuery Table
title: Orders
description: One row per completed order.
tags: [sales, orders]
timestamp: '2026-01-15T00:00:00Z'
---

# Schema

| Column | Type |
|--------|------|
| id     | STRING |

# Citations
- [API docs](https://example.com/docs)
`)

	c, err := ParseConcept("tables/orders.md", raw)
	if err != nil {
		t.Fatalf("ParseConcept failed: %v", err)
	}
	if c.Type != "BigQuery Table" {
		t.Errorf("type = %q, want %q", c.Type, "BigQuery Table")
	}
	if c.Title != "Orders" {
		t.Errorf("title = %q, want %q", c.Title, "Orders")
	}
	if c.Description != "One row per completed order." {
		t.Errorf("description = %q", c.Description)
	}
	if len(c.Tags) != 2 || c.Tags[0] != "sales" || c.Tags[1] != "orders" {
		t.Errorf("tags = %v", c.Tags)
	}
	if c.Timestamp != "2026-01-15T00:00:00Z" {
		t.Errorf("timestamp = %q", c.Timestamp)
	}
	if !strings.Contains(c.Body, "# Schema") {
		t.Error("body should contain # Schema")
	}
	if len(c.Links) != 0 {
		t.Errorf("links should be empty (citation section excluded), got %v", c.Links)
	}
	if len(c.Citations) != 1 || c.Citations[0] != "https://example.com/docs" {
		t.Errorf("citations = %v", c.Citations)
	}
	if c.ContentHash == "" {
		t.Error("content hash should be set")
	}
}

func TestParseConcept_MissingType(t *testing.T) {
	raw := []byte(`---
title: No Type
---
body
`)
	_, err := ParseConcept("test.md", raw)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error should mention type, got: %v", err)
	}
}

func TestParseConcept_UnknownKeys(t *testing.T) {
	raw := []byte(`---
type: Playbook
title: Runbook
custom_field: some value
another: 42
---
body
`)
	c, err := ParseConcept("playbooks/run.md", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Frontmatter["custom_field"] != "some value" {
		t.Errorf("custom_field = %v", c.Frontmatter["custom_field"])
	}
	if c.Frontmatter["another"] != 42 {
		t.Errorf("another = %v", c.Frontmatter["another"])
	}
}

func TestParseConcept_ExtensionKeys(t *testing.T) {
	raw := []byte(`---
type: GCA File
title: Auth
gca_in_degree: 5
gca_centrality: 0.123
gca_smells: [god_file, hub_anomaly]
my_custom: preserved
---
body
`)
	c, err := ParseConcept("auth.go.md", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Well-known keys should NOT appear in Frontmatter
	if _, ok := c.Frontmatter["gca_in_degree"]; ok {
		t.Error("gca_in_degree should not be in Frontmatter (well-known)")
	}
	// Unknown keys should be preserved
	if c.Frontmatter["my_custom"] != "preserved" {
		t.Errorf("my_custom = %v", c.Frontmatter["my_custom"])
	}
}

func TestParseRootIndexFrontmatter(t *testing.T) {
	raw := []byte(`---
okf_version: "0.1"
---
# My Bundle
`)
	v, err := ParseRootIndexFrontmatter(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "0.1" {
		t.Errorf("okf_version = %q, want %q", v, "0.1")
	}
}

func TestParseRootIndexFrontmatter_NoVersion(t *testing.T) {
	raw := []byte(`# My Bundle
Some content.
`)
	v, err := ParseRootIndexFrontmatter(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "" {
		t.Errorf("expected empty version, got %q", v)
	}
}

func TestExtractLinks(t *testing.T) {
	body := []byte(`
See the [customers table](/tables/customers.md) for the join key.
Also see [external](https://example.com).
And a [relative](./other.md) link.
`)
	links := extractLinks(body)
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d: %v", len(links), links)
	}
	if links[0] != "/tables/customers.md" {
		t.Errorf("link[0] = %q", links[0])
	}
	if links[1] != "https://example.com" {
		t.Errorf("link[1] = %q", links[1])
	}
	if links[2] != "./other.md" {
		t.Errorf("link[2] = %q", links[2])
	}
}

func TestSerializeFrontmatter(t *testing.T) {
	c := &Concept{
		Frontmatter: map[string]any{
			"custom": "value",
		},
	}
	j, err := c.SerializeFrontmatter()
	if err != nil {
		t.Fatalf("SerializeFrontmatter: %v", err)
	}
	if !strings.Contains(j, `"custom"`) || !strings.Contains(j, `"value"`) {
		t.Errorf("expected JSON with custom/value, got: %s", j)
	}
}

func TestSerializeFrontmatter_Empty(t *testing.T) {
	c := &Concept{Frontmatter: map[string]any{}}
	j, err := c.SerializeFrontmatter()
	if err != nil {
		t.Fatalf("SerializeFrontmatter: %v", err)
	}
	if j != "" {
		t.Errorf("expected empty string, got %q", j)
	}
}
