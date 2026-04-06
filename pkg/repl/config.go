package repl

import "github.com/firebase/genkit/go/genkit"

// Config holds configuration for the REPL environment.
type Config struct {
	// LLMProvider is the LLM provider (googleai, openai, anthropic, ollama).
	LLMProvider string
	// LLMAPIKey is the API key for the LLM provider.
	LLMAPIKey string
	// Model is the default model to use (e.g., "googleai/gemini-2.5-flash").
	Model string
	// Temperature is the default temperature for generation.
	Temperature float32
	// ReadOnly indicates if the store is in read-only mode.
	ReadOnly bool
	// Genkit is the shared Genkit instance (initialized once).
	Genkit *genkit.Genkit
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		LLMProvider: "googleai",
		Model:       "googleai/gemini-2.5-flash",
		Temperature: 0.2,
	}
}
