package gmail

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/api/googleapi"
)

// RateLimitConfig configures rate limiting behavior
type RateLimitConfig struct {
	RequestsPerSecond float64 // Target rate (default: 100)
	BurstCapacity     int64   // Max accumulated tokens (default: 1000)
	MaxRetries        int     // Retry attempts on quota errors (default: 5)
	InitialBackoffMs  int     // Starting backoff duration (default: 100)
	MaxBackoffMs      int     // Maximum backoff duration (default: 5000)
}

// DefaultRateLimitConfig returns conservative default configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerSecond: 100,  // 40% of Gmail's 250 req/sec quota
		BurstCapacity:     1000, // Allow bursts up to 1000 requests
		MaxRetries:        5,
		InitialBackoffMs:  100,
		MaxBackoffMs:      5000,
	}
}

// RateLimiter implements token bucket rate limiting with exponential backoff
type RateLimiter struct {
	config *RateLimitConfig

	tokens         float64
	lastRefillTime time.Time

	mu sync.Mutex
}

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(cfg *RateLimitConfig) *RateLimiter {
	if cfg == nil {
		cfg = DefaultRateLimitConfig()
	}

	// Validate config
	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = 100
	}
	if cfg.BurstCapacity <= 0 {
		cfg.BurstCapacity = 1000
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 5
	}
	if cfg.InitialBackoffMs <= 0 {
		cfg.InitialBackoffMs = 100
	}
	if cfg.MaxBackoffMs <= 0 {
		cfg.MaxBackoffMs = 5000
	}

	return &RateLimiter{
		config:         cfg,
		tokens:         float64(cfg.BurstCapacity), // Start with full capacity
		lastRefillTime: time.Now(),
	}
}

// Allow blocks until a token is available or context is canceled
func (r *RateLimiter) Allow(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(r.lastRefillTime).Seconds()
	r.tokens += elapsed * r.config.RequestsPerSecond

	// Cap at burst capacity
	if r.tokens > float64(r.config.BurstCapacity) {
		r.tokens = float64(r.config.BurstCapacity)
	}

	r.lastRefillTime = now

	// If we have a token, consume it immediately
	if r.tokens >= 1.0 {
		r.tokens -= 1.0
		return nil
	}

	// Need to wait for token
	tokensNeeded := 1.0 - r.tokens
	waitDuration := time.Duration(tokensNeeded/r.config.RequestsPerSecond*float64(time.Second)) + time.Millisecond

	// Release lock while waiting
	r.mu.Unlock()

	select {
	case <-time.After(waitDuration):
		// Re-acquire lock and try again
		r.mu.Lock()
		return r.Allow(ctx)
	case <-ctx.Done():
		r.mu.Lock() // Re-acquire lock before returning
		return ctx.Err()
	}
}

// HandleQuotaError performs exponential backoff for quota errors
// Returns true if the request should be retried, false otherwise
func (r *RateLimiter) HandleQuotaError(ctx context.Context, err error, attempt int) (bool, error) {
	// Check if this is a quota error (429)
	if apiErr, ok := err.(*googleapi.Error); ok {
		if apiErr.Code == 429 {
			if attempt >= r.config.MaxRetries {
				return false, fmt.Errorf("max retries exceeded for quota error: %w", err)
			}

			// Calculate backoff with exponential growth
			backoffMs := r.config.InitialBackoffMs * (1 << attempt) // 100, 200, 400, 800, 1600
			if backoffMs > r.config.MaxBackoffMs {
				backoffMs = r.config.MaxBackoffMs
			}

			// Add jitter (0-50ms)
			jitterMs := rand.Intn(51)
			totalBackoff := time.Duration(backoffMs+jitterMs) * time.Millisecond

			select {
			case <-time.After(totalBackoff):
				return true, nil // Retry
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
	}

	// Not a quota error or not retryable
	return false, err
}

// RecordSuccess can be called to track successful requests (currently no-op)
func (r *RateLimiter) RecordSuccess() {
	// Could be used for adaptive rate limiting in the future
}
