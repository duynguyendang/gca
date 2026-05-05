package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/duynguyendang/gca/pkg/llmconfig"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

// EmbeddingService handles interactions with the embedding model.
type EmbeddingService struct {
	g              *genkit.Genkit
	embeddingModel string
}

// NewEmbeddingService creates a new service instance.
func NewEmbeddingService(ctx context.Context) (*EmbeddingService, error) {
	cfg, err := llmconfig.LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load LLM config: %w", err)
	}

	g := genkit.Init(ctx, genkit.WithPlugins(cfg.Plugins...))

	return &EmbeddingService{
		g:              g,
		embeddingModel: cfg.EmbeddingModel,
	}, nil
}

// Close cleans up resources.
func (s *EmbeddingService) Close() {
}

// GetEmbedding generates a vector for the given text.
func (s *EmbeddingService) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text for embedding")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := genkit.Embed(ctx, s.g,
		ai.WithEmbedderName(s.embeddingModel),
		ai.WithTextDocs(text),
	)
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0].Embedding) == 0 {
		return nil, fmt.Errorf("no embedding values returned")
	}

	values := resp.Embeddings[0].Embedding
	result := make([]float32, len(values))
	for i, v := range values {
		result[i] = float32(v)
	}
	return result, nil
}
