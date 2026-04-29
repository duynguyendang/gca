package llmconfig

import (
	"fmt"
	"os"
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
	OllamaAddress  string
	Plugins        []api.Plugin
}

func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Provider: os.Getenv("LLM_PROVIDER"),
		APIKey:   os.Getenv("LLM_API_KEY"),
		DefaultModel: os.Getenv("LLM_MODEL"),
		EmbeddingModel: os.Getenv("EMBEDDING_MODEL"),
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