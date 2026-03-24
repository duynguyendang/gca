package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ModelInterface is what the agent needs from the Gemini service.
// This avoids circular imports with pkg/service/ai.
type ModelInterface interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

// GeminiAdapter implements ModelAdapter using the Google Generative AI SDK directly.
type GeminiAdapter struct {
	client *genai.Client
}

// NewGeminiAdapter creates a new adapter from the GEMINI_API_KEY env var.
func NewGeminiAdapter(ctx context.Context) (*GeminiAdapter, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	return &GeminiAdapter{client: client}, nil
}

// GenerateContent sends a prompt to Gemini and returns the text response.
func (a *GeminiAdapter) GenerateContent(ctx context.Context, prompt string) (string, error) {
	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-3-flash-preview"
	}

	model := a.client.GenerativeModel(modelName)
	model.SetTemperature(0.2)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("no response from gemini")
	}

	var text string
	for _, part := range resp.Candidates[0].Content.Parts {
		if t, ok := part.(genai.Text); ok {
			text += string(t)
		}
	}

	return text, nil
}

// GeminiModelAdapter wraps any ModelInterface for use with the agent.
// The server passes in the GeminiService which satisfies this via its existing GeminiModelAdapter.
type GeminiModelAdapter struct {
	Service ModelInterface
}

// GenerateContent delegates to the wrapped service.
func (a *GeminiModelAdapter) GenerateContent(ctx context.Context, prompt string) (string, error) {
	return a.Service.GenerateContent(ctx, prompt)
}
