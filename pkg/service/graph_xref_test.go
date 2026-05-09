package service

import (
	"context"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func TestSplitSymbolID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "with colon separator",
			input:    "pkg/foo.go:main",
			expected: []string{"pkg/foo.go", "main"},
		},
		{
			name:     "with parentheses",
			input:    "pkg/foo.go:main()",
			expected: []string{"pkg/foo.go", "main()"},
		},
		{
			name:     "no colon returns full string",
			input:    "main",
			expected: []string{"main"},
		},
		{
			name:     "multiple colons only splits at first",
			input:    "pkg/foo.go:main:something",
			expected: []string{"pkg/foo.go", "main:something"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSymbolID(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitSymbolID(%q) len = %d, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitSymbolID(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExtractName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple symbol",
			input:    "main",
			expected: "main",
		},
		{
			name:     "file colon symbol",
			input:    "pkg/foo.go:main",
			expected: "main",
		},
		{
			name:     "file colon function with parens",
			input:    "pkg/foo.go:main()",
			expected: "main",
		},
		{
			name:     "file colon method with parens",
			input:    "pkg/foo.go:HandleRequest()",
			expected: "HandleRequest",
		},
		{
			name:     "symbol with spaces",
			input:    "some symbol",
			expected: "some symbol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractName(tt.input)
			if got != tt.expected {
				t.Errorf("extractName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStringsIndex(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected int
	}{
		{
			name:     "found at start",
			s:        "hello world",
			substr:   "hel",
			expected: 0,
		},
		{
			name:     "found in middle",
			s:        "hello world",
			substr:   "wor",
			expected: 6,
		},
		{
			name:     "not found",
			s:        "hello world",
			substr:   "xyz",
			expected: -1,
		},
		{
			name:     "empty string",
			s:        "",
			substr:   "x",
			expected: -1,
		},
		{
			name:     "empty substr",
			s:        "hello",
			substr:   "",
			expected: 0,
		},
		{
			name:     "equal strings",
			s:        "hello",
			substr:   "hello",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringsIndex(tt.s, tt.substr)
			if got != tt.expected {
				t.Errorf("stringsIndex(%q, %q) = %d, want %d", tt.s, tt.substr, got, tt.expected)
			}
		})
	}
}

func TestStringsHasPrefix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		prefix   string
		expected bool
	}{
		{
			name:     "simple match",
			s:        "hello world",
			prefix:   "hel",
			expected: true,
		},
		{
			name:     "full match",
			s:        "hello",
			prefix:   "hello",
			expected: true,
		},
		{
			name:     "no match",
			s:        "hello world",
			prefix:   "xyz",
			expected: false,
		},
		{
			name:     "empty string",
			s:        "",
			prefix:   "hel",
			expected: false,
		},
		{
			name:     "empty prefix",
			s:        "hello",
			prefix:   "",
			expected: true,
		},
		{
			name:     "prefix longer than string",
			s:        "hi",
			prefix:   "hello",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringsHasPrefix(tt.s, tt.prefix)
			if got != tt.expected {
				t.Errorf("stringsHasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.expected)
			}
		})
	}
}

func TestGuessKind(t *testing.T) {
	tests := []struct {
		name     string
		symbol   string
		expected string
	}{
		{
			name:     "New prefix",
			symbol:   "NewClient",
			expected: "func",
		},
		{
			name:     "Create prefix",
			symbol:   "CreateUser",
			expected: "func",
		},
		{
			name:     "Get prefix",
			symbol:   "GetConfig",
			expected: "func",
		},
		{
			name:     "Load prefix",
			symbol:   "LoadData",
			expected: "func",
		},
		{
			name:     "function with parens",
			symbol:   "DoSomething()",
			expected: "func",
		},
		{
			name:     "uppercase start - struct",
			symbol:   "UserService",
			expected: "struct",
		},
		{
			name:     "lowercase start",
			symbol:   "userService",
			expected: "symbol",
		},
		{
			name:     "single lowercase letter",
			symbol:   "x",
			expected: "symbol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guessKind(tt.symbol)
			if got != tt.expected {
				t.Errorf("guessKind(%q) = %q, want %q", tt.symbol, got, tt.expected)
			}
		})
	}
}

func testStoreWithFacts(t *testing.T, facts []meb.Fact) *meb.MEBStore {
	cfg := &store.Config{
		InMemory:       true,
		DataDir:        "",
		DictDir:        "",
		BlockCacheSize: 10 << 20,
		IndexCacheSize: 10 << 20,
		LRUCacheSize:   1000,
	}
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatalf("NewMEBStore: %v", err)
	}
	for _, f := range facts {
		if err := s.AddFact(f); err != nil {
			t.Fatalf("AddFact: %v", err)
		}
	}
	return s
}

func TestGetCallerLine(t *testing.T) {
	facts := []meb.Fact{
		{Subject: "caller1", Predicate: config.PredicateCallsLine, Object: "foo:42"},
		{Subject: "caller1", Predicate: config.PredicateCallsLine, Object: "bar:17"},
		{Subject: "caller1", Predicate: config.PredicateCallsLine, Object: "baz:99"},
		{Subject: "caller2", Predicate: config.PredicateCallsLine, Object: "foo:10"},
		{Subject: "caller1", Predicate: config.PredicateCallsLine, Object: "foo:100"},
	}
	store := testStoreWithFacts(t, facts)
	ctx := context.Background()

	tests := []struct {
		name   string
		caller string
		callee string
		want   int
	}{
		{name: "found first match", caller: "caller1", callee: "foo", want: 42},
		{name: "found different callee same subject", caller: "caller1", callee: "bar", want: 17},
		{name: "different caller", caller: "caller2", callee: "foo", want: 10},
		{name: "not found", caller: "caller1", callee: "unknown", want: 0},
		{name: "missing colon", caller: "caller1", callee: "nocolon", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCallerLine(ctx, store, tt.caller, tt.callee)
			if got != tt.want {
				t.Errorf("getCallerLine(%q, %q) = %d, want %d", tt.caller, tt.callee, got, tt.want)
			}
		})
	}
}

func TestBuildCalleeLineMap(t *testing.T) {
	facts := []meb.Fact{
		{Subject: "symA", Predicate: config.PredicateCallsLine, Object: "foo:42"},
		{Subject: "symA", Predicate: config.PredicateCallsLine, Object: "bar:17"},
		{Subject: "symB", Predicate: config.PredicateCallsLine, Object: "foo:10"},
	}
	store := testStoreWithFacts(t, facts)
	ctx := context.Background()

	t.Run("multiple callees", func(t *testing.T) {
		m := buildCalleeLineMap(ctx, store, "symA")
		if m["foo"] != 42 {
			t.Errorf("foo = %d, want 42", m["foo"])
		}
		if m["bar"] != 17 {
			t.Errorf("bar = %d, want 17", m["bar"])
		}
	})

	t.Run("no facts for subject", func(t *testing.T) {
		m := buildCalleeLineMap(ctx, store, "symC")
		if len(m) != 0 {
			t.Errorf("symC map len = %d, want 0", len(m))
		}
	})

	t.Run("missing colon skipped", func(t *testing.T) {
		facts := []meb.Fact{
			{Subject: "symX", Predicate: config.PredicateCallsLine, Object: "badformat"},
		}
		store := testStoreWithFacts(t, facts)
		m := buildCalleeLineMap(ctx, store, "symX")
		if len(m) != 0 {
			t.Errorf("badformat should not produce entry, got len %d", len(m))
		}
	})
}
