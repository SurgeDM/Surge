package engine

import (
	"context"
	"io"
	"sync"
	"time"
)

// Limiter implements a token bucket rate limiter.
type Limiter struct {
	rate       int64 // tokens per second
	burst      int64 // maximum tokens in bucket
	tokens     int64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewLimiter creates a new rate limiter. rate is in bytes per second.
// If rate is 0, it acts as unlimited.
func NewLimiter(rate int64) *Limiter {
	if rate <= 0 {
		return nil
	}
	// Burst capacity is 1MB or 2x rate, whichever is smaller, but at least 32KB
	burst := rate * 2
	if burst > 1024*1024 {
		burst = 1024 * 1024
	}
	if burst < 1 {
		burst = 1
	}

	return &Limiter{
		rate:       rate,
		burst:      burst,
		tokens:     burst,
		lastRefill: time.Now(),
	}
}

// SetRate dynamically updates the rate.
func (l *Limiter) SetRate(rate int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rate = rate
	// Update burst as well
	burst := rate * 2
	if burst > 1024*1024 {
		burst = 1024 * 1024
	}
	if burst < 1 {
		burst = 1
	}
	l.burst = burst
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
}

// waitN blocks until n tokens are available or context is cancelled.
func (l *Limiter) waitN(ctx context.Context, n int) error {
	if l == nil || l.rate <= 0 {
		return nil
	}

	for {
		l.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(l.lastRefill)

		// Refill tokens: (elapsed * rate) / second
		// Using nanoseconds to avoid early truncation
		refill := int64(float64(elapsed.Nanoseconds()) * float64(l.rate) / float64(time.Second.Nanoseconds()))

		if refill > 0 {
			l.tokens += refill
			if l.tokens > l.burst {
				l.tokens = l.burst
			}
			// Important: don't just set lastRefill to now, only advance by the amount we refilled
			// to avoid losing sub-token time.
			l.lastRefill = l.lastRefill.Add(time.Duration(float64(refill) * float64(time.Second.Nanoseconds()) / float64(l.rate)))
		}

		if l.tokens >= int64(n) {
			l.tokens -= int64(n)
			l.mu.Unlock()
			return nil
		}

		// Calculate wait time for the remaining tokens
		needed := int64(n) - l.tokens
		waitDuration := time.Duration(float64(needed) * float64(time.Second.Nanoseconds()) / float64(l.rate))
		l.mu.Unlock()

		// Sleep at least a tiny bit to avoid busy loops if waitDuration is very small
		if waitDuration < 1*time.Millisecond {
			waitDuration = 1 * time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
		}
	}
}

// ThrottledReader wraps an io.Reader and enforces rate limits.
type ThrottledReader struct {
	r        io.Reader
	limiters []*Limiter
	ctx      context.Context
}

// NewThrottledReader creates a new throttled reader.
func NewThrottledReader(ctx context.Context, r io.Reader, limiters ...*Limiter) *ThrottledReader {
	// Filter out nil limiters
	var active []*Limiter
	for _, l := range limiters {
		if l != nil {
			active = append(active, l)
		}
	}

	if len(active) == 0 {
		return nil // Should handle this in caller or return a passthrough
	}

	return &ThrottledReader{
		r:        r,
		limiters: active,
		ctx:      ctx,
	}
}

func (t *ThrottledReader) Read(p []byte) (int, error) {
	if t == nil {
		return 0, io.ErrUnexpectedEOF
	}

	n, err := t.r.Read(p)
	if n > 0 {
		// Wait for tokens from all limiters
		for _, l := range t.limiters {
			if waitErr := l.waitN(t.ctx, n); waitErr != nil {
				return n, waitErr
			}
		}
	}
	return n, err
}
