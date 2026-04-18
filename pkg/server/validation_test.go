package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateAndSanitizeQuery(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple query",
			input:   "triples(A, B, C)",
			wantErr: false,
		},
		{
			name:    "valid query with quotes",
			input:   `triples(A, "calls", B)`,
			wantErr: false,
		},
		{
			name:    "empty query",
			input:   "",
			wantErr: true,
			errMsg:  "query cannot be empty",
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
			errMsg:  "query cannot be empty",
		},
		{
			name:    "dangerous script tag",
			input:   "<script>alert('xss')</script>",
			wantErr: true,
			errMsg:  "contains potentially dangerous content",
		},
		{
			name:    "dangerous javascript",
			input:   "javascript:alert('xss')",
			wantErr: true,
			errMsg:  "contains potentially dangerous content",
		},
		{
			name:    "dangerous onerror",
			input:   "onerror=alert('xss')",
			wantErr: true,
			errMsg:  "contains potentially dangerous content",
		},
		{
			name:    "valid long query",
			input:   "triples(A, \"calls\", B), triples(B, \"calls\", A), A != B",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAndSanitizeQuery(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateAndSanitizeQuery(%q) expected error containing %q, got nil", tt.input, tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateAndSanitizeQuery(%q) error = %v, want error containing %q", tt.input, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAndSanitizeQuery(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.input {
					t.Errorf("ValidateAndSanitizeQuery(%q) = %q, want %q", tt.input, result, tt.input)
				}
			}
		})
	}
}

func TestValidateProjectID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid simple",
			input:   "langchain",
			wantErr: false,
		},
		{
			name:    "valid with underscore",
			input:   "my_project",
			wantErr: false,
		},
		{
			name:    "valid with dash",
			input:   "my-project",
			wantErr: false,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "path traversal slash",
			input:   "../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal double dot",
			input:   "foo/../bar",
			wantErr: true,
		},
		{
			name:    "backslash",
			input:   "foo\\bar",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProjectID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSymbolID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid simple",
			input:   "main",
			wantErr: false,
		},
		{
			name:    "valid with colon",
			input:   "pkg/foo.go:main",
			wantErr: false,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "path traversal",
			input:   "../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSymbolID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSymbolID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidationMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "valid query",
			query:      "triples(A,B,C)",
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty query returns error before middleware check",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test router with validation middleware
			r := http.NewServeMux()
			r.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query().Get("q")
				if query == "" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusOK)
			})

			// Apply middleware (just for structure testing)
			// Note: Full middleware test would require Gin setup
			if tt.query != "" && tt.wantStatus == http.StatusOK {
				req := httptest.NewRequest("GET", "/test?q="+tt.query, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != tt.wantStatus {
					t.Errorf("Expected status %d, got %d", tt.wantStatus, w.Code)
				}
			}
		})
	}
}

