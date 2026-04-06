package repl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

// ExtractKeywords uses the LLM to extract technical keywords from a natural language query.
func ExtractKeywords(ctx context.Context, g *genkit.Genkit, query string) ([]string, error) {
	promptPath := "prompts/keywords.prompt"
	p, err := prompts.LoadPrompt(promptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load prompt %s: %w", promptPath, err)
	}

	data := map[string]interface{}{
		"query": query,
	}
	promptStr, err := p.Execute(data)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := genkit.Generate(ctx, g,
		ai.WithModelName("googleai/gemini-2.5-flash"),
		ai.WithPrompt(promptStr),
	)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	output := resp.Text()
	if output == "" {
		return nil, fmt.Errorf("no response from LLM")
	}

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
