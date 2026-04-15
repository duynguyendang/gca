package logger

import (
	"testing"
)

func TestSetLevelFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"DEBUG", LevelDebug},
		{"debug", LevelDebug},
		{"INFO", LevelInfo},
		{"info", LevelInfo},
		{"WARN", LevelWarn},
		{"warn", LevelWarn},
		{"WARNING", LevelWarn},
		{"warning", LevelWarn},
		{"ERROR", LevelError},
		{"error", LevelError},
		{"INVALID", LevelInfo}, // Default to INFO for unknown values
		{"", LevelInfo},        // Default to INFO for empty string
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			SetLevelFromString(tt.input)
			if GetLevel() != tt.expected {
				t.Errorf("SetLevelFromString(%q) = %v, want %v", tt.input, GetLevel(), tt.expected)
			}
		})
	}
}

func TestLoggerFunctions(t *testing.T) {
	// Set to DEBUG to ensure all levels are tested
	SetLevel(LevelDebug)

	// These should not panic
	Debug("test debug message", "key", "value")
	Info("test info message", "key", "value")
	Warn("test warn message", "key", "value")
	Error("test error message", "key", "value")

	// Test With logger
	loggerWith := With("component", "test")
	if loggerWith == nil {
		t.Error("With() returned nil")
	}
}

func TestLevelConstants(t *testing.T) {
	// Verify level constants match slog levels
	if LevelDebug != -4 {
		t.Errorf("LevelDebug = %v, want -4", LevelDebug)
	}
	if LevelInfo != 0 {
		t.Errorf("LevelInfo = %v, want 0", LevelInfo)
	}
	if LevelWarn != 4 {
		t.Errorf("LevelWarn = %v, want 4", LevelWarn)
	}
	if LevelError != 8 {
		t.Errorf("LevelError = %v, want 8", LevelError)
	}
}
