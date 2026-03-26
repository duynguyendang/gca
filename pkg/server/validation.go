package server

import (
	"html"
	"net/http"
	"regexp"
	"strings"

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
			sanitized := sanitizeInput(value, cfg.SanitizeHTML)

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

// sanitizeInput sanitizes user input
func sanitizeInput(input string, sanitizeHTML bool) string {
	// Trim whitespace
	sanitized := strings.TrimSpace(input)

	// Remove null bytes
	sanitized = strings.ReplaceAll(sanitized, "\x00", "")

	// Sanitize HTML if enabled
	if sanitizeHTML {
		sanitized = html.EscapeString(sanitized)
	}

	return sanitized
}

// validateFilePath validates file paths to prevent path traversal
func validateFilePath(path string, allowedExtensions []string) error {
	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return &ValidationError{
			Field:   "path",
			Message: "path traversal detected",
		}
	}

	// Check for absolute paths
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return &ValidationError{
			Field:   "path",
			Message: "absolute paths are not allowed",
		}
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return &ValidationError{
			Field:   "path",
			Message: "invalid path format",
		}
	}

	// Validate file extension if specified
	if len(allowedExtensions) > 0 {
		hasValidExtension := false
		for _, ext := range allowedExtensions {
			if strings.HasSuffix(strings.ToLower(path), ext) {
				hasValidExtension = true
				break
			}
		}
		if !hasValidExtension {
			return &ValidationError{
				Field:   "path",
				Message: "file extension not allowed",
			}
		}
	}

	return nil
}

// containsSQLInjection checks for common SQL injection patterns
func containsSQLInjection(input string) bool {
	lowercase := strings.ToLower(input)

	// Common SQL injection patterns
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

	// Common XSS patterns
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

	// Extract the main content type (before any parameters)
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

// SanitizeString sanitizes a string input
func SanitizeString(input string) string {
	return sanitizeInput(input, true)
}

// ValidateProjectID validates a project ID
func ValidateProjectID(projectID string) error {
	if projectID == "" {
		return &ValidationError{Field: "project_id", Message: "is required"}
	}

	if len(projectID) > 255 {
		return &ValidationError{Field: "project_id", Message: "exceeds maximum length"}
	}

	// Project ID should only contain alphanumeric characters, hyphens, and underscores
	validProjectID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validProjectID.MatchString(projectID) {
		return &ValidationError{Field: "project_id", Message: "contains invalid characters"}
	}

	return nil
}

// ValidateQuery validates a query string
func ValidateQuery(query string) error {
	if query == "" {
		return &ValidationError{Field: "query", Message: "is required"}
	}

	if len(query) > 10000 {
		return &ValidationError{Field: "query", Message: "exceeds maximum length"}
	}

	return nil
}
