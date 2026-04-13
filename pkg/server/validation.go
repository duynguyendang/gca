package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/gin-gonic/gin"
)

// ValidationConfig holds configuration for input validation
type ValidationConfig struct {
	// MaxQueryLength is the maximum allowed length for query parameters
	MaxQueryLength int
	// MaxBodySize is the maximum allowed size for request bodies (in bytes)
	MaxBodySize int64
	// AllowedFileExtensions is a list of allowed file extensions for file paths
	AllowedFileExtensions []string
	// SanitizeHTML determines whether to sanitize HTML in inputs
	SanitizeHTML bool
}

// DefaultValidationConfig returns a default validation configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxQueryLength: 10000,
		MaxBodySize:    10 * 1024 * 1024, // 10MB
		AllowedFileExtensions: []string{
			".go", ".py", ".js", ".ts", ".jsx", ".tsx",
			".java", ".c", ".cpp", ".h", ".hpp",
			".rs", ".rb", ".php", ".swift", ".kt",
			".md", ".txt", ".json", ".yaml", ".yml",
			".xml", ".html", ".css", ".sql", ".sh",
		},
		SanitizeHTML: true,
	}
}

// ValidationMiddleware returns a Gin middleware for input validation and sanitization
func ValidationMiddleware(config ...ValidationConfig) gin.HandlerFunc {
	cfg := DefaultValidationConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *gin.Context) {
		// Validate and sanitize query parameters
		if err := validateQueryParams(c, cfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid query parameters",
				"details": err.Error(),
			})
			c.Abort()
			return
		}

		// Validate request body size
		if c.Request.ContentLength > cfg.MaxBodySize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "Request body too large",
			})
			c.Abort()
			return
		}

		// Validate Content-Type for POST/PUT requests
		if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut {
			contentType := c.GetHeader("Content-Type")
			if contentType != "" && !isValidContentType(contentType) {
				c.JSON(http.StatusUnsupportedMediaType, gin.H{
					"error": "Unsupported content type",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// validateQueryParams validates and sanitizes query parameters
func validateQueryParams(c *gin.Context, cfg ValidationConfig) error {
	for key, values := range c.Request.URL.Query() {
		for _, value := range values {
			// Check length
			if len(value) > cfg.MaxQueryLength {
				return &ValidationError{
					Field:   key,
					Message: "exceeds maximum length",
				}
			}

			// Sanitize the value
			sanitized := sanitizeInput(value)

			// Check for path traversal attempts
			if key == "path" || key == "file" || key == "file_path" || strings.Contains(key, "path") {
				if err := validateFilePath(sanitized, cfg.AllowedFileExtensions); err != nil {
					return &ValidationError{
						Field:   key,
						Message: err.Error(),
					}
				}
			}

			// Check for SQL injection patterns
			if containsSQLInjection(sanitized) {
				return &ValidationError{
					Field:   key,
					Message: "contains potentially dangerous content",
				}
			}

			// Check for XSS patterns
			if containsXSS(sanitized) {
				return &ValidationError{
					Field:   key,
					Message: "contains potentially dangerous content",
				}
			}
		}
	}
	return nil
}

// sanitizeInput trims whitespace and removes null bytes.
// HTML escaping is intentionally omitted — JSON encoding handles output safety,
// and escaping would corrupt structured inputs like Datalog queries.
func sanitizeInput(input string) string {
	sanitized := strings.TrimSpace(input)
	sanitized = strings.ReplaceAll(sanitized, "\x00", "")
	return sanitized
}

// validateFilePath validates file paths to prevent path traversal
func validateFilePath(path string, allowedExtensions []string) error {
	if strings.Contains(path, "..") {
		return &ValidationError{Field: "path", Message: "path traversal detected"}
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return &ValidationError{Field: "path", Message: "absolute paths are not allowed"}
	}
	if strings.Contains(path, "\x00") {
		return &ValidationError{Field: "path", Message: "invalid path format"}
	}
	if len(allowedExtensions) > 0 {
		hasValidExtension := false
		for _, ext := range allowedExtensions {
			if strings.HasSuffix(strings.ToLower(path), ext) {
				hasValidExtension = true
				break
			}
		}
		if !hasValidExtension {
			return &ValidationError{Field: "path", Message: "file extension not allowed"}
		}
	}
	return nil
}

// containsSQLInjection checks for common SQL injection patterns
func containsSQLInjection(input string) bool {
	lowercase := strings.ToLower(input)
	patterns := []string{
		"union select",
		"drop table",
		"delete from",
		"insert into",
		"update set",
		"exec(",
		"execute(",
		"xp_",
		"sp_",
		";--",
		"' or '",
		"\" or \"",
		"1=1",
		"1 = 1",
	}
	for _, pattern := range patterns {
		if strings.Contains(lowercase, pattern) {
			return true
		}
	}
	return false
}

// containsXSS checks for common XSS patterns
func containsXSS(input string) bool {
	lowercase := strings.ToLower(input)
	patterns := []string{
		"<script",
		"javascript:",
		"onload=",
		"onerror=",
		"onclick=",
		"onmouseover=",
		"onfocus=",
		"onblur=",
		"onchange=",
		"onsubmit=",
		"eval(",
		"expression(",
		"url(",
		"document.cookie",
		"document.write",
		"window.location",
	}
	for _, pattern := range patterns {
		if strings.Contains(lowercase, pattern) {
			return true
		}
	}
	return false
}

// isValidContentType checks if the content type is allowed
func isValidContentType(contentType string) bool {
	allowedTypes := []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"text/plain",
	}
	mainType := strings.Split(contentType, ";")[0]
	mainType = strings.TrimSpace(mainType)
	for _, allowed := range allowedTypes {
		if strings.EqualFold(mainType, allowed) {
			return true
		}
	}
	return false
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// SanitizeString sanitizes a string input by trimming whitespace and removing null bytes.
// No HTML escaping is applied — callers should escape output at the rendering layer.
func SanitizeString(input string) string {
	return sanitizeInput(input)
}

// ValidateProjectID validates a project ID.
// Allows any printable characters except path traversal patterns.
func ValidateProjectID(projectID string) error {
	if projectID == "" {
		return &ValidationError{Field: "project_id", Message: "is required"}
	}
	if len(projectID) > config.MaxProjectIDLength {
		return &ValidationError{Field: "project_id", Message: "exceeds maximum length"}
	}
	// Check for path traversal attempts
	if strings.Contains(projectID, "..") || strings.Contains(projectID, "/") || strings.Contains(projectID, "\\") {
		return &ValidationError{Field: "project_id", Message: "contains invalid characters"}
	}
	return nil
}

// ValidateSymbolID validates a symbol ID
func ValidateSymbolID(symbolID string) error {
	if symbolID == "" {
		return &ValidationError{Field: "symbol_id", Message: "is required"}
	}
	// Check for path traversal attempts
	if strings.Contains(symbolID, "..") {
		return &ValidationError{Field: "symbol_id", Message: "contains invalid characters"}
	}
	if len(symbolID) > config.MaxSymbolIDLength {
		return &ValidationError{Field: "symbol_id", Message: "exceeds maximum length"}
	}
	return nil
}

// ValidateAndSanitizeQuery validates a Datalog query string.
// It trims whitespace, checks length, and rejects dangerous content,
// but does NOT HTML-escape the query — that would corrupt Datalog syntax.
func ValidateAndSanitizeQuery(query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	query = strings.TrimSpace(query)

	if len(query) > config.MaxQueryLength {
		return "", fmt.Errorf("query exceeds maximum length of %d characters", config.MaxQueryLength)
	}

	// Check for potentially dangerous patterns without escaping the query
	dangerousPatterns := []string{
		"<script",
		"javascript:",
		"onload=",
		"onerror=",
		"onclick=",
	}

	lowerQuery := strings.ToLower(query)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerQuery, pattern) {
			return "", fmt.Errorf("query contains potentially dangerous content")
		}
	}

	return query, nil
}

// ValidateIDs validates a list of IDs
func ValidateIDs(ids []string) error {
	if len(ids) == 0 {
		return &ValidationError{Field: "ids", Message: "list cannot be empty"}
	}
	if len(ids) > config.MaxIDsCount {
		return &ValidationError{Field: "ids", Message: fmt.Sprintf("too many (maximum %d)", config.MaxIDsCount)}
	}
	for _, id := range ids {
		if err := ValidateSymbolID(id); err != nil {
			return err
		}
	}
	return nil
}

// ValidateEmbedding validates an embedding vector
func ValidateEmbedding(embedding []float32) error {
	if len(embedding) == 0 {
		return &ValidationError{Field: "embedding", Message: "cannot be empty"}
	}
	if len(embedding) > config.MaxEmbeddingDim {
		return &ValidationError{Field: "embedding", Message: "dimensions exceed maximum"}
	}
	return nil
}

// ValidateLimit validates a limit parameter
func ValidateLimit(limit int, maxLimit int) error {
	if limit <= 0 {
		return &ValidationError{Field: "limit", Message: "must be positive"}
	}
	if limit > maxLimit {
		return &ValidationError{Field: "limit", Message: fmt.Sprintf("exceeds maximum of %d", maxLimit)}
	}
	return nil
}

// ValidateOffset validates an offset parameter
func ValidateOffset(offset int) error {
	if offset < 0 {
		return &ValidationError{Field: "offset", Message: "cannot be negative"}
	}
	if offset > config.MaxOffset {
		return &ValidationError{Field: "offset", Message: "exceeds maximum"}
	}
	return nil
}

// ValidateCursor validates a cursor string
func ValidateCursor(cursor string) error {
	if cursor == "" {
		return nil
	}
	if len(cursor) > config.MaxCursorLength {
		return &ValidationError{Field: "cursor", Message: "exceeds maximum length"}
	}
	dangerousPatterns := []string{
		"<script",
		"javascript:",
		"onload=",
		"onerror=",
	}
	lowerCursor := strings.ToLower(cursor)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCursor, pattern) {
			return &ValidationError{Field: "cursor", Message: "contains potentially dangerous content"}
		}
	}
	return nil
}

