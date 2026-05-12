package datalog

import (
	"testing"
)

func TestNewQueryOptimizer(t *testing.T) {
	opt := NewQueryOptimizer()
	if opt == nil {
		t.Error("NewQueryOptimizer returned nil")
	}
}

func TestOptimizeQuery_Simple(t *testing.T) {
	opt := NewQueryOptimizer()

	atoms := []Atom{
		{Predicate: "calls", Args: []string{"?x", "?y"}},
		{Predicate: "defines", Args: []string{"?x", "?z"}},
	}
	result := opt.OptimizeQuery(atoms)
	if len(result) != 2 {
		t.Errorf("OptimizeQuery len = %d, want 2", len(result))
	}
}

func TestOptimizeQuery_ManyAtoms(t *testing.T) {
	opt := NewQueryOptimizer()

	atoms := []Atom{
		{Predicate: "calls", Args: []string{"?x", "?y"}},
		{Predicate: "defines", Args: []string{"main.go", "?z"}},
		{Predicate: "imports", Args: []string{"main.go", "?w"}},
		{Predicate: "hasDoc", Args: []string{"?x", "?d"}},
	}

	result := opt.OptimizeQuery(atoms)

	foundDefinesIdx := -1
	foundImportsIdx := -1
	for i, a := range result {
		if a.Predicate == "defines" {
			foundDefinesIdx = i
		}
		if a.Predicate == "imports" {
			foundImportsIdx = i
		}
	}
	if foundDefinesIdx >= foundImportsIdx {
		t.Errorf("expected defines before imports, got defines at %d, imports at %d", foundDefinesIdx, foundImportsIdx)
	}
}

func TestOptimizeQuery_Empty(t *testing.T) {
	opt := NewQueryOptimizer()
	result := opt.OptimizeQuery([]Atom{})
	if len(result) != 0 {
		t.Errorf("OptimizeQuery empty = %d, want 0", len(result))
	}
}

func TestIsVariable(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		{"?x", true},
		{"?s", true},
		{"?Main", true},
		{"main.go", false},
		{"calls", false},
		{"_", false},
		{"X", false},   // only ? prefix is variable
		{"S", false},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			if got := isVariable(tt.arg); got != tt.want {
				t.Errorf("isVariable(%q) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestTrimQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"foo"`, "foo"},
		{`"bar"`, "bar"},
		{"foo", "foo"},
		{`""`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := trimQuotes(tt.input); got != tt.want {
				t.Errorf("trimQuotes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnalyzeVariables(t *testing.T) {
	opt := NewQueryOptimizer()
	atoms := []Atom{
		{Predicate: "calls", Args: []string{"?x", "?y"}},
		{Predicate: "defines", Args: []string{"?x", "?z"}},
		{Predicate: "imports", Args: []string{"?y", "?x"}},
	}

	info := opt.AnalyzeVariables(atoms)

	if info["?x"] == nil || len(info["?x"]) != 3 {
		t.Errorf("?x indices = %v, want [0,1,2]", info["?x"])
	}
	if info["?y"] == nil || len(info["?y"]) != 2 {
		t.Errorf("?y indices = %v, want [0,2]", info["?y"])
	}
	if info["?z"] == nil || len(info["?z"]) != 1 {
		t.Errorf("?z indices = %v, want [1]", info["?z"])
	}
}

func TestEstimateCost(t *testing.T) {
	opt := NewQueryOptimizer()

	// Multiple atoms vs single atom - more atoms = higher cost
	atoms1 := []Atom{
		{Predicate: "calls", Args: []string{"?x", "?y"}},
	}
	atoms3 := []Atom{
		{Predicate: "calls", Args: []string{"?x", "?y"}},
		{Predicate: "defines", Args: []string{"?y", "?z"}},
		{Predicate: "imports", Args: []string{"?z", "?w"}},
	}

	cost1 := opt.EstimateCost(atoms1)
	cost3 := opt.EstimateCost(atoms3)

	// More atoms should have higher cost
	if cost3 <= cost1 {
		t.Errorf("EstimateCost 3 atoms (%d) should be > 1 atom (%d)", cost3, cost1)
	}
}

func TestPredicatePushdown(t *testing.T) {
	opt := NewQueryOptimizer()

	atoms := []Atom{
		{Predicate: "filter", Args: []string{"?x", "public"}},
		{Predicate: "calls", Args: []string{"?x", "?y"}},
		{Predicate: "defines", Args: []string{"?x", "?z"}},
	}

	optimized, predicates := opt.PredicatePushdown(atoms)

	if len(optimized) != 3 {
		t.Errorf("PredicatePushdown optimized len = %d, want 3", len(optimized))
	}
	if predicates == nil {
		t.Error("PredicatePushdown predicates map is nil")
	}
}

func TestIsBound(t *testing.T) {
	opt := NewQueryOptimizer()

	tests := []struct {
		arg  string
		want bool
	}{
		{"?x", false},     // variable = not bound (unconstrained)
		{"X", true},       // uppercase var = not variable = bound
		{"main.go", true},  // constant = bound
		{`"foo"`, true},   // quoted constant = bound
		{"_", true},       // anonymous = not variable = bound
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			if got := opt.isBound(tt.arg); got != tt.want {
				t.Errorf("isBound(%q) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}