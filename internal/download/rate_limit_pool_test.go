package download

import (
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/engine"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

// TestWorkerPool_RateLimit_QueuedUpdateHonored ensures that a per-download
// rate limit set via SetDownloadRateLimit while the download is queued is
// carried through to the limiter when the worker starts.
func TestWorkerPool_RateLimit_QueuedUpdateHonored(t *testing.T) {
	ch := make(chan any, 10)
	pool := NewWorkerPool(ch, 1)

	id := "queued-rate-test"
	cfg := types.DownloadConfig{
		ID:            id,
		URL:           "http://example.com/file.bin",
		RateLimitBps:  0,
		RateLimitSet:  false,
	}

	pool.SetDefaultDownloadRateLimit(1000)
	pool.Add(cfg)

	pool.SetDownloadRateLimit(id, 5*1024*1024)

	// Verify queued config reflects the override
	pool.mu.RLock()
	qCfg := pool.queued[id]
	pool.mu.RUnlock()

	if !qCfg.RateLimitSet {
		t.Fatal("expected RateLimitSet=true after SetDownloadRateLimit")
	}
	if qCfg.RateLimitBps != 5*1024*1024 {
		t.Fatalf("queued RateLimitBps = %d, want %d", qCfg.RateLimitBps, 5*1024*1024)
	}

	pool.mu.Lock()
	delete(pool.queued, id)
	pool.mu.Unlock()
}

// TestWorkerPool_RateLimit_ExplicitUnlimitedSurvivesDefaultChange verifies
// that a download with RateLimitSet=true and RateLimitBps=0 (explicit
// unlimited) keeps rate=0 when the default is later raised.
func TestWorkerPool_RateLimit_ExplicitUnlimitedSurvivesDefaultChange(t *testing.T) {
	ch := make(chan any, 10)
	pool := NewWorkerPool(ch, 1)

	id := "explicit-unlimited"
	cfg := types.DownloadConfig{
		ID:            id,
		URL:           "http://example.com/file.bin",
		RateLimitBps:  0,
		RateLimitSet:  true,
	}

	pool.Add(cfg)

	// Verify ensureLimiterForConfig respects explicit unlimited
	testCfg := cfg
	pool.ensureLimiterForConfig(&testCfg)

	if testCfg.RateLimitBps != 0 {
		t.Fatalf("Explicit unlimited should stay at 0, got %d", testCfg.RateLimitBps)
	}

	// Now raise the default
	pool.SetDefaultDownloadRateLimit(5 * 1024 * 1024)

	pool.mu.RLock()
	qCfg, stillQueued := pool.queued[id]
	pool.mu.RUnlock()

	if stillQueued && qCfg.RateLimitBps != 0 {
		t.Errorf("Explicit unlimited was overridden by default change: got %d", qCfg.RateLimitBps)
	}

	pool.mu.Lock()
	delete(pool.queued, id)
	pool.mu.Unlock()
}

// TestWorkerPool_RateLimit_SetGlobalHonorsWaiter verifies that
// SetGlobalRateLimit wakes any goroutine blocked on the global limiter.
func TestWorkerPool_RateLimit_SetGlobalHonorsWaiter(t *testing.T) {
	ch := make(chan any, 10)
	pool := NewWorkerPool(ch, 1)

	// 1 byte/s so WaitN blocks on a 100-byte request
	pool.SetGlobalRateLimit(1)

	done := make(chan error, 1)
	go func() {
		done <- pool.globalLimiter.WaitN(nil, 100)
	}()

	select {
	case <-done:
		t.Fatal("global limiter waiter should be blocked")
	case <-time.After(100 * time.Millisecond):
		// expected
	}

	// Disabling should wake the waiter
	pool.SetGlobalRateLimit(0)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("global limiter waiter was not woken on disable")
	}
}

// TestWorkerPool_RateLimit_SetDownloadHonorsWaiter verifies that
// SetDownloadRateLimit wakes any waiter blocked on the per-download limiter.
func TestWorkerPool_RateLimit_SetDownloadHonorsWaiter(t *testing.T) {
	ch := make(chan any, 10)
	pool := NewWorkerPool(ch, 1)

	id := "dl-waiter-test"
	cfg := types.DownloadConfig{
		ID:            id,
		URL:           "http://example.com/file.bin",
		RateLimitBps:  10000,
		RateLimitSet:  true,
	}
	pool.Add(cfg)

	pool.mu.RLock()
	qCfg := pool.queued[id]
	pool.mu.RUnlock()
	pool.ensureLimiterForConfig(&qCfg)

	done := make(chan error, 1)
	go func() {
		done <- qCfg.Limiter.WaitN(nil, 20000)
	}()

	select {
	case <-done:
		t.Fatal("per-download limiter waiter should be blocked")
	case <-time.After(100 * time.Millisecond):
		// expected
	}

	// Increasing the rate should wake the waiter
	pool.SetDownloadRateLimit(id, 10*1024*1024)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("per-download limiter waiter was not woken on rate increase")
	}

	pool.mu.Lock()
	delete(pool.queued, id)
	pool.mu.Unlock()
}

// TestWorkerPool_RateLimit_MultiLimiterComposition verifies that the
// MultiLimiter blocks until all component limiters are satisfied.
func TestWorkerPool_RateLimit_MultiLimiterComposition(t *testing.T) {
	global := engine.NewRateLimiter(10000, 10000)
	perDl := engine.NewRateLimiter(10000, 10000)
	ml := engine.NewMultiLimiter(global, perDl)

	// Both limiters have 10000 tokens; requesting 20000 should block
	done := make(chan error, 1)
	go func() {
		done <- ml.WaitN(nil, 20000)
	}()

	select {
	case <-done:
		t.Fatal("multi limiter waiter should be blocked")
	case <-time.After(100 * time.Millisecond):
		// expected
	}

	// Satisfy the global limiter but not per-download
	global.SetRate(20000, 20000)

	select {
	case <-done:
		t.Fatal("multi limiter should still be blocked (per-dl not satisfied)")
	case <-time.After(100 * time.Millisecond):
		// expected
	}

	// Now satisfy both
	perDl.SetRate(20000, 20000)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("multi limiter waiter was not woken when all limiters satisfied")
	}
}