// ValidateDepth validates a depth parameter
func ValidateDepth(depth int) error {
	if depth < 0 {
		return &ValidationError{Field: "depth", Message: "cannot be negative"}
	}
	if depth > config.MaxDepth {
		return &ValidationError{Field: "depth", Message: fmt.Sprintf("exceeds maximum of %d", config.MaxDepth)}
	}
	return nil
}

// ValidateClusters validates a clusters parameter
func ValidateClusters(clusters int) error {
	if clusters <= 0 {
		return &ValidationError{Field: "clusters", Message: "must be positive"}
	}
	if clusters > config.MaxClusters {
		return &ValidationError{Field: "clusters", Message: fmt.Sprintf("exceeds maximum of %d", config.MaxClusters)}
	}
	return nil
}

// IsValidQueryPattern checks if a query pattern has balanced parentheses (for Datalog)
func IsValidQueryPattern(query string) bool {
	if !strings.Contains(query, "(") || !strings.Contains(query, ")") {
		return false
	}
	count := 0
	for _, char := range query {
		if char == '(' {
			count++
		} else if char == ')' {
			count--
		}
		if count < 0 {
			return false
		}
	}
	return count == 0
}

// ValidateQuery validates a query string
func ValidateQuery(query string) error {
	if query == "" {
		return &ValidationError{Field: "query", Message: "is required"}
	}
	if len(query) > config.MaxQueryLength {
		return &ValidationError{Field: "query", Message: "exceeds maximum length"}
	}
	return nil
}
