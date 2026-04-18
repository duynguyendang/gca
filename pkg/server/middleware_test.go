package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		wantStatus     int
		wantCORSHeader bool
	}{
		{
			name:           "allowed origin",
			origin:         "http://localhost:3000",
			wantStatus:     http.StatusOK,
			wantCORSHeader: true,
		},
		{
			name:           "non-allowed origin",
			origin:         "http://evil.com",
			wantStatus:     http.StatusOK,
			wantCORSHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(CORSMiddleware())
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Origin", tt.origin)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			hasCORS := w.Header().Get("Access-Control-Allow-Origin") != ""
			if hasCORS != tt.wantCORSHeader {
				t.Errorf("CORS header = %v, want %v", hasCORS, tt.wantCORSHeader)
			}
		})
	}
}

func TestCORSMiddlewarePreflight(t *testing.T) {
	r := gin.New()
	r.Use(CORSMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestCompressionMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		acceptEncoding string
		wantCompressed bool
	}{
		{
			name:           "with gzip",
			acceptEncoding: "gzip",
			wantCompressed: true,
		},
		{
			name:           "without compression",
			acceptEncoding: "",
			wantCompressed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(CompressionMiddleware())
			r.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "Hello, World!")
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if tt.wantCompressed {
				ce := w.Header().Get("Content-Encoding")
				if ce != "gzip" {
					t.Errorf("Content-Encoding = %q, want %q", ce, "gzip")
				}
			}
		})
	}
}

func TestDefaultCORSConfig(t *testing.T) {
	cfg := DefaultCORSConfig()

	if len(cfg.AllowOrigins) == 0 {
		t.Error("AllowOrigins is empty")
	}
	if len(cfg.AllowMethods) == 0 {
		t.Error("AllowMethods is empty")
	}
	if cfg.MaxAge <= 0 {
		t.Error("MaxAge should be positive")
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check response header was set by middleware
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("X-Request-ID header not set in response")
	}

	// Should be a valid UUID
	if len(requestID) != 36 {
		t.Errorf("X-Request-ID = %q, want valid UUID (36 chars)", requestID)
	}
}

func TestRequestIDMiddlewareUsesClientProvided(t *testing.T) {
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "client-provided-id")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	if requestID != "client-provided-id" {
		t.Errorf("X-Request-ID = %q, want %q", requestID, "client-provided-id")
	}
}

func TestValidateQueryParams(t *testing.T) {
	cfg := DefaultValidationConfig()

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "valid query",
			query:   "http://localhost:8080/test?q=hello",
			wantErr: false,
		},
		{
			name:    "query too long",
			query:   "http://localhost:8080/test?q=" + strings.Repeat("a", cfg.MaxQueryLength+1),
			wantErr: true,
		},
		{
			name:    "path traversal in query",
			query:   "http://localhost:8080/test?path=../etc/passwd",
			wantErr: true,
		},
		{
			name:    "sql injection in query",
			query:   "http://localhost:8080/test?q=1%20=%201",
			wantErr: true,
		},
		{
			name:    "xss in query",
			query:   "http://localhost:8080/test?q=<script>alert(1)</script>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(ValidationMiddleware())
			r.GET("/test", func(c *gin.Context) {
				err := validateQueryParams(c, cfg)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", tt.query, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			gotErr := w.Code == http.StatusBadRequest
			if gotErr != tt.wantErr {
				t.Errorf("got error = %v, want error = %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(0, 5) // 0 tokens per second (no replenishment during test), capacity 5

	// First 5 should succeed
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th should be blocked
	if rl.Allow("192.168.1.1") {
		t.Error("6th request should be blocked")
	}

	rl.Stop()
}

func TestRateLimiterDifferentKeys(t *testing.T) {
	rl := NewRateLimiter(0, 2) // 2 requests max, no replenishment

	// 2 requests for key1 should succeed
	for i := 0; i < 2; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("key1 request %d should be allowed", i+1)
		}
	}

	// key1 is now blocked, but different key should succeed
	if !rl.Allow("192.168.1.2") {
		t.Error("different key should be allowed")
	}

	// key1 should still be blocked
	if rl.Allow("192.168.1.1") {
		t.Error("key1 should still be blocked")
	}

	rl.Stop()
}

func TestDefaultValidationConfig(t *testing.T) {
	cfg := DefaultValidationConfig()

	if cfg.MaxQueryLength <= 0 {
		t.Error("MaxQueryLength should be positive")
	}
	if cfg.MaxBodySize <= 0 {
		t.Error("MaxBodySize should be positive")
	}
	if len(cfg.AllowedFileExtensions) == 0 {
		t.Error("AllowedFileExtensions should not be empty")
	}
}