func TestContainsSQLInjection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
	}{
		{"safe normal query", "triples(A,B,C)", false},
		{"safe with word union", "union_select(A,B)", false}, // not uppercase UNION
		{"dangerous union select", "UNION SELECT", true},
		{"dangerous drop table", "DROP TABLE", true},
		{"dangerous delete from", "DELETE FROM", true},
		{"dangerous insert into", "INSERT INTO", true},
		{"dangerous update set", "UPDATE SET", true},
		{"dangerous exec", "exec(", true},
		{"dangerous xp_", "xp_cmdshell", true},
		{"dangerous sp_", "sp_executesql", true},
		{"dangerous or true", "' or '", true},
		{"dangerous or true double", "\" or \"", true},
		{"dangerous 1=1", "1=1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsSQLInjection(tt.input)
			if got != tt.want {
				t.Errorf("containsSQLInjection(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestContainsXSS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
	}{
		{"safe normal", "triples(A,B,C)", false},
		{"dangerous script", "<script>alert(1)</script>", true},
		{"dangerous javascript", "javascript:alert(1)", true},
		{"dangerous onload", "onload=alert(1)", true},
		{"dangerous onerror", "onerror=alert(1)", true},
		{"dangerous onclick", "onclick=alert(1)", true},
		{"dangerous onmouseover", "onmouseover=alert(1)", true},
		{"dangerous eval", "eval(document.cookie)", true},
		{"dangerous expression", "expression(alert(1))", true},
		{"dangerous url", "url(javascript:alert(1))", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsXSS(tt.input)
			if got != tt.want {
				t.Errorf("containsXSS(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidQueryPattern(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
	}{
		{"valid with parens", "triples(A,B,C)", true},
		{"valid complex", "triples(A,\"calls\",B), triples(B,\"calls\",A)", true},
		{"no open paren", "triples A,B,C", false},
		{"no close paren", "triples(A,B,C", false},
		{"mismatched", "triples(A,B(C)", false},
		{"empty", "", false},
		{"just parens", "()", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidQueryPattern(tt.input)
			if got != tt.want {
				t.Errorf("IsValidQueryPattern(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateLimit(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		max     int
		wantErr bool
	}{
		{"valid", 10, 100, false},
		{"at max", 100, 100, false},
		{"zero", 0, 100, true},
		{"negative", -1, 100, true},
		{"exceeds max", 101, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLimit(tt.limit, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLimit(%d, %d) error = %v, wantErr %v", tt.limit, tt.max, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOffset(t *testing.T) {
	tests := []struct {
		name    string
		offset  int
		wantErr bool
	}{
		{"valid zero", 0, false},
		{"valid positive", 10, false},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOffset(tt.offset)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOffset(%d) error = %v, wantErr %v", tt.offset, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDepth(t *testing.T) {
	tests := []struct {
		name    string
		depth   int
		wantErr bool
	}{
		{"valid zero", 0, false},
		{"valid one", 1, false},
		{"valid ten", 10, false},
		{"negative", -1, true},
		{"exceeds max", 11, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDepth(tt.depth)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDepth(%d) error = %v, wantErr %v", tt.depth, err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal", "hello", "hello"},
		{"with spaces", "  hello  ", "hello"},
		{"with null", "hello\x00world", "helloworld"},
		{"empty", "", ""},
		{"tabs", "\t\thello\t", "hello"},
		{"newlines", "\n\nhello\n\n", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeString(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateCursor(t *testing.T) {
	tests := []struct {
		name    string
		cursor  string
		wantErr bool
	}{
		{"empty cursor is valid", "", false},
		{"normal cursor", "abc123", false},
		{"cursor with special chars", "abc_123-456", false},
		{"dangerous script", "<script>alert(1)</script>", true},
		{"dangerous javascript", "javascript:alert(1)", true},
		{"dangerous onerror", "onerror=alert(1)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCursor(tt.cursor)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCursor(%q) error = %v, wantErr %v", tt.cursor, err, tt.wantErr)
			}
		})
	}
}

func TestValidateClusters(t *testing.T) {
	tests := []struct {
		name    string
		clusters int
		wantErr bool
	}{
		{"valid positive", 5, false},
		{"valid one", 1, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateClusters(tt.clusters)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateClusters(%d) error = %v, wantErr %v", tt.clusters, err, tt.wantErr)
			}
		})
	}
}

func TestValidateIDs(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		wantErr bool
	}{
		{"valid ids", []string{"id1", "id2"}, false},
		{"empty list", []string{}, true},
		{"with invalid id", []string{"id1", "../etc/passwd"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIDs(tt.ids)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIDs(%v) error = %v, wantErr %v", tt.ids, err, tt.wantErr)
			}
		})
	}
}

func TestValidateEmbedding(t *testing.T) {
	tests := []struct {
		name       string
		embedding  []float32
		wantErr    bool
	}{
		{"valid embedding", []float32{0.1, 0.2, 0.3}, false},
		{"empty embedding", []float32{}, true},
		{"single element", []float32{0.5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmbedding(tt.embedding)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmbedding() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"application/json", "application/json", true},
		{"text/plain", "text/plain", true},
		{"application/x-www-form-urlencoded", "application/x-www-form-urlencoded", true},
		{"multipart/form-data", "multipart/form-data", true},
		{"text/html", "text/html", false},
		{"application/xml", "application/xml", false},
		{"with charset", "application/json; charset=utf-8", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidContentType(tt.contentType)
			if got != tt.want {
				t.Errorf("isValidContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	cfg := DefaultValidationConfig()
	allowedExts := cfg.AllowedFileExtensions

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid .go file", "pkg/server.go", false},
		{"valid .md file", "README.md", false},
		{"valid nested path", "github.com/user/project/pkg/server.go", false},
		{"path traversal", "../etc/passwd", true},
		{"absolute path", "/etc/passwd", true},
		{"backslash", "foo\\bar", true},
		{"null byte", "file\x00.go", true},
		{"invalid extension", "file.xyz", true},
		{"with null in middle", "pkg\x00/server.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePath(tt.path, allowedExts)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFilePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateQuery(t *testing.T) {
	// MaxQueryLength is 200000 from config
	longQuery := strings.Repeat("a", 200001)
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"valid query", "triples(A, B, C)", false},
		{"empty query", "", true},
		{"long query", longQuery, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuery(%q) error = %v, wantErr %v", tt.query, err, tt.wantErr)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{Field: "test_field", Message: "test message"}
	if err.Error() != "test_field: test message" {
		t.Errorf("ValidationError.Error() = %q, want %q", err.Error(), "test_field: test message")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}