package common

import (
	"testing"
)

func TestFormatNodesWithCode(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		limit int
		want  string
	}{
		{
			name:  "nil data",
			data:  nil,
			limit: 10,
			want:  "",
		},
		{
			name:  "non-list data",
			data:  "not a list",
			limit: 10,
			want:  "",
		},
		{
			name: "valid data",
			data: []interface{}{
				map[string]interface{}{
					"id":   "main.go:main",
					"name": "main",
					"kind": "func",
					"code": "func main() {}",
				},
			},
			limit: 10,
			want:  "## Query Results:\n\n### 1. main.go:main\nName: main\nType: func\n```\nfunc main() {}\n```\n\n",
		},
		{
			name: "data with limit",
			data: []interface{}{
				map[string]interface{}{"id": "a", "name": "A", "kind": "f", "code": "A"},
				map[string]interface{}{"id": "b", "name": "B", "kind": "f", "code": "B"},
				map[string]interface{}{"id": "c", "name": "C", "kind": "f", "code": "C"},
			},
			limit: 2,
			want:  "## Query Results:\n\n### 1. a\nName: A\nType: f\n```\nA\n```\n\n### 2. b\nName: B\nType: f\n```\nB\n```\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatNodesWithCode(tt.data, tt.limit)
			if got != tt.want {
				t.Errorf("FormatNodesWithCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatNodesSimple(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		limit int
		want  string
	}{
		{
			name:  "nil data",
			data:  nil,
			limit: 10,
			want:  "",
		},
		{
			name: "valid data",
			data: []interface{}{
				map[string]interface{}{"name": "main", "kind": "func"},
				map[string]interface{}{"name": "helper", "kind": "func"},
			},
			limit: 10,
			want:  "- main (func)\n- helper (func)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatNodesSimple(tt.data, tt.limit)
			if got != tt.want {
				t.Errorf("FormatNodesSimple() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatPredicatesList(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		want  string
	}{
		{
			name:  "string passthrough",
			data:  "already formatted",
			want:  "already formatted",
		},
		{
			name:  "nil data",
			data:  nil,
			want:  "",
		},
		{
			name:  "non-list data",
			data:  123,
			want:  "",
		},
		{
			name: "valid predicates",
			data: []interface{}{"calls", "defines", "imports"},
			want: "- `calls`\n- `defines`\n- `imports`\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPredicatesList(tt.data)
			if got != tt.want {
				t.Errorf("FormatPredicatesList() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatNodeList(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		want  string
	}{
		{
			name:  "string passthrough",
			data:  "already formatted",
			want:  "already formatted",
		},
		{
			name:  "nil data",
			data:  nil,
			want:  "",
		},
		{
			name: "valid nodes",
			data: []interface{}{
				map[string]interface{}{"name": "main", "kind": "func", "id": "main.go:main"},
			},
			want: "- main (Kind: func, ID: main.go:main)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatNodeList(tt.data)
			if got != tt.want {
				t.Errorf("FormatNodeList() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractNodeNames(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		want  string
	}{
		{
			name:  "nil data",
			data:  nil,
			want:  "",
		},
		{
			name: "valid nodes",
			data: []interface{}{
				map[string]interface{}{"name": "main"},
				map[string]interface{}{"name": "helper"},
				map[string]interface{}{"name": "init"},
			},
			want: "main, helper, init",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractNodeNames(tt.data)
			if got != tt.want {
				t.Errorf("ExtractNodeNames() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractStringList(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		limit int
		want  string
	}{
		{
			name:  "nil data",
			data:  nil,
			limit: 10,
			want:  "",
		},
		{
			name: "valid strings",
			data: []interface{}{"a", "b", "c"},
			limit: 2,
			want:  "a, b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractStringList(tt.data, tt.limit)
			if got != tt.want {
				t.Errorf("ExtractStringList() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractPathString(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		want  string
	}{
		{
			name:  "nil data",
			data:  nil,
			want:  "",
		},
		{
			name: "valid path",
			data: []interface{}{
				map[string]interface{}{"id": "A"},
				map[string]interface{}{"id": "B"},
				map[string]interface{}{"id": "C"},
			},
			want: " -> A -> B -> C",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPathString(tt.data)
			if got != tt.want {
				t.Errorf("ExtractPathString() = %q, want %q", got, tt.want)
			}
		})
	}
}
