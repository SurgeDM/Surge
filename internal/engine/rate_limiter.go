package engine

import (
	"context"
	"math/bits"
	"sync"
	"time"
)

const maxInt64 = int64(^uint64(0) >> 1)

type RateLimiter struct {
	rate       int64
	tokens     int64
	bucketSize int64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewRateLimiter(rate int64, bucketSize int64) *RateLimiter {
	if bucketSize < 0 {
		bucketSize = 0
	}
	return &RateLimiter{
		rate:       rate,
		bucketSize: bucketSize,
		tokens:     bucketSize,
		lastRefill: time.Now(),
	}
}

func (r *RateLimiter) WaitN(ctx context.Context, n int64) error {
	if n <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		r.mu.Lock()

		if r.rate <= 0 {
			r.mu.Unlock()
			return nil
		}

		bucketCap := r.bucketSize
		if bucketCap < n {
			bucketCap = n
		}

		now := time.Now()

		if r.lastRefill.IsZero() {
			r.lastRefill = now
		} else {
			elapsed := now.Sub(r.lastRefill)
			if elapsed > 0 {
				hi, lo := bits.Mul64(uint64(elapsed.Nanoseconds()), uint64(r.rate))
				add, _ := bits.Div64(hi, lo, uint64(time.Second))
				if add > 0 {
					if add > uint64(maxInt64) {
						r.tokens = maxInt64
					} else {
						r.tokens += int64(add)
					}
					if r.tokens > bucketCap {
						r.tokens = bucketCap
					}
					r.lastRefill = now
				}
			}
		}

		if r.tokens > bucketCap {
			r.tokens = bucketCap
		}

		if r.tokens >= n {
			r.tokens -= n
			r.mu.Unlock()
			return nil
		}

		missing := n - r.tokens

		hi, lo := bits.Mul64(uint64(missing), uint64(time.Second))
		waitNs, rem := bits.Div64(hi, lo, uint64(r.rate))
		if rem > 0 {
			waitNs++
		}

		r.mu.Unlock()

		if waitNs == 0 {
			continue
		}
		if waitNs > uint64(maxInt64) {
			waitNs = uint64(maxInt64)
		}

		timer := time.NewTimer(time.Duration(int64(waitNs)))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *RateLimiter) SetRate(rate int64, bucketSize int64) {
	if bucketSize < 0 {
		bucketSize = 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// settle refill using the old rate before changing config
	if !r.lastRefill.IsZero() && r.rate > 0 {
		elapsed := now.Sub(r.lastRefill)
		if elapsed > 0 {
			hi, lo := bits.Mul64(uint64(elapsed.Nanoseconds()), uint64(r.rate))
			add, _ := bits.Div64(hi, lo, uint64(time.Second))
			if add > 0 {
				if add > uint64(maxInt64) {
					r.tokens = maxInt64
				} else {
					r.tokens += int64(add)
				}
			}
		}
	}

	r.rate = rate
	r.bucketSize = bucketSize

	if r.tokens > bucketSize {
		r.tokens = bucketSize
	}
	r.lastRefill = now
}
