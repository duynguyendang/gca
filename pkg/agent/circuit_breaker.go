package agent

import (
	"context"
	"fmt"
	"time"
)

const DefaultQueryTimeout = 2 * time.Second

// WithQueryTimeout wraps a context with the standard Datalog query timeout.
func WithQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, DefaultQueryTimeout)
}

// CircuitBreaker protects against cascading failures from slow queries.
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	failures     int
	lastFailure  time.Time
	state        string // "closed", "open", "half-open"
}

// NewCircuitBreaker creates a breaker that opens after maxFailures consecutive errors.
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        "closed",
	}
}

// Allow returns nil if the circuit is closed (healthy), or an error if open.
func (cb *CircuitBreaker) Allow() error {
	if cb.state == "open" {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = "half-open"
			return nil
		}
		return fmt.Errorf("circuit breaker open: too many failures, retry after %v", cb.resetTimeout-time.Since(cb.lastFailure))
	}
	return nil
}

// RecordSuccess resets the failure counter.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.failures = 0
	cb.state = "closed"
}

// RecordFailure increments failures and may open the circuit.
func (cb *CircuitBreaker) RecordFailure() {
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.maxFailures {
		cb.state = "open"
	}
}
