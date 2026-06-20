package llmconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/firebase/genkit/go/plugins/ollama"
)

type Config struct {
	Provider       string
	APIKey         string
	DefaultModel   string
	EmbeddingModel string
	EmbeddingDim   int
	OllamaAddress  string
	Plugins        []api.Plugin
}

func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Provider: os.Getenv("LLM_PROVIDER"),
		APIKey:   os.Getenv("LLM_API_KEY"),
		DefaultModel: os.Getenv("LLM_MODEL"),
		EmbeddingModel: os.Getenv("EMBEDDING_MODEL"),
		EmbeddingDim:   resolveEmbeddingDim(os.Getenv("EMBEDDING_MODEL")),
		OllamaAddress: os.Getenv("OLLAMA_ADDRESS"),
	}

	if cfg.Provider == "" {
		cfg.Provider = "googleai"
	}

	if cfg.APIKey == "" && cfg.Provider != "ollama" {
		return nil, fmt.Errorf("LLM_API_KEY not found")
	}

	if cfg.OllamaAddress == "" {
		cfg.OllamaAddress = "http://localhost:11434"
	}

	cfg.Plugins = createPlugins(cfg.Provider, cfg.APIKey, cfg.OllamaAddress)
	cfg.DefaultModel = resolveDefaultModel(cfg.Provider, cfg.DefaultModel)
	cfg.EmbeddingModel = resolveEmbeddingModel(cfg.Provider, cfg.EmbeddingModel)
	// Re-resolve dim now that model is fully resolved
	cfg.EmbeddingDim = resolveEmbeddingDim(cfg.EmbeddingModel)

	return cfg, nil
}

func createPlugins(provider, apiKey, ollamaAddr string) []api.Plugin {
	var plugins []api.Plugin

	switch provider {
	case "googleai", "gemini":
		plugins = append(plugins, &googlegenai.GoogleAI{APIKey: apiKey})
	case "openai":
		plugins = append(plugins, &openai.OpenAI{APIKey: apiKey})
	case "anthropic":
		plugins = append(plugins, &anthropic.Anthropic{APIKey: apiKey})
	case "ollama":
		plugins = append(plugins, &ollama.Ollama{ServerAddress: ollamaAddr})
	default:
		plugins = append(plugins, &googlegenai.GoogleAI{APIKey: apiKey})
	}

	return plugins
}

func resolveDefaultModel(provider, model string) string {
	if model != "" {
		if !strings.Contains(model, "/") {
			return provider + "/" + model
		}
		return model
	}

	switch provider {
	case "googleai", "gemini":
		return "googleai/gemini-2.5-flash"
	case "openai":
		return "openai/gpt-4o"
	case "anthropic":
		return "anthropic/claude-3-5-sonnet-20241022"
	case "ollama":
		return "ollama/llama3.2"
	default:
		return "googleai/gemini-2.5-flash"
	}
}

func resolveEmbeddingModel(provider, model string) string {
	if model != "" {
		if !strings.Contains(model, "/") {
			return provider + "/" + model
		}
		return model
	}

	switch provider {
	case "googleai", "gemini":
		return "googleai/text-embedding-004"
	case "openai":
		return "openai/text-embedding-3-large"
	case "anthropic":
		return ""
	case "ollama":
		return "ollama/nomic-embed-text"
	default:
		return "googleai/text-embedding-004"
	}
}

// resolveEmbeddingDim returns the embedding vector dimension for the given model name.
// This determines the VectorFullDim for MEB's vector registry — mismatches cause
// all vector Add operations to fail with "invalid vector dimension" errors.
func resolveEmbeddingDim(model string) int {
	name := strings.ToLower(model)

	// Known embedding model dimensions
	switch {
	case strings.Contains(name, "text-embedding-004"):
		return 768
	case strings.Contains(name, "text-embedding-3-large"):
		return 3072
	case strings.Contains(name, "text-embedding-3-small"):
		return 1536
	case strings.Contains(name, "text-embedding-ada-002"):
		return 1536
	case strings.Contains(name, "gemini-embedding-001"), strings.Contains(name, "embedding-001"):
		return 3072
	case strings.Contains(name, "nomic-embed-text"):
		return 768
	default:
		return 768 // safe default for most common models
	}
}

// GetEmbeddingDim returns the embedding dimension for the given model.
// Checks EMBEDDING_DIM env var first (for override), then falls back to model-based lookup.
// This is used independently of LoadFromEnv() for store configuration paths
// that don't need the full LLM config (e.g., CLI ingest, StoreManager).
func GetEmbeddingDim(model string) int {
	if dimStr := os.Getenv("EMBEDDING_DIM"); dimStr != "" {
		if dim, err := strconv.Atoi(dimStr); err == nil && dim > 0 {
			return dim
		}
	}
	if model == "" {
		// Resolve default embedding model from env
		provider := os.Getenv("LLM_PROVIDER")
		if provider == "" {
			provider = "googleai"
		}
		model = resolveEmbeddingModel(provider, os.Getenv("EMBEDDING_MODEL"))
	}
	return resolveEmbeddingDim(model)
}