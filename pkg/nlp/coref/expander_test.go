package coref

import (
	"testing"

	"github.com/duynguyendang/gca/pkg/nlp/types"
)

func TestPronounExpander_ExpandPronouns(t *testing.T) {
	history := []*types.ConversationTurn{
		{UserInput: "explain auth.go", Intent: "explain", ResultCount: 5},
		{UserInput: "what does auth.go call", Intent: "what_calls", ResultCount: 3},
	}
	entities := []*types.Entity{
		{Name: "auth.go", EntityType: "file", Source: "history"},
	}

	expander := NewExpander(history, entities)

	tests := []struct {
		name          string
		query         string
		wantExpanded  string
		wantExpansion bool
	}{
		{"it reference", "it is slow", "auth.go is slow", true},
		{"that reference", "that was helpful", "auth.go was helpful", true},
		{"no pronoun", "explain auth.go", "explain auth.go", false},
		{"pronoun at start", "it is defined", "auth.go is defined", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, expansions := expander.ExpandPronouns(tt.query)
			if tt.wantExpansion && len(expansions) == 0 {
				t.Errorf("expected expansion for %q", tt.query)
			}
			if !tt.wantExpansion && len(expansions) > 0 {
				t.Errorf("did not expect expansion for %q", tt.query)
			}
			_ = expanded
		})
	}
}

func TestPronounExpander_isPronoun(t *testing.T) {
	expander := NewExpander(nil, nil)

	tests := []struct {
		word     string
		expected bool
	}{
		{"it", true},
		{"this", true},
		{"that", true},
		{"they", true},
		{"them", true},
		{"we", true},
		{"us", true},
		{"auth", false},
		{"explain", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := expander.isPronoun(tt.word)
			if got != tt.expected {
				t.Errorf("isPronoun(%q) = %v, want %v", tt.word, got, tt.expected)
			}
		})
	}
}

func TestPronounExpander_findInHistory(t *testing.T) {
	history := []*types.ConversationTurn{
		{UserInput: "explain auth.go:LoginHandler", Intent: "explain", ResultCount: 5},
	}
	expander := NewExpander(history, nil)

	t.Run("finds file in history", func(t *testing.T) {
		entity := expander.findInHistory()
		if entity == nil {
			t.Fatal("expected to find entity in history")
		}
		if entity.EntityType != "file" {
			t.Errorf("EntityType = %q, want %q", entity.EntityType, "file")
		}
	})

	t.Run("returns nil for empty history", func(t *testing.T) {
		expander.history = nil
		entity := expander.findInHistory()
		if entity != nil {
			t.Error("expected nil for empty history")
		}
	})
}

func TestPronounExpander_SetHistory(t *testing.T) {
	expander := NewExpander(nil, nil)
	newHistory := []*types.ConversationTurn{
		{UserInput: "test query", Intent: "find", ResultCount: 1},
	}

	expander.SetHistory(newHistory)

	if len(expander.history) != 1 {
		t.Errorf("history length = %d, want 1", len(expander.history))
	}
}

func TestPronounExpander_replacePronounWord(t *testing.T) {
	expander := NewExpander(nil, nil)

	tests := []struct {
		query     string
		pronoun   string
		resolved  string
		expected  string
	}{
		{"it is slow", "it", "auth.go", "auth.go is slow"},
		{"that was great", "that", "main.go", "main.go was great"},
		{"where is it", "it", "Handler", "where is Handler"},
		{"spit it out", "it", "auth.go", "spit auth.go out"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := expander.replacePronounWord(tt.query, tt.pronoun, tt.resolved)
			if got != tt.expected {
				t.Errorf("replacePronounWord(%q, %q, %q) = %q, want %q", tt.query, tt.pronoun, tt.resolved, got, tt.expected)
			}
		})
	}
}

func TestPronounExpander_resolveToEntity(t *testing.T) {
	history := []*types.ConversationTurn{
		{UserInput: "explain auth.go", Intent: "explain", ResultCount: 1},
	}
	entities := []*types.Entity{
		{Name: "auth.go", EntityType: "file", Source: "pattern"},
	}
	expander := NewExpander(history, entities)

	t.Run("resolves to entity in history", func(t *testing.T) {
		expansion := expander.resolveToEntity("it")
		if expansion == nil {
			t.Fatal("expected expansion")
		}
		if expansion.Confidence < 0.5 {
			t.Errorf("Confidence = %v, want >= 0.5", expansion.Confidence)
		}
	})

	t.Run("returns nil for non-pronoun query", func(t *testing.T) {
		expansion := expander.resolveToEntity("explain")
		if expansion != nil {
			t.Error("expected nil for non-pronoun query")
		}
	})
}