package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsStdLibCall_Go(t *testing.T) {
	tests := []struct {
		name   string
		callee string
		want   bool
	}{
		{"fmt.Println", "fmt.Println", true},
		{"fmt.Printf", "fmt.Printf", true},
		{"log.Print", "log.Print", true},
		{"os.Open", "os.Open", true},
		{"strings.Split", "strings.Split", true},
		{"strconv.Atoi", "strconv.Atoi", true},
		{"time.Now", "time.Now", true},
		{"errors.New", "errors.New", true},
		{"context.Background", "context.Background", true},
		{"panic", "panic", true},
		{"append", "append", true},
		{"len", "len", true},
		{"custom.Package", "custom.Package", false},
		{"myFunc", "myFunc", false},
		{"unknownpkg.DoSomething", "unknownpkg.DoSomething", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStdLibCall(tt.callee, "go")
			if got != tt.want {
				t.Errorf("isStdLibCall(%q, go) = %v, want %v", tt.callee, got, tt.want)
			}
		})
	}
}

func TestIsStdLibCall_Python(t *testing.T) {
	tests := []struct {
		name   string
		callee string
		want   bool
	}{
		{"print", "print", true},
		{"len", "len", true},
		{"str", "str", true},
		{"list", "list", true},
		{"open", "open", true},
		{"enumerate", "enumerate", true},
		{"map", "map", true},
		{"custom_func", "custom_func", false},
		{"myClass.method", "myClass.method", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStdLibCall(tt.callee, "python")
			if got != tt.want {
				t.Errorf("isStdLibCall(%q, python) = %v, want %v", tt.callee, got, tt.want)
			}
		})
	}
}

func TestIsStdLibCall_JavaScript(t *testing.T) {
	tests := []struct {
		name   string
		callee string
		want   bool
	}{
		{"console.log", "console.log", true},
		{"Math.abs", "Math.abs", true},
		{"JSON.parse", "JSON.parse", true},
		{"Intl.DateTimeFormat", "Intl.DateTimeFormat", true},
		{"window", "window", true},
		{"document", "document", true},
		{"navigator", "navigator", true},
		{"location", "location", true},
		{"fetch", "fetch", true},
		{"Promise", "Promise", true},
		{"process", "process", true},
		{"setTimeout", "setTimeout", true},
		{"setInterval", "setInterval", true},
		{"myFunc", "myFunc", false},
		{"myModule.doSomething", "myModule.doSomething", false},
		{"window.location", "window.location", false}, // not exact match
		{"document.getElementById", "document.getElementById", false}, // not exact match
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStdLibCall(tt.callee, "js")
			if got != tt.want {
				t.Errorf("isStdLibCall(%q, js) = %v, want %v", tt.callee, got, tt.want)
			}
		})
	}
}

func TestIsStdLibCall_UnknownLanguage(t *testing.T) {
	got := isStdLibCall("somefunction", "unknown")
	if got != false {
		t.Errorf("isStdLibCall with unknown language should return false")
	}
}

func TestLoadProjectMetadata_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "project.yaml")

	yamlContent := `
name: test-project
description: A test project
version: 1.0.0
tags:
  - go
  - api
components:
  server:
    type: http
    language: go
    path: ./server
`
	if err := os.WriteFile(metadataPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := LoadProjectMetadata(metadataPath)
	if err != nil {
		t.Fatalf("LoadProjectMetadata failed: %v", err)
	}

	if metadata.Name != "test-project" {
		t.Errorf("Name = %q, want %q", metadata.Name, "test-project")
	}
	if metadata.Description != "A test project" {
		t.Errorf("Description = %q, want %q", metadata.Description, "A test project")
	}
	if metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", metadata.Version, "1.0.0")
	}
	if len(metadata.Tags) != 2 {
		t.Errorf("Tags length = %d, want %d", len(metadata.Tags), 2)
	}
	if metadata.Components["server"].Type != "http" {
		t.Errorf("Components[server].Type = %q, want %q", metadata.Components["server"].Type, "http")
	}
}

func TestLoadProjectMetadata_FileNotFound(t *testing.T) {
	_, err := LoadProjectMetadata("/nonexistent/path/project.yaml")
	if err == nil {
		t.Error("LoadProjectMetadata should fail for nonexistent file")
	}
}

func TestLoadProjectMetadata_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "project.yaml")

	invalidContent := `
name: test
  invalid: [unterminated
`
	if err := os.WriteFile(metadataPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadProjectMetadata(metadataPath)
	if err == nil {
		t.Error("LoadProjectMetadata should fail for invalid YAML")
	}
}

func TestDocument_Structure(t *testing.T) {
	doc := Document{
		ID:      "test.go",
		Content: []byte("package main"),
		Metadata: map[string]any{
			"language": "go",
		},
	}

	if doc.ID != "test.go" {
		t.Errorf("Document.ID = %q, want %q", doc.ID, "test.go")
	}
	if string(doc.Content) != "package main" {
		t.Errorf("Document.Content = %q, want %q", string(doc.Content), "package main")
	}
	if doc.Metadata["language"] != "go" {
		t.Errorf("Document.Metadata[language] = %v, want %v", doc.Metadata["language"], "go")
	}
}

func TestAnalysisBundle_Structure(t *testing.T) {
	bundle := AnalysisBundle{
		Documents: []Document{
			{ID: "main.go", Content: []byte("package main")},
			{ID: "utils.go", Content: []byte("package util")},
		},
	}

	if len(bundle.Documents) != 2 {
		t.Errorf("AnalysisBundle.Documents length = %d, want %d", len(bundle.Documents), 2)
	}
	if bundle.Documents[0].ID != "main.go" {
		t.Errorf("Documents[0].ID = %q, want %q", bundle.Documents[0].ID, "main.go")
	}
}
