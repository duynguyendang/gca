package okf

import (
	"testing"
)

func TestParseGCAURI_ConceptID(t *testing.T) {
	raw := "gca://project/acme/okf/tables/orders"
	u, ok := ParseGCAURI(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if u.ProjectID != "acme" {
		t.Errorf("project = %q", u.ProjectID)
	}
	if u.Kind != "okf" {
		t.Errorf("kind = %q", u.Kind)
	}
	if u.Path != "tables/orders" {
		t.Errorf("path = %q", u.Path)
	}
	if u.SymbolName != "" {
		t.Errorf("symbol = %q, want empty", u.SymbolName)
	}
	if u.String() != raw {
		t.Errorf("String() = %q, want %q", u.String(), raw)
	}
}

func TestParseGCAURI_WithFragment(t *testing.T) {
	raw := "gca://project/acme/file/src/auth.go#LoginHandler"
	u, ok := ParseGCAURI(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if u.ProjectID != "acme" {
		t.Errorf("project = %q", u.ProjectID)
	}
	if u.Kind != "file" {
		t.Errorf("kind = %q", u.Kind)
	}
	if u.Path != "src/auth.go" {
		t.Errorf("path = %q", u.Path)
	}
	if u.SymbolName != "LoginHandler" {
		t.Errorf("symbol = %q", u.SymbolName)
	}
	if u.String() != raw {
		t.Errorf("String() = %q, want %q", u.String(), raw)
	}
}

func TestParseGCAURI_NonGCA(t *testing.T) {
	_, ok := ParseGCAURI("https://example.com")
	if ok {
		t.Error("expected ok=false for non-gca:// URI")
	}
	_, ok = ParseGCAURI("gca://bad")
	if ok {
		t.Error("expected ok=false for malformed gca:// URI")
	}
}

func TestSplitPathFragment(t *testing.T) {
	tests := []struct {
		input    string
		wantPath string
		wantSym  string
		wantOK   bool
	}{
		{"src/auth.go#LoginHandler", "src/auth.go", "LoginHandler", true},
		{"/src/auth.go#LoginHandler", "src/auth.go", "LoginHandler", true},
		{"handler.go#New", "handler.go", "New", true},
		{"no_fragment.go", "", "", false},
		{"#nosymbol", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			path, sym, ok := SplitPathFragment(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if sym != tt.wantSym {
				t.Errorf("sym = %q, want %q", sym, tt.wantSym)
			}
		})
	}
}

func TestParseBQResource_APIURL(t *testing.T) {
	raw := "https://bigquery.googleapis.com/v2/projects/bigquery-public-data/datasets/ga4_obfuscated_sample_ecommerce/tables/events_*"
	r, ok := ParseBQResource(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.Project != "bigquery-public-data" {
		t.Errorf("project = %q", r.Project)
	}
	if r.Dataset != "ga4_obfuscated_sample_ecommerce" {
		t.Errorf("dataset = %q", r.Dataset)
	}
	if r.Table != "events_*" {
		t.Errorf("table = %q", r.Table)
	}
}

func TestParseBQResource_ConsoleURL(t *testing.T) {
	raw := "https://console.cloud.google.com/bigquery?p=myproj&d=myds&t=mytbl"
	r, ok := ParseBQResource(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.Project != "myproj" {
		t.Errorf("project = %q", r.Project)
	}
	if r.Dataset != "myds" {
		t.Errorf("dataset = %q", r.Dataset)
	}
	if r.Table != "mytbl" {
		t.Errorf("table = %q", r.Table)
	}
}

func TestParseBQResource_NonBQ(t *testing.T) {
	_, ok := ParseBQResource("https://example.com/foo")
	if ok {
		t.Error("expected ok=false for non-BQ URL")
	}
}

func TestIsExternalURL(t *testing.T) {
	if !IsExternalURL("https://example.com") {
		t.Error("expected true for https")
	}
	if !IsExternalURL("http://example.com") {
		t.Error("expected true for http")
	}
	if IsExternalURL("./relative.md") {
		t.Error("expected false for relative")
	}
	if IsExternalURL("/absolute/path.md") {
		t.Error("expected false for absolute")
	}
}

func TestIsBundleAbsolute(t *testing.T) {
	if !IsBundleAbsolute("/tables/orders.md") {
		t.Error("expected true")
	}
	if IsBundleAbsolute("tables/orders.md") {
		t.Error("expected false")
	}
}

func TestIsRelative(t *testing.T) {
	if !IsRelative("./other.md") {
		t.Error("expected true for ./")
	}
	if !IsRelative("../sibling/bar.md") {
		t.Error("expected true for ../")
	}
	if IsRelative("/absolute.md") {
		t.Error("expected false")
	}
}
