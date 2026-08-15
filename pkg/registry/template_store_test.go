package registry

import (
	"context"
	"os"
	"testing"
)

// TestParseTemplateFile_RulePopulatesPredicateAndSmellType guards the fix where
// query() rule templates dropped the Predicate/SmellType metadata — without it,
// smells emitted as has_<id> instead of has_smell_type=<smell_type>.
func TestParseTemplateFile_RulePopulatesPredicateAndSmellType(t *testing.T) {
	content := `
query_metadata("smell_god_file", "category", "smell").
query_metadata("smell_god_file", "severity", "medium").
query_metadata("smell_god_file", "smell_type", "god_file").
query_metadata("smell_god_file", "Predicate", "has_smell_type").

query("smell_god_file", File, "excessive_imports") :-
    triples(File, "has_import_count", "excessive").
`
	ts := NewTemplateStore(nil)
	templates, err := ts.parseTemplateFile(content)
	if err != nil {
		t.Fatalf("parseTemplateFile failed: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}
	tmpl := templates[0]
	if tmpl.ID != "smell_god_file" {
		t.Errorf("expected ID smell_god_file, got %q", tmpl.ID)
	}
	if tmpl.Predicate != "has_smell_type" {
		t.Errorf("expected Predicate has_smell_type, got %q", tmpl.Predicate)
	}
	if tmpl.SmellType != "god_file" {
		t.Errorf("expected SmellType god_file, got %q", tmpl.SmellType)
	}
	if tmpl.Body == "" {
		t.Error("expected non-empty template body")
	}
	if tmpl.Category != "smell" || tmpl.Severity != "medium" {
		t.Errorf("metadata not carried: category=%q severity=%q", tmpl.Category, tmpl.Severity)
	}
}

// TestMultiRuleSameID guards against the collision where two query() rules with
// the same ID (e.g. god_file: has_import_count + has_define_count) collapsed to
// one body — the second rule was silently dropped by the cache map overwrite.
func TestParseTemplateFile_MultiRuleSameID(t *testing.T) {
	content := `
query_metadata("smell_god_file", "category", "smell").
query_metadata("smell_god_file", "severity", "medium").
query_metadata("smell_god_file", "smell_type", "god_file").
query_metadata("smell_god_file", "Predicate", "has_smell_type").

query("smell_god_file", File, "excessive_imports") :-
    triples(File, "has_import_count", "excessive").

query("smell_god_file", File, "excessive_defines") :-
    triples(File, "has_define_count", "excessive").
`
	ts := NewTemplateStore(nil)
	templates, err := ts.parseTemplateFile(content)
	if err != nil {
		t.Fatalf("parseTemplateFile failed: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates (one per rule body), got %d", len(templates))
	}
	var sawImports, sawDefines bool
	for _, tmpl := range templates {
		if tmpl.ID != "smell_god_file" {
			t.Errorf("expected ID smell_god_file, got %q", tmpl.ID)
		}
		switch {
		case tmpl.Body == "triples(File, \"has_import_count\", \"excessive\")":
			sawImports = true
		case tmpl.Body == "triples(File, \"has_define_count\", \"excessive\")":
			sawDefines = true
		}
	}
	if !sawImports || !sawDefines {
		t.Errorf("both rule bodies must survive: imports=%v defines=%v", sawImports, sawDefines)
	}
}

// TestLoadPolicyFiles_PreservesMultiRuleBodies verifies the in-memory cache keeps
// all bodies for an ID and ListTemplates returns all of them.
func TestLoadPolicyFiles_PreservesMultiRuleBodies(t *testing.T) {
	content := `
query_metadata("smell_god_file", "category", "smell").

query("smell_god_file", File, "excessive_imports") :-
    triples(File, "has_import_count", "excessive").

query("smell_god_file", File, "excessive_defines") :-
    triples(File, "has_define_count", "excessive").
`
	dir := t.TempDir()
	initPath := dir + "/init.mg"
	smellPath := dir + "/smell.mg"
	if err := os.WriteFile(smellPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write smell.mg: %v", err)
	}
	manifest := `
load_policy("smell.mg").
`
	if err := os.WriteFile(initPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write init.mg: %v", err)
	}

	ts := NewTemplateStore(nil)
	if err := ts.LoadPolicyFiles(context.Background(), initPath); err != nil {
		t.Fatalf("LoadPolicyFiles failed: %v", err)
	}

	tmpls, err := ts.ListTemplates(context.Background(), "", "smell")
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}
	if len(tmpls) != 2 {
		t.Fatalf("expected 2 templates from cache, got %d", len(tmpls))
	}
}
