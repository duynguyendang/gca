package datalog

import (
	"testing"
)

func TestParseEnhanced(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		wantAtomCount  int
		wantLimit      *int
		wantSortCount  int
		wantGroupCount int
	}{
		{
			name:          "Simple Query with Limit",
			query:         `triples(?s, "calls", ?o) LIMIT 10`,
			wantAtomCount: 1,
			wantLimit:     intPtr(10),
		},
		{
			name:          "Query with Offset",
			query:         `triples(?s, "calls", ?o) OFFSET 5`,
			wantAtomCount: 1,
			wantLimit:     nil,
		},
		{
			name:          "Query with Order By",
			query:         `triples(?s, "calls", ?o) ORDER BY ?s ASC`,
			wantAtomCount: 1,
			wantSortCount: 1,
		},
		{
			name:          "Query with Order By Desc",
			query:         `triples(?s, "calls", ?o) ORDER BY ?s DESC`,
			wantAtomCount: 1,
			wantSortCount: 1,
		},
		{
			name:           "Query with Group By",
			query:          `triples(?s, "calls", ?o) GROUP BY ?s`,
			wantAtomCount:  1,
			wantGroupCount: 1,
		},
		{
			name:           "Query with Count Aggregation",
			query:          `triples(?s, "calls", ?o) GROUP BY ?s COUNT(?o) AS count`,
			wantAtomCount:  1,
			wantGroupCount: 1,
		},
		{
			name:           "Complex Query with Multiple Modifiers",
			query:          `triples(?s, "calls", ?o) GROUP BY ?s ORDER BY ?s ASC LIMIT 5`,
			wantAtomCount:  1,
			wantLimit:      intPtr(5),
			wantSortCount:  1,
			wantGroupCount: 1,
		},
		{
			name:          "Query with Sum Aggregation",
			query:         `triples(?s, "has_value", ?v) SUM(?v) AS total`,
			wantAtomCount: 1,
		},
		{
			name:          "Query with Min Max",
			query:         `triples(?s, "has_score", ?score) MIN(?score) AS minScore MAX(?score) AS maxScore`,
			wantAtomCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pq, err := ParseEnhanced(tt.query)
			if err != nil {
				t.Errorf("ParseEnhanced() error = %v", err)
				return
			}
			if len(pq.Atoms) != tt.wantAtomCount {
				t.Errorf("ParseEnhanced() got %d atoms, want %d", len(pq.Atoms), tt.wantAtomCount)
			}
			if tt.wantLimit != nil {
				if pq.Modifier.Limit == nil || *pq.Modifier.Limit != *tt.wantLimit {
					t.Errorf("ParseEnhanced() got limit %v, want %v", pq.Modifier.Limit, tt.wantLimit)
				}
			}
			if len(pq.Modifier.SortBy) != tt.wantSortCount {
				t.Errorf("ParseEnhanced() got %d sort specs, want %d", len(pq.Modifier.SortBy), tt.wantSortCount)
			}
			if len(pq.Modifier.GroupBy) != tt.wantGroupCount {
				t.Errorf("ParseEnhanced() got %d group by vars, want %d", len(pq.Modifier.GroupBy), tt.wantGroupCount)
			}
		})
	}
}

func TestApplyModifiers(t *testing.T) {
	tests := []struct {
		name      string
		results   []map[string]any
		modifier  *QueryModifier
		wantCount int
	}{
		{
			name: "Limit applies correctly",
			results: []map[string]any{
				{"?s": "a"}, {"?s": "b"}, {"?s": "c"}, {"?s": "d"},
			},
			modifier:  &QueryModifier{Limit: intPtr(2)},
			wantCount: 2,
		},
		{
			name: "Offset applies correctly",
			results: []map[string]any{
				{"?s": "a"}, {"?s": "b"}, {"?s": "c"}, {"?s": "d"},
			},
			modifier:  &QueryModifier{Offset: intPtr(2)},
			wantCount: 2,
		},
		{
			name: "Sort Ascending",
			results: []map[string]any{
				{"?s": "d"}, {"?s": "b"}, {"?s": "a"}, {"?s": "c"},
			},
			modifier: &QueryModifier{
				SortBy: []SortSpec{{Variable: "?s", Direction: SortAsc}},
			},
			wantCount: 4,
		},
		{
			name: "Sort Descending",
			results: []map[string]any{
				{"?s": "a"}, {"?s": "c"}, {"?s": "b"}, {"?s": "d"},
			},
			modifier: &QueryModifier{
				SortBy: []SortSpec{{Variable: "?s", Direction: SortDesc}},
			},
			wantCount: 4,
		},
		{
			name: "Aggregation Count",
			results: []map[string]any{
				{"?s": "a", "?o": "1"},
				{"?s": "a", "?o": "2"},
				{"?s": "b", "?o": "3"},
			},
			modifier: &QueryModifier{
				GroupBy: []string{"?s"},
				Aggregation: &AggregationSpec{
					Type:     AggregationCount,
					Variable: "?o",
					Alias:    "cnt",
				},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyModifiers(tt.results, tt.modifier)
			if len(got) != tt.wantCount {
				t.Errorf("ApplyModifiers() got %d results, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestApplyModifiers_SortOrder(t *testing.T) {
	results := []map[string]any{
		{"?val": float64(3)},
		{"?val": float64(1)},
		{"?val": float64(2)},
	}

	modifier := &QueryModifier{
		SortBy: []SortSpec{{Variable: "?val", Direction: SortAsc}},
	}

	got := ApplyModifiers(results, modifier)

	if got[0]["?val"].(float64) != 1 || got[2]["?val"].(float64) != 3 {
		t.Errorf("Sort not applied correctly: %v", got)
	}
}

func TestApplyModifiers_AggregationSum(t *testing.T) {
	results := []map[string]any{
		{"?category": "A", "?amount": 10},
		{"?category": "A", "?amount": 20},
		{"?category": "B", "?amount": 30},
	}

	modifier := &QueryModifier{
		GroupBy: []string{"?category"},
		Aggregation: &AggregationSpec{
			Type:     AggregationSum,
			Variable: "?amount",
			Alias:    "total",
		},
	}

	got := ApplyModifiers(results, modifier)

	if len(got) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(got))
	}

	var aTotal, bTotal float64
	for _, r := range got {
		if cat, ok := r["?category"].(string); ok {
			if total, ok := r["?total"].(float64); ok {
				if cat == "A" {
					aTotal = total
				} else if cat == "B" {
					bTotal = total
				}
			}
		}
	}

	if aTotal != 30 {
		t.Errorf("Expected category A sum = 30, got %v", aTotal)
	}
	if bTotal != 30 {
		t.Errorf("Expected category B sum = 30, got %v", bTotal)
	}
}

func intPtr(i int) *int {
	return &i
}
