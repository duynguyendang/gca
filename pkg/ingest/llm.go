package ingest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/firebase/genkit/go/plugins/ollama"
)

// EmbeddingService handles interactions with the embedding model.
type EmbeddingService struct {
	g              *genkit.Genkit
	embeddingModel string
}

// NewEmbeddingService creates a new service instance.
func NewEmbeddingService(ctx context.Context) (*EmbeddingService, error) {
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY not set")
	}

	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = "googleai"
	}

	var plugins []api.Plugin

	switch provider {
	case "googleai", "gemini":
		plugins = append(plugins, &googlegenai.GoogleAI{APIKey: apiKey})
	case "openai":
		plugins = append(plugins, &openai.OpenAI{APIKey: apiKey})
	case "anthropic":
		plugins = append(plugins, &anthropic.Anthropic{APIKey: apiKey})
	case "ollama":
		addr := os.Getenv("OLLAMA_ADDRESS")
		if addr == "" {
			addr = "http://localhost:11434"
		}
		plugins = append(plugins, &ollama.Ollama{ServerAddress: addr})
	default:
		plugins = append(plugins, &googlegenai.GoogleAI{APIKey: apiKey})
	}

	model := os.Getenv("EMBEDDING_MODEL")
	if model == "" {
		switch provider {
		case "googleai", "gemini":
			model = "googleai/text-embedding-004"
		case "openai":
			model = "openai/text-embedding-3-large"
		case "anthropic":
			return nil, fmt.Errorf("embedding model not supported for provider %s", provider)
		case "ollama":
			model = "ollama/nomic-embed-text"
		default:
			model = "googleai/text-embedding-004"
		}
	} else if !strings.Contains(model, "/") {
		model = provider + "/" + model
	}

	g := genkit.Init(ctx, genkit.WithPlugins(plugins...))

	return &EmbeddingService{
		g:              g,
		embeddingModel: model,
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
