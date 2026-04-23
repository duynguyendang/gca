package prompts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrompt(t *testing.T) {
	// Create a temporary .prompt file
	content := `---
model: gemini-1.5-flash
temperature: 0.7
---
Hello {{.Name}}, you have {{.Count}} messages.
`
	tmpDir := t.TempDir()
	promptPath := filepath.Join(tmpDir, "test.prompt")
	if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	prompt, err := LoadPrompt(promptPath)
	if err != nil {
		t.Fatalf("LoadPrompt() error = %v", err)
	}
	if prompt == nil {
		t.Fatal("LoadPrompt returned nil")
	}
	if prompt.Config.Model != "gemini-1.5-flash" {
		t.Errorf("Config.Model = %q, want %q", prompt.Config.Model, "gemini-1.5-flash")
	}
	if prompt.Config.Temperature != 0.7 {
		t.Errorf("Config.Temperature = %f, want %f", prompt.Config.Temperature, 0.7)
	}
}

func TestLoadPrompt_FileNotFound(t *testing.T) {
	_, err := LoadPrompt("/nonexistent/path/test.prompt")
	if err == nil {
		t.Error("LoadPrompt should return error for non-existent file")
	}
}

func TestLoadPrompt_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	promptPath := filepath.Join(tmpDir, "invalid.prompt")
	// Missing --- delimiters
	if err := os.WriteFile(promptPath, []byte("no frontmatter"), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, err := LoadPrompt(promptPath)
	if err == nil {
		t.Error("LoadPrompt should error on invalid format")
	}
}

func TestLoadPrompt_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	promptPath := filepath.Join(tmpDir, "invalid.prompt")
	if err := os.WriteFile(promptPath, []byte("---\ninvalid: [yaml\n---\nbody"), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, err := LoadPrompt(promptPath)
	if err == nil {
		t.Error("LoadPrompt should error on invalid YAML")
	}
}

func TestPrompt_Execute(t *testing.T) {
	content := `---
model: test
---
Hello {{.Name}}.
`
	tmpDir := t.TempDir()
	promptPath := filepath.Join(tmpDir, "test.prompt")
	if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	prompt, err := LoadPrompt(promptPath)
	if err != nil {
		t.Fatalf("LoadPrompt() error = %v", err)
	}

	result, err := prompt.Execute(map[string]string{"Name": "World"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	// Template body is trimmed, period at end is preserved
	expected := "Hello World."
	if result != expected && result != expected+"\n" {
		t.Errorf("Execute() = %q, want %q or %q", result, expected, expected+"\n")
	}
}

func TestPrompt_Execute_MissingVariableHandled(t *testing.T) {
	// Go's text/template silently ignores missing keys by default.
	// This test verifies that behavior.
	content := `---
model: test
---
Hello {{.Missing}}.
`
	tmpDir := t.TempDir()
	promptPath := filepath.Join(tmpDir, "test.prompt")
	if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	prompt, err := LoadPrompt(promptPath)
	if err != nil {
		t.Fatalf("LoadPrompt() error = %v", err)
	}

	// Execute with different data - Go's text/template outputs "<no value>" for missing keys
	result, err := prompt.Execute(map[string]string{"Other": "Data"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "Hello <no value>."
	if result != expected && result != expected+"\n" {
		t.Errorf("Execute() = %q, want %q or %q", result, expected, expected+"\n")
	}
}

func TestLoadPrompt_TrimsBodyWhitespace(t *testing.T) {
	content := `---
model: test
---
   Hello World
`
	tmpDir := t.TempDir()
	promptPath := filepath.Join(tmpDir, "test.prompt")
	if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	prompt, err := LoadPrompt(promptPath)
	if err != nil {
		t.Fatalf("LoadPrompt() error = %v", err)
	}

	// Execute with empty data to just check the template
	result, err := prompt.Execute(nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "Hello World"
	if result != expected {
		t.Errorf("Execute() = %q, want %q", result, expected)
	}
}
