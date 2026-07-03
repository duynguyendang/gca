package common

import (
	"testing"
)

func TestExtractBaseName(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "simple file", path: "foo.go", want: "foo.go"},
		{name: "nested path", path: "pkg/server/handlers.go", want: "handlers.go"},
		{name: "trailing slash", path: "pkg/server/", want: ""},
		{name: "empty string", path: "", want: ""},
		{name: "just slash", path: "/", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBaseName(tt.path)
			if got != tt.want {
				t.Errorf("ExtractBaseName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestQuotePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "simple path", path: "foo.go", want: "\"foo.go\""},
		{name: "nested path", path: "pkg/server/handlers.go", want: "\"pkg/server/handlers.go\""},
		{name: "empty string", path: "", want: "\"\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuotePath(tt.path)
			if got != tt.want {
				t.Errorf("QuotePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMakeLinkKey(t *testing.T) {
	got := MakeLinkKey("A", "B")
	want := "A->B"
	if got != want {
		t.Errorf("MakeLinkKey(A, B) = %q, want %q", got, want)
	}
}

func TestExtractSymbolName(t *testing.T) {
	tests := []struct {
		name     string
		symbolID string
		want     string
	}{
		{name: "full path", symbolID: "pkg/server/handlers.go:HandleFunc", want: "HandleFunc"},
		{name: "method with receiver", symbolID: "handlers.go:Server.Listen", want: "Listen"},
		{name: "simple name", symbolID: "main", want: "main"},
		{name: "empty string", symbolID: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSymbolName(tt.symbolID)
			if got != tt.want {
				t.Errorf("ExtractSymbolName(%q) = %q, want %q", tt.symbolID, got, tt.want)
			}
		})
	}
}

func TestExtractSymbolName_DotSeparated(t *testing.T) {
	// When there's a dot in the name, it should extract the last part
	result := ExtractSymbolName("foo.go:Bar.Baz")
	if result != "Baz" {
		t.Errorf("ExtractSymbolName with dot = %q, want %q", result, "Baz")
	}
}
