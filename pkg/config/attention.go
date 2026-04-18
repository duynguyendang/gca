package config

import "strings"

// IsAttentionWorthyName returns true if the symbol name matches attention-worthy patterns.
// These are name-based heuristics for detecting important code elements.
// Used by both the OODA orient phase and the query registry sticky-fact detection.
func IsAttentionWorthyName(name string) bool {
	lower := strings.ToLower(name)

	// Entry point patterns
	if strings.Contains(lower, "main") || strings.Contains(lower, "init") || strings.Contains(lower, "start") {
		return true
	}

	// Event/Callback patterns
	if strings.Contains(lower, "handler") || strings.HasPrefix(lower, "on_") || strings.HasSuffix(lower, "handler") || strings.HasSuffix(lower, "callback") {
		return true
	}

	// Lifecycle patterns
	if strings.Contains(lower, "setup") || strings.Contains(lower, "teardown") || strings.Contains(lower, "bootstrap") || strings.Contains(lower, "cleanup") {
		return true
	}

	// Test patterns
	if strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "Test") || strings.HasPrefix(name, "Test") || strings.HasSuffix(name, "_bench") || strings.HasSuffix(name, "Benchmark") {
		return true
	}

	// Security/Auth patterns
	if strings.Contains(lower, "auth") || strings.Contains(lower, "login") || strings.Contains(lower, "logout") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") || strings.Contains(lower, "token") || strings.Contains(lower, "session") || strings.Contains(lower, "sanitize") || strings.Contains(lower, "authorize") {
		return true
	}

	// Validation patterns
	if strings.Contains(lower, "validate") || strings.Contains(lower, "verify") || strings.Contains(lower, "check") {
		return true
	}

	// CRUD patterns (prefix-based)
	if strings.HasPrefix(name, "Create") || strings.HasPrefix(name, "Delete") || strings.HasPrefix(name, "Update") || strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List") || strings.HasPrefix(name, "Put") {
		return true
	}

	// Error/Recovery patterns
	if strings.Contains(lower, "error") || strings.Contains(lower, "retry") || strings.Contains(lower, "fallback") || strings.Contains(lower, "recovery") || strings.Contains(lower, "recover") {
		return true
	}

	return false
}
