package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenBucket_Disabled(t *testing.T) {
	// A rate of 0 disables the limiter
	tb := NewTokenBucket(0)

	start := time.Now()
	// Ask for a large amount of tokens
	err := tb.WaitN(context.Background(), 1024*1024*100) // 100MB
	elapsed := time.Since(start)

	assert.NoError(t, err)
	// Shouldn't have blocked for any meaningful amount of time
	assert.Less(t, elapsed, 10*time.Millisecond, "Disabled bucket should not block")
}

func TestTokenBucket_Throttles(t *testing.T) {
	// 500 bytes per second
	tb := NewTokenBucket(500)

	// Consume all initial tokens immediately (500 tokens usually) Wait, the burst size is the rate.
	// We wait 500 tokens. This shouldn't block much because we start full.
	start := time.Now()
	err := tb.WaitN(context.Background(), 500)
	assert.NoError(t, err)
	elapsedInitial := time.Since(start)
	assert.Less(t, elapsedInitial, 50*time.Millisecond, "Initial capacity should service immediately")

	// Now ask for 250 more tokens. At 500 bytes/sec, this should take ~0.5 seconds.
	start = time.Now()
	err = tb.WaitN(context.Background(), 250)
	assert.NoError(t, err)
	elapsedNext := time.Since(start)

	assert.GreaterOrEqual(t, float64(elapsedNext), float64(400*time.Millisecond), "Should block waiting for tokens")
	assert.Less(t, float64(elapsedNext), float64(700*time.Millisecond), "Should wake up reasonably close to target time")
}

func TestTokenBucket_ContextCancellation(t *testing.T) {
	// 100 bytes per second
	tb := NewTokenBucket(100)

	// Consume initial bucket
	_ = tb.WaitN(context.Background(), 100)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Ask for 500 tokens, which would take 5 seconds.
	// But context will cancel in 100ms.
	start := time.Now()
	err := tb.WaitN(ctx, 500)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, float64(elapsed), float64(200*time.Millisecond), "Should abort early due to context cancellation")
}

func TestTokenBucket_DynamicSetRate(t *testing.T) {
	tb := NewTokenBucket(500)

	// Consume initial
	_ = tb.WaitN(context.Background(), 500)

	// Change rate to 5000 bytes/sec
	tb.SetRate(5000)

	// Ask for 2500 bytes. With new rate this takes 0.5s.
	start := time.Now()
	err := tb.WaitN(context.Background(), 2500)
	assert.NoError(t, err)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, float64(elapsed), float64(400*time.Millisecond))
	assert.Less(t, float64(elapsed), float64(700*time.Millisecond))
}
