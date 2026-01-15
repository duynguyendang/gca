package repl

import (
	"os"
	"testing"
)

func TestLoadAndExecutePrompt(t *testing.T) {
	// Create a temporary prompt file
	content := `---
model: test-model
temperature: 0.5
input:
  schema:
    name: string
---
Hello {{.name}}!
`
	tmpfile, err := os.CreateTemp("", "test.prompt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test loading
	p, err := LoadPrompt(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadPrompt failed: %v", err)
	}

	if p.Config.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got '%s'", p.Config.Model)
	}
	if p.Config.Temperature != 0.5 {
		t.Errorf("Expected temperature 0.5, got %f", p.Config.Temperature)
	}

	// Test execution
	data := map[string]string{"name": "World"}
	result, err := p.Execute(data)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	expected := "Hello World!"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}
