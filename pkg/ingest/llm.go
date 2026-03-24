package ingest

import (
	"context"
	"fmt"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// EmbeddingService handles interactions with the embedding model.
type EmbeddingService struct {
	client *genai.Client
	model  *genai.EmbeddingModel
}

// NewEmbeddingService creates a new service instance.
func NewEmbeddingService(ctx context.Context) (*EmbeddingService, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	model := client.EmbeddingModel("gemini-embedding-001")
	return &EmbeddingService{
		client: client,
		model:  model,
	}, nil
}

// Close cleans up resources.
func (s *EmbeddingService) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

// GetEmbedding generates a vector for the given text.
func (s *EmbeddingService) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text for embedding")
	}

	res, err := s.model.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	if res.Embedding == nil || len(res.Embedding.Values) == 0 {
		return nil, fmt.Errorf("no embedding values returned")
	}

	return res.Embedding.Values, nil
}
