package throttle

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
	l := &Limiter{}
	l.SetRate(rate)
	l.tokens = l.burst // Start full
	l.lastRefill = time.Now()
	return l
}

// SetRate dynamically updates the rate. If rate <= 0, the limiter is effectively disabled.
func (l *Limiter) SetRate(rate int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rate = rate
	if rate <= 0 {
		l.burst = 0
		l.tokens = 0
		return
	}

	// Burst capacity is 1MB or 2x rate, whichever is smaller.
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
	if l == nil {
		return nil
	}

	remaining := int64(n)
	for remaining > 0 {
		charge := remaining
		if charge > l.burst {
			charge = l.burst
		}

		for {
			l.mu.Lock()
			if l.rate <= 0 {
				l.mu.Unlock()
				return nil
			}

			now := time.Now()
			elapsed := now.Sub(l.lastRefill)
			refill := int64(float64(elapsed.Nanoseconds()) * float64(l.rate) / float64(time.Second.Nanoseconds()))
			if refill > 0 {
				l.tokens += refill
				if l.tokens > l.burst {
					l.tokens = l.burst
				}
				l.lastRefill = l.lastRefill.Add(time.Duration(float64(refill) * float64(time.Second.Nanoseconds()) / float64(l.rate)))
			}

			if l.tokens >= charge {
				l.tokens -= charge
				l.mu.Unlock()
				break
			}

			needed := charge - l.tokens
			waitDuration := time.Duration(float64(needed) * float64(time.Second.Nanoseconds()) / float64(l.rate))
			l.mu.Unlock()

			if waitDuration < time.Millisecond {
				waitDuration = time.Millisecond
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDuration):
			}
		}

		remaining -= charge
	}
	return nil
}

// ThrottledReader wraps an io.Reader and enforces rate limits.
type ThrottledReader struct {
	r        io.Reader
	limiters []*Limiter
	ctx      context.Context
}

// NewThrottledReader creates a new throttled reader. It returns a passthrough reader
// if no active limiters are provided.
func NewThrottledReader(ctx context.Context, r io.Reader, limiters ...*Limiter) io.Reader {
	var active []*Limiter
	for _, l := range limiters {
		if l != nil {
			active = append(active, l)
		}
	}

	if len(active) == 0 {
		return r
	}

	return &ThrottledReader{
		r:        r,
		limiters: active,
		ctx:      ctx,
	}
}

func (t *ThrottledReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		for _, l := range t.limiters {
			if waitErr := l.waitN(t.ctx, n); waitErr != nil {
				return n, waitErr
			}
		}
	}
	return n, err
}
