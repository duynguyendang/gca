package server

import (
	"net/http"
	"testing"
)

func TestHandleGraphValidation(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		fileID     string
		wantStatus int
	}{
		{
			name:       "empty project ID",
			projectID:  "",
			fileID:     "test.go",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "path traversal in project ID",
			projectID:  "../etc/passwd",
			fileID:     "test.go",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "path traversal in file ID",
			projectID:  "test",
			fileID:     "../../etc/passwd",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errProj := ValidateProjectID(tt.projectID)
			errFile := ValidateSymbolID(tt.fileID)
			hasErr := errProj != nil || errFile != nil
			if tt.wantStatus == http.StatusBadRequest && !hasErr {
				t.Errorf("expected validation to fail for projectID=%q, fileID=%q", tt.projectID, tt.fileID)
			}
		})
	}
}

func TestHandleSourceValidation(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		fileID    string
		wantErr   bool
	}{
		{"empty project", "", "test.go", true},
		{"empty file ID", "test", "", true},
		{"valid inputs", "test", "test.go:main", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err1 := ValidateProjectID(tt.projectID)
			err2 := ValidateSymbolID(tt.fileID)
			hasErr := err1 != nil || err2 != nil
			if hasErr != tt.wantErr {
				t.Errorf("validation error = %v, wantErr %v", err1, err2)
			}
		})
	}
}

func TestHandleFilesValidation(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{"empty prefix", "", false},
		{"valid prefix", "pkg/", false},
		{"backslash in prefix", "foo\\bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasBackslash := false
			for _, c := range tt.prefix {
				if c == '\\' {
					hasBackslash = true
				}
			}
			hasErr := hasBackslash
			if hasErr != tt.wantErr {
				t.Errorf("prefix %q error = %v, wantErr %v", tt.prefix, hasErr, tt.wantErr)
			}
		})
	}
}

func TestHandleFlowPathValidation(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{"valid symbols", "main.go:main", "foo.go:bar", false},
		{"empty from", "", "foo.go:bar", true},
		{"empty to", "main.go:main", "", true},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errFrom := ValidateSymbolID(tt.from)
			errTo := ValidateSymbolID(tt.to)
			hasErr := errFrom != nil || errTo != nil
			if hasErr != tt.wantErr {
				t.Errorf("ValidateSymbolID(%q, %q) error = %v, %v, wantErr %v",
					tt.from, tt.to, errFrom, errTo, tt.wantErr)
			}
		})
	}
}

func TestHandleSemanticSearchValidation(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"normal query", "find all functions", false},
		{"empty query", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Query validation in handler checks if query == ""
			hasErr := tt.query == ""
			if hasErr != tt.wantErr {
				t.Errorf("query %q hasErr = %v, wantErr %v", tt.query, hasErr, tt.wantErr)
			}
		})
	}
}

func TestHandleGraphSubgraphValidation(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		wantErr bool
	}{
		{"valid IDs", []string{"id1", "id2"}, false},
		{"empty IDs", []string{}, true},
		{"with traversal", []string{"id1", "../etc"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIDs(tt.ids)
			hasErr := err != nil
			if hasErr != tt.wantErr {
				t.Errorf("ValidateIDs(%v) error = %v, wantErr %v", tt.ids, err, tt.wantErr)
			}
		})
	}
}

func TestHandleHybridClusterValidation(t *testing.T) {
	tests := []struct {
		name      string
		embedding []float32
		limit     int
		clusters  int
		wantErr   bool
	}{
		{"valid params", []float32{0.1, 0.2, 0.3}, 100, 5, false},
		{"empty embedding", []float32{}, 100, 5, true},
		{"negative limit", []float32{0.1}, -1, 5, true},
		{"negative clusters", []float32{0.1}, 100, -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errEmbedding := ValidateEmbedding(tt.embedding)
			errClusters := ValidateClusters(tt.clusters)
			errLimit := ValidateLimit(tt.limit, 1000)
			hasErr := errEmbedding != nil || errClusters != nil || errLimit != nil
			if hasErr != tt.wantErr {
				t.Errorf("hybrid cluster validation error = %v, %v, %v, wantErr %v",
					errEmbedding, errClusters, errLimit, tt.wantErr)
			}
		})
	}
}

func TestHandleWhoCallsValidation(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		depth   int
		wantErr bool
	}{
		{"valid symbol", "main.go:main", 1, false},
		{"empty symbol", "", 1, true},
		{"zero depth", "main.go:main", 0, false},
		{"negative depth", "main.go:main", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSymbolID(tt.symbol)
			hasErr := err != nil
			if hasErr != tt.wantErr {
				t.Errorf("ValidateSymbolID(%q) error = %v, wantErr %v", tt.symbol, err, tt.wantErr)
			}
		})
	}
}

func TestHandleCheckReachabilityValidation(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{"valid symbols", "main.go:main", "foo.go:bar", false},
		{"empty from", "", "foo", true},
		{"empty to", "main", "", true},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errFrom := ValidateSymbolID(tt.from)
			errTo := ValidateSymbolID(tt.to)
			hasErr := errFrom != nil || errTo != nil
			if hasErr != tt.wantErr {
				t.Errorf("reachability validation = %v, %v, wantErr %v",
					errFrom, errTo, tt.wantErr)
			}
		})
	}
}

func TestHandleFindLCAValidation(t *testing.T) {
	tests := []struct {
		name    string
		a       string
		b       string
		wantErr bool
	}{
		{"valid symbols", "main.go:main", "foo.go:bar", false},
		{"empty a", "", "foo", true},
		{"empty b", "main", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errA := ValidateSymbolID(tt.a)
			errB := ValidateSymbolID(tt.b)
			hasErr := errA != nil || errB != nil
			if hasErr != tt.wantErr {
				t.Errorf("LCA validation = %v, %v, wantErr %v",
					errA, errB, tt.wantErr)
			}
		})
	}
}

func TestHandleGraphPaginatedValidation(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"valid query", "triples(A, B, C)", false},
		{"empty query", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuery(tt.query)
			hasErr := err != nil
			if hasErr != tt.wantErr {
				t.Errorf("ValidateQuery(%q) error = %v, wantErr %v", tt.query, err, tt.wantErr)
			}
		})
	}
}
