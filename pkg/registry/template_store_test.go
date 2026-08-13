package registry

import "testing"

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
