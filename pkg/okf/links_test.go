package okf

import (
	"context"
	"testing"
)

func TestLinkResolver_BundleAbsolute(t *testing.T) {
	concepts := []*Concept{
		{ID: "gca://project/test/okf/tables/orders", SourcePath: "tables/orders.md"},
		{ID: "gca://project/test/okf/tables/customers", SourcePath: "tables/customers.md"},
	}
	r := NewLinkResolver("test", nil, concepts)

	rl := r.Resolve(context.Background(), "/tables/orders.md", "")
	if rl.Target != "gca://project/test/okf/tables/orders" {
		t.Errorf("target = %q", rl.Target)
	}
	if rl.IsBridge {
		t.Error("expected no bridge for concept-to-concept link")
	}
}

func TestLinkResolver_Relative(t *testing.T) {
	concepts := []*Concept{
		{ID: "gca://project/test/okf/tables/orders", SourcePath: "tables/orders.md"},
		{ID: "gca://project/test/okf/references/joins", SourcePath: "references/joins.md"},
	}
	r := NewLinkResolver("test", nil, concepts)

	// Relative link from tables/orders.md to references/joins.md
	rl := r.Resolve(context.Background(), "../references/joins.md", "tables")
	if rl.Target != "gca://project/test/okf/references/joins" {
		t.Errorf("target = %q", rl.Target)
	}
}

func TestLinkResolver_ExternalURL(t *testing.T) {
	r := NewLinkResolver("test", nil, nil)
	rl := r.Resolve(context.Background(), "https://example.com/docs", "")
	if rl.Target != "https://example.com/docs" {
		t.Errorf("target = %q", rl.Target)
	}
	if rl.IsBridge {
		t.Error("external URL should not be a bridge")
	}
}

func TestLinkResolver_CodePathNoStore(t *testing.T) {
	// With nil store, code-path links should not bridge but should produce a gca:// URI
	r := NewLinkResolver("test", nil, nil)
	rl := r.Resolve(context.Background(), "src/auth.go#LoginHandler", "")
	if rl.Target == "" {
		t.Fatal("expected non-empty target")
	}
	if rl.IsBridge {
		t.Error("expected no bridge when store is nil")
	}
	if rl.SymbolID != "" {
		t.Error("expected empty symbol ID when store is nil")
	}
}

func TestDirOfConcept(t *testing.T) {
	if got := DirOfConcept("tables/orders.md"); got != "tables" {
		t.Errorf("DirOfConcept = %q", got)
	}
	if got := DirOfConcept("overview.md"); got != "" {
		t.Errorf("DirOfConcept(root) = %q, want empty", got)
	}
	if got := DirOfConcept("a/b/c.md"); got != "a/b" {
		t.Errorf("DirOfConcept = %q", got)
	}
}
