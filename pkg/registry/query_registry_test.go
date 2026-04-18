package registry

import (
	"os"
	"testing"
)

func TestSortEntries(t *testing.T) {
	// Create temp dir with known entries
	tmpDir, err := os.MkdirTemp("", "sort_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files in random order
	names := []string{"z_file.dl", "a_file.dl", "m_file.dl"}
	for _, name := range names {
		f, err := os.CreateTemp(tmpDir, name)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Shuffle for test (but sort won't shuffle - we test sorting is stable)
	sortEntries(entries)

	// Verify sorted order
	for i := range entries {
		if i > 0 && entries[i].Name() < entries[i-1].Name() {
			t.Errorf("Entries not sorted: at index %d got %q which is less than previous %q",
				i, entries[i].Name(), entries[i-1].Name())
		}
	}
}

func TestBuildQueryTemplate(t *testing.T) {
	// Create registry with nil engine - tests that nil engine is handled gracefully
	r := &QueryRegistry{}

	tests := []struct {
		queryName string
	}{
		{
			queryName: "find_defines",
		},
		{
			queryName: "smell_circular_direct",
		},
		{
			queryName: "unknown_query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.queryName, func(t *testing.T) {
			// With nil engine, buildQueryTemplate should return error or fallback
			got, err := r.buildQueryTemplate(nil, tt.queryName)
			if err != nil {
				// Nil engine is acceptable - error is expected
				t.Logf("buildQueryTemplate with nil engine returned error (expected): %v", err)
				return
			}
			if got == "" {
				t.Errorf("buildQueryTemplate(%q) returned empty string", tt.queryName)
			}
		})
	}
}

func TestBuildQueryFromTemplate(t *testing.T) {
	r := &QueryRegistry{}

	tests := []struct {
		name     string
		template string
		params   map[string]any
		expected string
	}{
		{
			name:     "simple substitution",
			template: `triples({FileID}, "defines", Symbol)`,
			params:   map[string]any{"FileID": "main.go"},
			expected: `triples(main.go, "defines", Symbol)`,
		},
		{
			name:     "multiple substitutions",
			template: `triples({FileID}, "calls", {Target})`,
			params:   map[string]any{"FileID": "main.go", "Target": "foo.go"},
			expected: `triples(main.go, "calls", foo.go)`,
		},
		{
			name:     "no substitution",
			template: `triples(A, "calls", B)`,
			params:   map[string]any{},
			expected: `triples(A, "calls", B)`,
		},
		{
			name:     "partial substitution",
			template: `triples({FileID}, "imports", Target)`,
			params:   map[string]any{"Other": "value"},
			expected: `triples({FileID}, "imports", Target)`,
		},
		{
			name:     "integer substitution",
			template: `limit({MaxResults})`,
			params:   map[string]any{"MaxResults": 100},
			expected: `limit(100)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.buildQueryFromTemplateSecure(tt.template, tt.params)
			if got != tt.expected {
				t.Errorf("buildQueryFromTemplateSecure(%q, %v) = %q, want %q",
					tt.template, tt.params, got, tt.expected)
			}
		})
	}
}

func TestExtractParameters(t *testing.T) {
	r := &QueryRegistry{}

	tests := []struct {
		name     string
		template string
		wantNames []string
		wantCount int
	}{
		{
			name:     "has FileID placeholder",
			template: `triples({FileID}, "defines", Symbol)`,
			wantNames: []string{"FileID"},
			wantCount:  1,
		},
		{
			name:     "has Symbol placeholder",
			template: `triples(File, "defines", {Symbol})`,
			wantNames: []string{"Symbol"},
			wantCount:  1,
		},
		{
			name:     "has both FileID and Symbol placeholders",
			template: `triples({FileID}, "defines", {Symbol})`,
			wantNames: []string{"FileID", "Symbol"},
			wantCount:  2,
		},
		{
			name:     "has FileID and Target placeholders",
			template: `triples({FileID}, "calls", {Target})`,
			wantNames: []string{"FileID", "Target"},
			wantCount:  2, // All placeholders are now extracted
		},
		{
			name:     "no placeholders",
			template: `triples(A, "calls", B)`,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.extractParameters(tt.template)
			if len(got) != tt.wantCount {
				t.Errorf("extractParameters(%q) returned %d params, want %d",
					tt.template, len(got), tt.wantCount)
			}
			for _, wantName := range tt.wantNames {
				found := false
				for _, p := range got {
					if p.Name == wantName {
						found = true
						if p.Type != "string" || !p.Required {
							t.Errorf("%s param type=%q required=%v, want string and true", wantName, p.Type, p.Required)
						}
						break
					}
				}
				if !found {
					t.Errorf("extractParameters(%q) missing %s param", tt.template, wantName)
				}
			}
		})
	}
}

func TestQueryRegistryNew(t *testing.T) {
	r := NewQueryRegistry(nil)
	if r == nil {
		t.Fatal("NewQueryRegistry returned nil")
	}
	if r.queries == nil {
		t.Error("r.queries is nil")
	}
	if r.categories == nil {
		t.Error("r.categories is nil")
	}
}
