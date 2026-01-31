package repl

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ExtractKeywords uses Gemini to extract technical keywords from a natural language query.
func ExtractKeywords(ctx context.Context, query string) ([]string, error) {
	// Load the keyword prompt
	promptPath := "prompts/keywords.prompt"
	p, err := prompts.LoadPrompt(promptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load prompt %s: %w", promptPath, err)
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	// Create client
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(p.Config.Model)
	if p.Config.Model == "" {
		model = client.GenerativeModel("gemini-3-flash-preview")
	}
	model.SetTemperature(p.Config.Temperature)

	// Execute prompt
	data := map[string]interface{}{
		"query": query,
	}
	promptStr, err := p.Execute(data)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// Call Gemini
	resp, err := model.GenerateContent(ctx, genai.Text(promptStr))
	if err != nil {
		return nil, fmt.Errorf("gemini error: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("no response from Gemini")
	}

	var output string
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			output = string(txt)
			break
		}
	}

	// Parse comma-separated keywords
	rawParts := strings.Split(output, ",")
	var keywords []string
	for _, part := range rawParts {
		clean := strings.TrimSpace(part)
		if clean != "" {
			keywords = append(keywords, clean)
		}
	}

	return keywords, nil
}
