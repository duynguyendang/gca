package server

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     int           // tokens per second
	capacity int           // max tokens
	cleanup  time.Duration // cleanup interval for stale buckets
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate, capacity int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     rate,
		capacity: capacity,
		cleanup:  5 * time.Minute,
	}

	// Start cleanup goroutine
	go rl.cleanupStaleBuckets()

	return rl
}

// Allow checks if a request from the given key is allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]

	if !exists {
		rl.buckets[key] = &bucket{
			tokens:    rl.capacity - 1,
			lastReset: now,
		}
		return true
	}

	// Calculate tokens to add based on elapsed time
	elapsed := now.Sub(b.lastReset)
	tokensToAdd := int(elapsed.Seconds()) * rl.rate
	b.tokens += tokensToAdd
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastReset = now

	// Check if request is allowed
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// cleanupStaleBuckets removes buckets that haven't been used in a while
func (rl *RateLimiter) cleanupStaleBuckets() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, b := range rl.buckets {
			if now.Sub(b.lastReset) > rl.cleanup {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware returns a Gin middleware for rate limiting
func RateLimitMiddleware() gin.HandlerFunc {
	// Get rate limit configuration from environment variables
	rateStr := os.Getenv("RATE_LIMIT_REQUESTS_PER_SECOND")
	capacityStr := os.Getenv("RATE_LIMIT_BURST_CAPACITY")

	rate := 10     // default: 10 requests per second
	capacity := 20 // default: burst capacity of 20

	if rateStr != "" {
		if r, err := strconv.Atoi(rateStr); err == nil && r > 0 {
			rate = r
		}
	}

	if capacityStr != "" {
		if c, err := strconv.Atoi(capacityStr); err == nil && c > 0 {
			capacity = c
		}
	}

	limiter := NewRateLimiter(rate, capacity)

	return func(c *gin.Context) {
		// Use IP address as the rate limit key
		// In production, you might want to use API keys or user IDs
		key := c.ClientIP()

		// Check for API key in header (optional, for authenticated requests)
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			key = "api:" + apiKey
		}

		if !limiter.Allow(key) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Please try again later.",
				"retry_after": 1,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimitInfoMiddleware adds rate limit headers to responses
func RateLimitInfoMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Add rate limit headers
		c.Header("X-RateLimit-Limit", os.Getenv("RATE_LIMIT_REQUESTS_PER_SECOND"))
		c.Header("X-RateLimit-Remaining", "N/A")
		c.Header("X-RateLimit-Reset", "N/A")

		c.Next()
	}
}

// GetRateLimitConfig returns the current rate limit configuration
func GetRateLimitConfig() (rate int, capacity int) {
	rateStr := os.Getenv("RATE_LIMIT_REQUESTS_PER_SECOND")
	capacityStr := os.Getenv("RATE_LIMIT_BURST_CAPACITY")

	rate = 10
	capacity = 20

	if rateStr != "" {
		if r, err := strconv.Atoi(rateStr); err == nil && r > 0 {
			rate = r
		}
	}

	if capacityStr != "" {
		if c, err := strconv.Atoi(capacityStr); err == nil && c > 0 {
			capacity = c
		}
	}

	return rate, capacity
}

// IsRateLimitEnabled checks if rate limiting is enabled
func IsRateLimitEnabled() bool {
	enabled := os.Getenv("RATE_LIMIT_ENABLED")
	if enabled == "" {
		return true // enabled by default
	}
	return strings.ToLower(enabled) == "true"
}
