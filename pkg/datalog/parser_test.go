package datalog

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    []Atom
		wantErr bool
	}{
		{
			name:  "Simple Triple",
			query: `triples(A, "calls", B)`,
			want: []Atom{
				{Predicate: "triples", Args: []string{"A", "calls", "B"}},
			},
		},
		{
			name:  "Multiple Triples",
			query: `triples(A, "calls", B), triples(B, "calls", C)`,
			want: []Atom{
				{Predicate: "triples", Args: []string{"A", "calls", "B"}},
				{Predicate: "triples", Args: []string{"B", "calls", "C"}},
			},
		},
		{
			name:  "Inequality Sugar",
			query: `triples(A, "calls", B), A != B`,
			want: []Atom{
				{Predicate: "triples", Args: []string{"A", "calls", "B"}},
				{Predicate: "neq", Args: []string{"A", "B"}},
			},
		},
		{
			name:  "Regex Constraint",
			query: `triples(A, "calls", B), regex(A, ".*Service")`,
			want: []Atom{
				{Predicate: "triples", Args: []string{"A", "calls", "B"}},
				{Predicate: "regex", Args: []string{"A", ".*Service"}},
			},
		},
		{
			name:  "Complex Mix",
			query: `triples(A, "calls", B), triples(B, "calls", C), regex(A, "foo"), A != C`,
			want: []Atom{
				{Predicate: "triples", Args: []string{"A", "calls", "B"}},
				{Predicate: "triples", Args: []string{"B", "calls", "C"}},
				{Predicate: "regex", Args: []string{"A", "foo"}},
				{Predicate: "neq", Args: []string{"A", "C"}},
			},
		},
		{
			name:  "Quoted Args Handling",
			query: `triples(A, 'calls', "B")`,
			want: []Atom{
				{Predicate: "triples", Args: []string{"A", "calls", "B"}},
			},
		},
		{
			name:    "Invalid Syntax",
			query:   `triples(A, B`,
			wantErr: true,
		},
		{
			name:    "Empty Query",
			query:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSmartSplit(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{`a, "b,c", d`, []string{"a", "\"b,c\"", "d"}},
		{`fn(a,b), c`, []string{"fn(a,b)", "c"}},
		{`triples(A, "calls", B), A != B`, []string{`triples(A, "calls", B)`, `A != B`}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SmartSplit(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SmartSplit(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
