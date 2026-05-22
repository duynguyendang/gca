package intent

import (
	"testing"

	"github.com/duynguyendang/gca/pkg/nlp/types"
)

func TestIntentClassifier_Classify(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name     string
		query    string
		wantIntent Intent
		wantConfMin float64
	}{
		{"who calls", "who calls auth.go", IntentWhoCalls, 0.8},
		{"what calls", "what does auth.go call", IntentWhatCalls, 0.8},
		{"find symbol", "where is the login handler", IntentFind, 0.7},
		{"explain", "explain how auth works", IntentExplain, 0.7},
		{"summarize", "summarize main.go", IntentSummarize, 0.7},
		{"security", "check for sql injection", IntentSecurity, 0.85},
		{"test generation", "write unit tests for auth", IntentTestGen, 0.8},
		{"refactor", "refactor this code", IntentRefactor, 0.8},
		{"performance", "find performance bottlenecks", IntentPerformance, 0.8},
		{"chat fallback", "hello there", IntentChat, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier.Classify(tt.query)
			if got.Intent != tt.wantIntent {
				t.Errorf("Classify(%q) intent = %v, want %v", tt.query, got.Intent, tt.wantIntent)
			}
			if got.Confidence < tt.wantConfMin {
				t.Errorf("Classify(%q) confidence = %v, want >= %v", tt.query, got.Confidence, tt.wantConfMin)
			}
		})
	}
}

func TestIntentClassifier_ClassifyWithContext(t *testing.T) {
	classifier := NewClassifier()

	history := []*types.ConversationTurn{
		{UserInput: "explain auth.go", Intent: "explain", ResultCount: 5},
	}

	t.Run("follow-up with pronoun", func(t *testing.T) {
		result := classifier.ClassifyWithContext("it is slow", history)
		if result.Intent != IntentExplain {
			t.Errorf("follow-up should maintain intent, got %v", result.Intent)
		}
		if result.Extracted["pronoun_target"] == "" {
			t.Errorf("pronoun_target should be extracted")
		}
	})

	t.Run("intent transition to find", func(t *testing.T) {
		result := classifier.ClassifyWithContext("where is it defined", history)
		if result.Intent != IntentFind {
			t.Errorf("transition from explain to find should switch intent, got %v", result.Intent)
		}
	})
}

func TestIntentClassifier_extractIntentFeatures(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name          string
		query         string
		wantHasSymbol bool
	}{
		{"camelcase symbol", "Explain AuthService.Login", true},
		{"dot path symbol", "Explain auth/service.go", true},
		{"plain text no symbol", "what is this code doing", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			features := classifier.extractIntentFeatures(tt.query, "")
			if features.HasSymbol != tt.wantHasSymbol {
				t.Errorf("HasSymbol = %v, want %v", features.HasSymbol, tt.wantHasSymbol)
			}
		})
	}
}

func TestIntentClassifier_isFollowUp(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		query    string
		expected bool
	}{
		{"it is slow", true},
		{"that was helpful", true},
		{"tell me more", true},
		{"and then what", true},
		{"explain how auth works", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := classifier.isFollowUp(tt.query)
			if got != tt.expected {
				t.Errorf("isFollowUp(%q) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

func TestIntentClassifier_TemplateFor(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		intent Intent
		target string
		check  func(string) bool
	}{
		{IntentWhoCalls, "Auth", func(s string) bool {
			return contains(s, "?caller") && contains(s, "calls") && contains(s, "Auth")
		}},
		{IntentWhatCalls, "Auth", func(s string) bool {
			return contains(s, "Auth") && contains(s, "calls") && contains(s, "?callee")
		}},
		{IntentFind, "Handler", func(s string) bool {
			return contains(s, "defines") && contains(s, "regex")
		}},
		{IntentChat, "", func(s string) bool {
			return contains(s, "triples")
		}},
	}

	for _, tt := range tests {
		t.Run(string(tt.intent), func(t *testing.T) {
			got := classifier.TemplateFor(tt.intent, tt.target)
			if !tt.check(got) {
				t.Errorf("TemplateFor(%v, %q) = %q, did not match expected pattern", tt.intent, tt.target, got)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}