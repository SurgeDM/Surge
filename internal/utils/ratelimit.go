package utils

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// TokenBucket is a thread-safe rate limiter.
type TokenBucket struct {
	mu      sync.RWMutex
	limiter *rate.Limiter
	enabled bool
}

// GlobalRateLimiter is the shared singleton for global application bandwidth limit.
var GlobalRateLimiter = NewTokenBucket(0)

// NewTokenBucket creates a new rate limiter allowing 'rateBytes' bytes per second.
// If rateBytes is 0, the limiter is disabled and WaitN returns immediately.
func NewTokenBucket(rateBytes int64) *TokenBucket {
	// A max burst size is usually 1 second's worth of tokens or more.
	// Since we chunk requests in WaitN, burst can strictly follow rateBytes.
	burst := int(rateBytes)
	if burst < 1 && rateBytes > 0 {
		burst = 1
	}

	enabled := rateBytes > 0
	var l *rate.Limiter
	if enabled {
		l = rate.NewLimiter(rate.Limit(rateBytes), burst)
	}

	return &TokenBucket{
		limiter: l,
		enabled: enabled,
	}
}

// SetRate dynamically updates the rate limit. 0 disables it.
func (tb *TokenBucket) SetRate(rateBytes int64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if rateBytes <= 0 {
		tb.enabled = false
		tb.limiter = nil
		return
	}

	burst := int(rateBytes)
	if burst < 1 {
		burst = 1
	}

	if tb.limiter == nil {
		tb.limiter = rate.NewLimiter(rate.Limit(rateBytes), burst)
	} else {
		tb.limiter.SetLimit(rate.Limit(rateBytes))
		tb.limiter.SetBurst(burst)
	}
	tb.enabled = true
}

// WaitN blocks until 'n' bytes are available according to the rate limit.
func (tb *TokenBucket) WaitN(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}

	tb.mu.RLock()
	enabled := tb.enabled
	l := tb.limiter
	tb.mu.RUnlock()

	if !enabled || l == nil {
		return nil
	}

	// Wait handles context cancellation naturally
	// Loop over burst chunks to avoid WaitN error if n > burst
	burst := l.Burst()
	if burst <= 0 {
		return nil
	}

	for n > 0 {
		req := n
		if req > burst {
			req = burst
		}
		if err := l.WaitN(ctx, req); err != nil {
			// rate.Limiter may return a non-wrapped error if the deadline is too soon.
			// Map it to standard context errors for compatibility.
			if err.Error() != "" && (err == context.DeadlineExceeded || err == context.Canceled) {
				return err
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			// If rate limiter rejected it because it would exceed the deadline:
			return context.DeadlineExceeded
		}
		n -= req
	}
	return nil
}
