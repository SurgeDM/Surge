package engine

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_SetRateDisableWakesWaiter(t *testing.T) {
	limiter := NewRateLimiter(1, 0)
	done := make(chan error, 1)

	go func() {
		done <- limiter.WaitN(context.Background(), 10)
	}()

	select {
	case err := <-done:
		t.Fatalf("WaitN returned before rate change: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	limiter.SetRate(0, 0)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitN returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitN did not wake after disabling rate limit")
	}
}

func TestRateLimiter_SetRateIncreaseWakesWaiter(t *testing.T) {
	limiter := NewRateLimiter(1, 0)
	done := make(chan error, 1)

	go func() {
		done <- limiter.WaitN(context.Background(), 10)
	}()

	select {
	case err := <-done:
		t.Fatalf("WaitN returned before rate change: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	limiter.SetRate(10*1024*1024, 10*1024*1024)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitN returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitN did not wake after increasing rate limit")
	}
}
