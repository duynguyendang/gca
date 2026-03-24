package server

import (
	"compress/gzip"
	"strings"

	"github.com/gin-gonic/gin"
)

// CompressionMiddleware returns a middleware that compresses responses using gzip.
// It compresses responses for the following content types:
// - application/json
// - text/html
// - text/plain
// - text/css
// - text/javascript
// - application/javascript
// The middleware skips compression for responses that are already compressed.
func CompressionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Don't compress if client doesn't accept gzip
		if !strings.Contains(c.Request.Header.Get("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		// Don't compress if response is already compressed
		if c.Writer.Header().Get("Content-Encoding") != "" {
			c.Next()
			return
		}

		// Create gzip writer
		gz := gzip.NewWriter(c.Writer)
		defer gz.Close()

		// Set compression headers
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")

		// Wrap response writer
		c.Writer = &gzipWriter{Writer: gz, ResponseWriter: c.Writer}
		c.Next()
	}
}

// gzipWriter wraps a gzip.Writer around a gin.ResponseWriter.
type gzipWriter struct {
	gin.ResponseWriter
	Writer *gzip.Writer
}

// Write writes data to the gzip writer.
func (g *gzipWriter) Write(data []byte) (int, error) {
	return g.Writer.Write(data)
}

// WriteString writes a string to the gzip writer.
func (g *gzipWriter) WriteString(s string) (int, error) {
	return g.Writer.Write([]byte(s))
}

// Flush flushes the gzip writer and the underlying response writer.
func (g *gzipWriter) Flush() {
	err := g.Writer.Flush()
	if err != nil {
		return
	}
	g.ResponseWriter.Flush()
}

// shouldCompress determines if a response should be compressed based on content type.
// This function can be extended with more sophisticated logic if needed.
func shouldCompress(contentType string) bool {
	compressibleTypes := []string{
		"application/json",
		"text/html",
		"text/plain",
		"text/css",
		"text/javascript",
		"application/javascript",
		"application/xml",
		"text/xml",
	}

	for _, t := range compressibleTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}
	return false
}
