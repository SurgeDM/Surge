package transport

import (
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestParseRetryAfter_Seconds(t *testing.T) {
	now := time.Now()
	d, ok := ParseRetryAfter("120", now)
	if !ok {
		t.Fatal("expected ok=true for seconds form")
	}
	if d != 120*time.Second {
		t.Fatalf("expected 120s, got %v", d)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Now()
	future := now.Add(5*time.Second).UTC().Format("Mon, 02 Jan 2006 15:04:05") + " GMT"
	d, ok := ParseRetryAfter(future, now)
	if !ok {
		t.Fatalf("expected ok=true for HTTP-date form: %q", future)
	}
	if d < 4*time.Second || d > 6*time.Second {
		t.Fatalf("expected ~5s, got %v", d)
	}
}

func TestParseRetryAfter_Empty(t *testing.T) {
	_, ok := ParseRetryAfter("", time.Now())
	if ok {
		t.Fatal("expected ok=false for empty header")
	}
}

func TestParseRetryAfter_Garbage(t *testing.T) {
	_, ok := ParseRetryAfter("not-valid", time.Now())
	if ok {
		t.Fatal("expected ok=false for garbage")
	}
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	now := time.Now()
	past := now.Add(-10*time.Second).UTC().Format("Mon, 02 Jan 2006 15:04:05") + " GMT"
	d, ok := ParseRetryAfter(past, now)
	if !ok {
		t.Fatalf("expected ok=true for past HTTP-date: %q", past)
	}
	if d >= 0 {
		t.Fatalf("expected negative duration for past date, got %v", d)
	}
}

func TestHostRateLimiter_PenalizeExpBackoff(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	penalize := func(host string) time.Duration {
		deadline := h.Penalize(host, 0, false, now)
		return deadline.Sub(now)
	}

	d1 := penalize("a.example.com")
	d2 := penalize("a.example.com")
	d3 := penalize("a.example.com")

	if d2 < d1 {
		t.Fatalf("expected backoff to grow: d1=%v d2=%v", d1, d2)
	}
	if d3 < d2 {
		t.Fatalf("expected backoff to keep growing: d2=%v d3=%v", d2, d3)
	}
}

func TestHostRateLimiter_PenalizeRetryAfterClamp(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	deadline := h.Penalize("example.com", 3600*time.Second, true, now)
	backoff := deadline.Sub(now)

	if backoff > types.RateLimitMaxBackoff+time.Second {
		t.Fatalf("expected backoff clamped to max %v, got %v", types.RateLimitMaxBackoff, backoff)
	}
}

func TestHostRateLimiter_PenalizeMinClamp(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	deadline := h.Penalize("example.com", 0, true, now)
	backoff := deadline.Sub(now)

	if backoff < types.RateLimitMinBackoff {
		t.Fatalf("expected backoff at least %v, got %v", types.RateLimitMinBackoff, backoff)
	}
}

func TestHostRateLimiter_BlockedUntil(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	if bu := h.BlockedUntil("unknown.example.com", now); !bu.IsZero() {
		t.Fatal("expected zero time for unknown host")
	}

	h.Penalize("example.com", 5*time.Second, true, now)
	bu := h.BlockedUntil("example.com", now)
	if bu.IsZero() {
		t.Fatal("expected non-zero blocked until")
	}
	if !now.Before(bu) {
		t.Fatalf("blocked until %v should be after now %v", bu, now)
	}

	free := h.BlockedUntil("example.com", now.Add(6*time.Second))
	if !free.IsZero() {
		t.Fatal("expected free after penalty expires")
	}
}

func TestHostRateLimiter_RecordSuccess(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	h.Penalize("example.com", 1*time.Second, true, now)
	h.recordSuccess("example.com", now.Add(100*time.Millisecond))

	bu := h.BlockedUntil("example.com", now.Add(100*time.Millisecond))
	if bu.IsZero() {
		t.Fatal("expected overlapping success to preserve the future cooldown")
	}

	h.recordSuccess("example.com", now.Add(2*time.Second))
	h.mu.Lock()
	penalty, retained := h.hosts["example.com"]
	h.mu.Unlock()
	if !retained {
		t.Fatal("expected success after cooldown to retain the learned host state")
	}
	if !penalty.until.IsZero() || penalty.consecutive != 0 || !penalty.lastHit.IsZero() {
		t.Fatalf("expected cooldown state to be cleared, got %+v", penalty)
	}
	if penalty.concurrencyCap != UnknownHostInitialCap {
		t.Fatalf("expected host concurrency cap to be retained, got %d", penalty.concurrencyCap)
	}
}

func TestHostRateLimiter_LaterPenaltyCannotShortenCooldown(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	first := h.Penalize("example.com", 10*time.Second, true, now)
	second := h.Penalize("example.com", time.Second, true, now.Add(time.Second))
	if second.Before(first) {
		t.Fatalf("later penalty shortened deadline from %v to %v", first, second)
	}

	h.recordSuccess("example.com", now.Add(2*time.Second))
	if got := h.BlockedUntil("example.com", now.Add(2*time.Second)); got.Before(first) {
		t.Fatalf("overlapping success shortened deadline from %v to %v", first, got)
	}
}

func TestHostRateLimiter_PickMirror_FreeChosen(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	hosts := []string{"a.example.com", "b.example.com"}
	h.Penalize("b.example.com", 10*time.Second, true, now)

	idx, wait := h.PickMirror(hosts, 1, now)
	if idx != 0 {
		t.Fatalf("expected free mirror a (idx 0), got %d", idx)
	}
	if wait != 0 {
		t.Fatalf("expected no wait, got %v", wait)
	}
}

func TestHostRateLimiter_PickMirror_AllPenalized(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	hosts := []string{"a.example.com", "b.example.com"}
	h.Penalize("a.example.com", 10*time.Second, true, now)
	h.Penalize("b.example.com", 5*time.Second, true, now)

	idx, wait := h.PickMirror(hosts, 0, now)
	if wait <= 0 {
		t.Fatal("expected positive wait when all penalized")
	}
	if idx != 1 {
		t.Fatalf("expected soonest mirror b (idx 1), got %d", idx)
	}
}

func TestHostRateLimiter_PickMirror_StartIdxRotation(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	hosts := []string{"a.example.com", "b.example.com", "c.example.com"}

	idx, wait := h.PickMirror(hosts, 1, now)
	if idx != 1 {
		t.Fatalf("expected to start at index 1, got %d", idx)
	}
	if wait != 0 {
		t.Fatalf("expected no wait, got %v", wait)
	}
}

func TestHostRateLimiter_PenaltyDecay(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	h.Penalize("example.com", 1*time.Second, true, now)

	h.Penalize("example.com", 1*time.Second, true, now.Add(types.RateLimitPenaltyDecay+time.Second))

	bu := h.BlockedUntil("example.com", now.Add(types.RateLimitPenaltyDecay+time.Second))
	if bu.IsZero() {
		t.Fatal("expected host to still be penalized after decay")
	}
}

func TestMirrorHost(t *testing.T) {
	h := MirrorHost("https://cdn.example.com:443/path/file.bin")
	if h != "cdn.example.com:443" {
		t.Fatalf("expected cdn.example.com:443, got %s", h)
	}
}

func TestMirrorHost_ParseError(t *testing.T) {
	raw := "://invalid"
	h := MirrorHost(raw)
	if h != raw {
		t.Fatalf("expected fallback to raw URL on parse error, got %s", h)
	}
}

func TestHostRateLimiter_PenalizeNegativeRetryAfter(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	deadline := h.Penalize("example.com", -10*time.Second, true, now)
	backoff := deadline.Sub(now)

	if backoff < types.RateLimitMinBackoff {
		t.Fatalf("expected backoff at least %v for negative Retry-After, got %v", types.RateLimitMinBackoff, backoff)
	}
	if backoff > types.RateLimitMinBackoff+time.Second {
		t.Fatalf("expected backoff near min %v for negative Retry-After, got %v", types.RateLimitMinBackoff, backoff)
	}
}

func TestHostRateLimiter_CleanupRemovesExpired(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	h.Penalize("old.example.com", 1*time.Second, true, now)

	h.Penalize("new.example.com", 10*time.Second, true, now.Add(types.RateLimitPenaltyDecay+2*time.Second))

	bu := h.BlockedUntil("old.example.com", now.Add(types.RateLimitPenaltyDecay+3*time.Second))
	if !bu.IsZero() {
		t.Fatal("expected old host to be cleaned up after decay window + expiry")
	}

	bu2 := h.BlockedUntil("new.example.com", now.Add(types.RateLimitPenaltyDecay+3*time.Second))
	if bu2.IsZero() {
		t.Fatal("expected new host to still exist")
	}
}

func TestHostRateLimiter_PenaltyDecayResetsConsecutive(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	penalizeAt := func(t time.Time) time.Duration {
		deadline := h.Penalize("example.com", 0, false, t)
		return deadline.Sub(t)
	}

	d1 := penalizeAt(now)
	d2 := penalizeAt(now)

	d3 := penalizeAt(now.Add(types.RateLimitPenaltyDecay + time.Second))

	if d3 >= d2 {
		t.Fatalf("expected decay-reset backoff (d3=%v) to be less than exponential (d2=%v)", d3, d2)
	}
	_ = d1
}

func TestHostRateLimiter_ConcurrencyCapLookup(t *testing.T) {
	h := NewHostRateLimiter()

	// Unknown host: should default to UnknownHostInitialCap (4)
	if cap := h.ConcurrencyCap("unknown.com", 8); cap != 4 {
		t.Fatalf("expected unknown host cap=4, got %d", cap)
	}

	// Configured max less than 4
	if cap := h.ConcurrencyCap("unknown.com", 2); cap != 2 {
		t.Fatalf("expected unknown host cap=2 when configuredMax=2, got %d", cap)
	}
}

func TestThrottleBurstCoalesced(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	// Initial cap is 4. Ramp up to 8 via progress reports
	for i := 0; i < 4; i++ {
		h.ReportProgressBytes("host.com", 512*1024)
		h.ReportCompletedRange("host.com", 8, now.Add(time.Duration(i)*20*time.Second))
		h.ReportCompletedRange("host.com", 8, now.Add(time.Duration(i)*20*time.Second))
	}
	if cap := h.ConcurrencyCap("host.com", 8); cap != 8 {
		t.Fatalf("expected cap=8 after progress, got %d", cap)
	}

	// 4 workers report throttle in the same burst (same timestamp/active cooldown)
	_, cap1 := h.ReportThrottle("host.com", 8, 5*time.Second, true, now.Add(100*time.Second))
	_, cap2 := h.ReportThrottle("host.com", 8, 5*time.Second, true, now.Add(100*time.Second+10*time.Millisecond))
	_, cap3 := h.ReportThrottle("host.com", 8, 5*time.Second, true, now.Add(100*time.Second+20*time.Millisecond))
	until4, cap4 := h.ReportThrottle("host.com", 8, 5*time.Second, true, now.Add(100*time.Second+30*time.Millisecond))

	if cap1 != 4 || cap2 != 4 || cap3 != 4 || cap4 != 4 {
		t.Fatalf("expected burst throttles to coalesce to cap=4, got cap1=%d cap2=%d cap3=%d cap4=%d", cap1, cap2, cap3, cap4)
	}

	// After cooldown expires (new episode), another throttle halves cap from 4 to 2
	_, capNext := h.ReportThrottle("host.com", 4, 5*time.Second, true, until4.Add(time.Second))
	if capNext != 2 {
		t.Fatalf("expected next episode throttle cap=2, got %d", capNext)
	}
}

func TestHostRateLimiter_ReportProgressRecovery(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	// Throttle down to cap 2
	h.ReportThrottle("recover.com", 4, 2*time.Second, true, now)

	// Partial progress (256 KB, 1 range) -> should not recover cap
	h.ReportProgressBytes("recover.com", 256*1024)
	cap, ok := h.ReportCompletedRange("recover.com", 8, now.Add(RecoveryWindow+time.Second))
	if ok || cap != 2 {
		t.Fatalf("expected cap=2 without full recovery threshold, got cap=%d ok=%v", cap, ok)
	}

	// Second range completing 256 KB (total 512 KB, 2 ranges) -> should recover to 3
	h.ReportProgressBytes("recover.com", 256*1024)
	cap, ok = h.ReportCompletedRange("recover.com", 8, now.Add(RecoveryWindow+time.Second))
	if !ok || cap != 3 {
		t.Fatalf("expected recovery to cap=3, got cap=%d ok=%v", cap, ok)
	}
}

func TestHostRateLimiter_SubEpisodeThrottleUpdatesLastThrottle(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	// Initial throttle at t=0
	h.ReportThrottle("subepisode.com", 4, 10*time.Second, true, now)

	// Sub-episode throttle at t=4s (same episode cooldown until t=10s)
	h.ReportThrottle("subepisode.com", 4, 2*time.Second, true, now.Add(4*time.Second))

	// At t=16s (16s since t=0, but only 12s since t=4s throttle)
	// RecoveryWindow is 15s. Recovery should NOT happen yet because lastThrottle was updated to t=4s.
	h.ReportProgressBytes("subepisode.com", 512*1024)
	h.ReportCompletedRange("subepisode.com", 4, now.Add(16*time.Second))
	cap, ok := h.ReportCompletedRange("subepisode.com", 4, now.Add(16*time.Second))
	if ok || cap != 2 {
		t.Fatalf("expected no recovery at t=16s due to sub-episode throttle at t=4s, got cap=%d ok=%v", cap, ok)
	}

	// At t=20s (16s since t=4s throttle > 15s RecoveryWindow), recovery should succeed
	cap, ok = h.ReportCompletedRange("subepisode.com", 4, now.Add(20*time.Second))
	if !ok || cap != 3 {
		t.Fatalf("expected recovery at t=20s (16s after sub-episode throttle), got cap=%d ok=%v", cap, ok)
	}
}

func TestHostRateLimiter_LowConcurrencyCapReduction(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	// Download configured for 2 max connections. Unknown host defaults to min(2, 4) = 2.
	cap := h.ConcurrencyCap("lowconn.com", 2)
	if cap != 2 {
		t.Fatalf("expected initial cap 2 for low max connections, got %d", cap)
	}

	// First throttle passes observed currentCap = 2
	_, newCap := h.ReportThrottle("lowconn.com", 2, 5*time.Second, true, now)
	if newCap != 1 {
		t.Fatalf("expected throttle on 2 connections to halve to cap=1, got %d", newCap)
	}
}

func TestHostRateLimiter_ZeroCurrentCapDoesNotAdapt(t *testing.T) {
	h := NewHostRateLimiter()
	h.ReportThrottle("disabled.com", 0, time.Second, true, time.Now())
	if cap := h.ConcurrencyCap("disabled.com", 8); cap != UnknownHostInitialCap {
		t.Fatalf("disabled adaptive concurrency changed learned cap to %d", cap)
	}
}

func TestHostRateLimiter_MultipleFlushesOneRangeDoesNotRecover(t *testing.T) {
	h := NewHostRateLimiter()
	now := time.Now()

	// Initial throttle to drop cap to 2
	h.ReportThrottle("flushes.com", 4, 2*time.Second, true, now)

	// Simulate 10 flushes on a single range (accumulating > 512 KB)
	for i := 0; i < 10; i++ {
		h.ReportProgressBytes("flushes.com", 100*1024)
	}

	// Only 1 range completed
	cap, ok := h.ReportCompletedRange("flushes.com", 8, now.Add(RecoveryWindow+time.Second))
	if ok || cap != 2 {
		t.Fatalf("expected cap=2 with only 1 completed range despite 1MB flushes, got cap=%d ok=%v", cap, ok)
	}
}
