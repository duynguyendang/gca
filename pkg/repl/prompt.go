package repl

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// PromptConfig holds metadata from the YAML frontmatter.
type PromptConfig struct {
	Model       string                 `yaml:"model"`
	Temperature float32                `yaml:"temperature"`
	Input       map[string]interface{} `yaml:"input"`
}

// Prompt represents a loaded prompt with config and template.
type Prompt struct {
	Config   PromptConfig
	Template *template.Template
}

// LoadPrompt reads a .prompt file, parses frontmatter and body.
func LoadPrompt(path string) (*Prompt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file: %w", err)
	}

	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid prompt format: missing frontmatter delimiters")
	}

	frontmatter := parts[1]
	body := parts[2]

	var config PromptConfig
	if err := yaml.Unmarshal([]byte(frontmatter), &config); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	tmpl, err := template.New("prompt").Parse(strings.TrimSpace(body))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template body: %w", err)
	}

	return &Prompt{
		Config:   config,
		Template: tmpl,
	}, nil
}

// Execute applies data to the template and returns the result string.
func (p *Prompt) Execute(data any) (string, error) {
	var buf bytes.Buffer
	if err := p.Template.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.String(), nil
}
