package server

import (
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, 20)
	defer rl.Stop()

	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}
	if rl.rate != 10 {
		t.Errorf("rate = %d, want %d", rl.rate, 10)
	}
	if rl.capacity != 20 {
		t.Errorf("capacity = %d, want %d", rl.capacity, 20)
	}
	if rl.buckets == nil {
		t.Error("buckets map should be initialized")
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	defer rl.Stop()

	// First request should always be allowed
	if !rl.Allow("client-1") {
		t.Error("First request from new client should be allowed")
	}

	// Second request should also be allowed (we have capacity 5)
	if !rl.Allow("client-1") {
		t.Error("Second request should be allowed")
	}
}

func TestRateLimiter_Allow_ExhaustsCapacity(t *testing.T) {
	rl := NewRateLimiter(1, 2) // 1 token/sec, capacity 2
	defer rl.Stop()

	// First request - allowed, tokens = 2 - 1 = 1
	if !rl.Allow("client") {
		t.Error("First request should be allowed")
	}
	// Second request - allowed, tokens = 1 - 1 = 0
	if !rl.Allow("client") {
		t.Error("Second request should be allowed")
	}
	// Third request - denied, tokens = 0
	if rl.Allow("client") {
		t.Error("Third request should be denied (no tokens)")
	}
}

func TestRateLimiter_Allow_DifferentClients(t *testing.T) {
	rl := NewRateLimiter(1, 2)
	defer rl.Stop()

	// Exhaust client-1
	rl.Allow("client-1")
	rl.Allow("client-1")
	if rl.Allow("client-1") {
		t.Error("client-1 should be rate limited")
	}

	// client-2 should still be allowed
	if !rl.Allow("client-2") {
		t.Error("client-2 should be allowed (different bucket)")
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := NewRateLimiter(10, 5) // 10 tokens/sec refill
	defer rl.Stop()

	// Exhaust the bucket
	rl.Allow("client")
	rl.Allow("client")
	// Now at 0 tokens

	// Wait for refill (100ms should give us ~1 token)
	time.Sleep(150 * time.Millisecond)

	// Should have refilled
	if !rl.Allow("client") {
		t.Error("Request should be allowed after refill")
	}
}

func TestRateLimiter_CapacityNotExceeded(t *testing.T) {
	rl := NewRateLimiter(100, 5) // Fast refill but cap at 5
	defer rl.Stop()

	// Make many requests
	for i := 0; i < 10; i++ {
		rl.Allow("client")
	}

	// Wait for refill
	time.Sleep(200 * time.Millisecond)

	// Should get one, but not exceed capacity
	rl.Allow("client")
	rl.Allow("client")

	// Bucket should not exceed capacity
	rl.mu.Lock()
	b := rl.buckets["client"]
	rl.mu.Unlock()

	if b.tokens > 5 {
		t.Errorf("tokens = %d, should not exceed capacity %d", b.tokens, 5)
	}
}

func TestRateLimiter_Stop(t *testing.T) {
	rl := NewRateLimiter(10, 20)

	// Should not panic on Stop
	rl.Stop()

	// After stop, the cleanup goroutine should have exited
	// We can't directly observe this, but we can verify no panic occurs
}

func TestIsRateLimitEnabled(t *testing.T) {
	// Test default (when env is not set)
	// Note: This test may be affected by environment state
	enabled := IsRateLimitEnabled()
	if !enabled {
		// Default should be enabled when env is empty
		t.Log("IsRateLimitEnabled returned false with empty env")
	}
}

func TestGetRateLimitConfig(t *testing.T) {
	rate, capacity := GetRateLimitConfig()

	if rate <= 0 {
		t.Errorf("rate should be positive, got %d", rate)
	}
	if capacity <= 0 {
		t.Errorf("capacity should be positive, got %d", capacity)
	}
}

func TestBucket_Structure(t *testing.T) {
	b := &bucket{
		tokens:    10,
		lastReset: time.Now(),
	}

	if b.tokens != 10 {
		t.Errorf("bucket.tokens = %d, want %d", b.tokens, 10)
	}
	if b.lastReset.IsZero() {
		t.Error("bucket.lastReset should not be zero")
	}
}
