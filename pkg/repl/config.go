package repl

// Config holds configuration for the REPL environment.
type Config struct {
	// GeminiAPIKey is the API key for Google Gemini.
	GeminiAPIKey string
	// Model is the default model to use (e.g., "gemini-3-flash-preview").
	Model string
	// Temperature is the default temperature for generation.
	Temperature float32
	// ReadOnly indicates if the store is in read-only mode.
	ReadOnly bool
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Model:       "gemini-3-flash-preview",
		Temperature: 0.2,
	}
}
