package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

// ModelInterface is what the agent needs from the AI service.
type ModelInterface interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

// GeminiAdapter implements ModelInterface using Genkit.
type GeminiAdapter struct {
	g            *genkit.Genkit
	defaultModel string
}

// NewGeminiAdapter creates a new adapter from environment variables.
func NewGeminiAdapter(ctx context.Context) (*GeminiAdapter, error) {
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY not set")
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "googleai/gemini-2.5-flash"
	}

	g := genkit.Init(ctx, genkit.WithDefaultModel(model))

	return &GeminiAdapter{
		g:            g,
		defaultModel: model,
	}, nil
}

// GenerateContent sends a prompt to the LLM and returns the text response.
func (a *GeminiAdapter) GenerateContent(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := genkit.Generate(ctx, a.g,
		ai.WithModelName(a.defaultModel),
		ai.WithPrompt(prompt),
	)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}

	text := resp.Text()
	if text == "" {
		return "", fmt.Errorf("no response from LLM")
	}

	return text, nil
}
