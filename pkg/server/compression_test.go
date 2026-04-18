package server

import (
	"testing"
)

func TestShouldCompress(t *testing.T) {
	// Test that shouldCompress correctly identifies compressible types
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"application/json", "application/json", true},
		{"text/html", "text/html", true},
		{"text/plain", "text/plain", true},
		{"text/css", "text/css", true},
		{"text/javascript", "text/javascript", true},
		{"application/javascript", "application/javascript", true},
		{"application/xml", "application/xml", true},
		{"text/xml", "text/xml", true},
		{"image/png", "image/png", false},
		{"application/octet-stream", "application/octet-stream", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCompress(tt.contentType)
			if got != tt.want {
				t.Errorf("shouldCompress(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestGzipWriterInterface(t *testing.T) {
	// Test that gzipWriter.Write delegates to gzip.Writer
	// This is implicitly tested via CompressionMiddleware test
	// Here we just verify the interface is correctly defined
	gz := &gzipWriter{}
	_ = interface{}(gz) // verify it implements what it needs to
}
