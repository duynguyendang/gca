package meb

import (
	"testing"
)

func TestBuilder_SimilarTo(t *testing.T) {
	// Create a dummy store (nil is fine since we are only testing builder configuration)
	b := NewBuilder(nil)

	vec := []float32{1.0, 2.0, 3.0}

	// Test 1: SimilarTo without threshold
	b.SimilarTo(vec)
	if len(b.vectorQuery) != 3 {
		t.Errorf("expected vector query length 3, got %d", len(b.vectorQuery))
	}
	if b.threshold != 0 {
		t.Errorf("expected threshold 0, got %f", b.threshold)
	}

	// Test 2: SimilarTo with threshold
	b.SimilarTo(vec, 0.85)
	if len(b.vectorQuery) != 3 {
		t.Errorf("expected vector query length 3, got %d", len(b.vectorQuery))
	}
	if b.threshold != 0.85 {
		t.Errorf("expected threshold 0.85, got %f", b.threshold)
	}

	// Test 3: SimilarTo with multiple thresholds (should use first)
	b.SimilarTo(vec, 0.5, 0.9)
	if b.threshold != 0.5 {
		t.Errorf("expected threshold 0.5 (first arg), got %f", b.threshold)
	}
}
