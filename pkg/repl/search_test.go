package repl

import (
	"testing"
)

func TestFindNodesBySimilarity(t *testing.T) {
	symbols := []string{
		"pkg/meb/store.go",
		"pkg/meb/dict/encoder.go",
		"pkg/datalog/parser.go",
		"pkg/repl/repl.go",
		"main.go",
		"README.md",
	}

	tests := []struct {
		name     string
		query    string
		expected []string // We expect at least the first one to be the top match
	}{
		{
			name:     "Exact mismatch but relevant",
			query:    "meb store",
			expected: []string{"pkg/meb/store.go"},
		},
		{
			name:     "Typo in query",
			query:    "storag",
			expected: []string{"pkg/meb/store.go"},
		},
		{
			name:     "CamelCase split",
			query:    "repl go",
			expected: []string{"pkg/repl/repl.go"},
		},
		{
			name:     "Partial filename",
			query:    "parser",
			expected: []string{"pkg/datalog/parser.go"},
		},
		{
			name:     "Acronym or short",
			query:    "readme",
			expected: []string{"README.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindNodesBySimilarity(tt.query, symbols)
			if len(got) == 0 {
				t.Errorf("FindNodesBySimilarity() returned no results for %q", tt.query)
				return
			}
			// Check if expected[0] is in the top 3 results
			found := false
			limit := 3
			if len(got) < limit {
				limit = len(got)
			}

			for i := 0; i < limit; i++ {
				if got[i] == tt.expected[0] {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("FindNodesBySimilarity() top results = %v, want %v to be in top results", got, tt.expected[0])
			}
		})
	}
}
