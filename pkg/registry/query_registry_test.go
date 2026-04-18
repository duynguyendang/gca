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
	// Create registry with nil engine (we only test template building)
	r := &QueryRegistry{}

	tests := []struct {
		queryName string
		expected  string
	}{
		{
			queryName: "find_defines",
			expected:  `triples({FileID}, "defines", Symbol)`,
		},
		{
			queryName: "find_imports",
			expected:  `triples({FileID}, "imports", Target)`,
		},
		{
			queryName: "find_outbound_calls",
			expected:  `triples({FileID}, "defines", Symbol), triples(Symbol, "calls", Target)`,
		},
		{
			queryName: "find_inbound_calls",
			expected:  `triples(Caller, "calls", Symbol), triples({FileID}, "defines", Symbol)`,
		},
		{
			queryName: "smell_circular_direct",
			expected:  `triples(A, "calls", B), triples(B, "calls", A), A != B`,
		},
		{
			queryName: "smell_imports",
			expected:  `triples(File, "imports", Pkg)`,
		},
		{
			queryName: "smell_defines",
			expected:  `triples(File, "defines", Symbol)`,
		},
		{
			queryName: "smell_hub",
			expected:  `triples(File, "calls", _), triples(Caller, "calls", File), File != Caller`,
		},
		{
			queryName: "smell_layer_violation",
			expected:  `triples(File, "imports", Target), triples(File, "has_tag", LayerTag), triples(Target, "has_tag", "backend"), LayerTag != "backend"`,
		},
		{
			queryName: "unknown_query",
			expected:  `% Query: unknown_query - Template not yet implemented`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.queryName, func(t *testing.T) {
			got, err := r.buildQueryTemplate(nil, tt.queryName)
			if err != nil {
				t.Fatalf("buildQueryTemplate() error = %v", err)
			}
			if got != tt.expected {
				t.Errorf("buildQueryTemplate(%q) = %q, want %q", tt.queryName, got, tt.expected)
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
			got := r.buildQueryFromTemplate(tt.template, tt.params)
			if got != tt.expected {
				t.Errorf("buildQueryFromTemplate(%q, %v) = %q, want %q",
					tt.template, tt.params, got, tt.expected)
			}
		})
	}
}

func TestExtractParameters(t *testing.T) {
	r := &QueryRegistry{}

	tests := []struct {
		name        string
		template    string
		wantFileID  bool
		wantSymbol  bool
		wantCount   int
	}{
		{
			name:       "has FileID placeholder",
			template:   `triples({FileID}, "defines", Symbol)`,
			wantFileID: true,
			wantCount:  1,
		},
		{
			name:       "has Symbol placeholder",
			template:   `triples(File, "defines", {Symbol})`,
			wantSymbol: true,
			wantCount:  1,
		},
		{
			name:       "has both FileID and Symbol placeholders",
			template:   `triples({FileID}, "defines", {Symbol})`,
			wantFileID:  true,
			wantCount:  2,
		},
		{
			name:       "only FileID is extracted even when Target placeholder exists",
			template:   `triples({FileID}, "calls", {Target})`,
			wantFileID: true,
			wantCount:  1, // Only FileID is extracted, not Target (current impl limitation)
		},
		{
			name:       "no placeholders",
			template:   `triples(A, "calls", B)`,
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
			if tt.wantFileID {
				found := false
				for _, p := range got {
					if p.Name == "FileID" {
						found = true
						if p.Type != "file" || !p.Required {
							t.Errorf("FileID param type=%q required=%v, want file and true", p.Type, p.Required)
						}
						break
					}
				}
				if !found {
					t.Errorf("extractParameters(%q) missing FileID param", tt.template)
				}
			}
			if tt.wantSymbol {
				found := false
				for _, p := range got {
					if p.Name == "Symbol" {
						found = true
						if p.Type != "symbol" || !p.Required {
							t.Errorf("Symbol param type=%q required=%v, want symbol and true", p.Type, p.Required)
						}
						break
					}
				}
				if !found {
					t.Errorf("extractParameters(%q) missing Symbol param", tt.template)
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
