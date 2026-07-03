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

type providerDescriptor struct {
	plugin         func(apiKey, ollamaAddr string) api.Plugin
	defaultModel   string
	embeddingModel string
}

var providerDescriptors = map[string]providerDescriptor{
	"googleai": {
		plugin:         func(apiKey, _ string) api.Plugin { return &googlegenai.GoogleAI{APIKey: apiKey} },
		defaultModel:   "googleai/gemini-2.5-flash",
		embeddingModel: "googleai/text-embedding-004",
	},
	"gemini": {
		plugin:         func(apiKey, _ string) api.Plugin { return &googlegenai.GoogleAI{APIKey: apiKey} },
		defaultModel:   "googleai/gemini-2.5-flash",
		embeddingModel: "googleai/text-embedding-004",
	},
	"openai": {
		plugin:         func(apiKey, _ string) api.Plugin { return &openai.OpenAI{APIKey: apiKey} },
		defaultModel:   "openai/gpt-4o",
		embeddingModel: "openai/text-embedding-3-large",
	},
	"anthropic": {
		plugin:         func(apiKey, _ string) api.Plugin { return &anthropic.Anthropic{APIKey: apiKey} },
		defaultModel:   "anthropic/claude-3-5-sonnet-20241022",
		embeddingModel: "",
	},
	"ollama": {
		plugin:         func(_, ollamaAddr string) api.Plugin { return &ollama.Ollama{ServerAddress: ollamaAddr} },
		defaultModel:   "ollama/llama3.2",
		embeddingModel: "ollama/nomic-embed-text",
	},
}

func createPlugins(provider, apiKey, ollamaAddr string) []api.Plugin {
	if desc, ok := providerDescriptors[provider]; ok {
		return []api.Plugin{desc.plugin(apiKey, ollamaAddr)}
	}
	return []api.Plugin{&googlegenai.GoogleAI{APIKey: apiKey}}
}

func resolveDefaultModel(provider, model string) string {
	if model != "" {
		if !strings.Contains(model, "/") {
			return provider + "/" + model
		}
		return model
	}
	if desc, ok := providerDescriptors[provider]; ok {
		return desc.defaultModel
	}
	return "googleai/gemini-2.5-flash"
}

func resolveEmbeddingModel(provider, model string) string {
	if model != "" {
		if !strings.Contains(model, "/") {
			return provider + "/" + model
		}
		return model
	}
	if desc, ok := providerDescriptors[provider]; ok {
		return desc.embeddingModel
	}
	return "googleai/text-embedding-004"
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