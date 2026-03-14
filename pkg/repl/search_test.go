package repl

import (
	"testing"
)

func TestFindNodesBySimilarity(t *testing.T) {
	symbols := []string{
		"github.com/duynguyendang/meb/store.go",
		"github.com/duynguyendang/meb/dict/encoder.go",
		"github.com/duynguyendang/gca/pkg/datalog/parser.go",
		"github.com/duynguyendang/gca/pkg/repl/repl.go",
		"github.com/duynguyendang/gca/main.go",
		"github.com/duynguyendang/gca/README.md",
	}

	tests := []struct {
		name     string
		query    string
		expected []string // We expect at least the first one to be the top match
	}{
		{
			name:     "Exact mismatch but relevant",
			query:    "meb store",
			expected: []string{"github.com/duynguyendang/meb/store.go"},
		},
		{
			name:     "Typo in query",
			query:    "storag",
			expected: []string{"github.com/duynguyendang/meb/store.go"},
		},
		{
			name:     "CamelCase split",
			query:    "repl go",
			expected: []string{"github.com/duynguyendang/gca/pkg/repl/repl.go"},
		},
		{
			name:     "Partial filename",
			query:    "parser",
			expected: []string{"github.com/duynguyendang/gca/pkg/datalog/parser.go"},
		},
		{
			name:     "Acronym or short",
			query:    "readme",
			expected: []string{"github.com/duynguyendang/gca/README.md"},
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
