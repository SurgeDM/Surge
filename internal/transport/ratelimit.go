package transport

import (
	"context"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

var DefaultHostRateLimiter = NewHostRateLimiter()

const (
	UnknownHostInitialCap  = 4
	RecoveryByteThreshold  = 512 * 1024 // 512 KB
	RecoveryRangeThreshold = 2
	RecoveryWindow         = 15 * time.Second
)

type hostPenalty struct {
	until            time.Time
	consecutive      int
	lastHit          time.Time
	concurrencyCap   int
	lastThrottle     time.Time
	successfulBytes  int64
	successfulRanges int
	lastProgress     time.Time
	lastRecovery     time.Time
	activeRequests   int
}

type HostRateLimiter struct {
	mu      sync.Mutex
	hosts   map[string]*hostPenalty
	changed chan struct{}
}

func NewHostRateLimiter() *HostRateLimiter {
	return &HostRateLimiter{
		hosts:   make(map[string]*hostPenalty),
		changed: make(chan struct{}),
	}
}

func (h *HostRateLimiter) notifyLocked() {
	close(h.changed)
	h.changed = make(chan struct{})
}

// AcquireRequest shares the learned connection budget across downloads using
// the same host. Existing streams keep their permits when the cap is reduced.
func (h *HostRateLimiter) AcquireRequest(ctx context.Context, host string, configuredMax int) (func(), error) {
	configuredMax = max(1, configuredMax)
	for {
		h.mu.Lock()
		if err := ctx.Err(); err != nil {
			h.mu.Unlock()
			return nil, err
		}
		p := h.hosts[host]
		if p == nil {
			p = &hostPenalty{concurrencyCap: min(UnknownHostInitialCap, configuredMax)}
			h.hosts[host] = p
		}
		wait := time.Until(p.until)
		if wait <= 0 && p.activeRequests < h.concurrencyCapLocked(host, configuredMax) {
			p.activeRequests++
			h.mu.Unlock()
			return func() {
				h.mu.Lock()
				p.activeRequests--
				h.notifyLocked()
				h.mu.Unlock()
			}, nil
		}
		changed := h.changed
		h.mu.Unlock()
		var timer *time.Timer
		var deadline <-chan time.Time
		if wait > 0 {
			timer = time.NewTimer(wait)
			deadline = timer.C
		}
		select {
		case <-ctx.Done():
		case <-changed:
		case <-deadline:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func (h *HostRateLimiter) ConcurrencyCap(host string, configuredMax int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.concurrencyCapLocked(host, configuredMax)
}

// ConcurrencyCapForHosts returns the conservative cap shared by a download
// that may use any of hosts. A single download-wide gate must never be raised
// above the lowest learned cap of its eligible hosts.
func (h *HostRateLimiter) ConcurrencyCapForHosts(hosts []string, configuredMax int) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	cap := configuredMax
	seen := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		if hostCap := h.concurrencyCapLocked(host, configuredMax); hostCap < cap {
			cap = hostCap
		}
	}
	return cap
}

func (h *HostRateLimiter) AnyBlocked(hosts []string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, host := range hosts {
		if p := h.hosts[host]; p != nil && now.Before(p.until) {
			return true
		}
	}
	return false
}

func (h *HostRateLimiter) concurrencyCapLocked(host string, configuredMax int) int {

	p, known := h.hosts[host]
	if !known || p.concurrencyCap <= 0 {
		if configuredMax < UnknownHostInitialCap {
			return configuredMax
		}
		return UnknownHostInitialCap
	}
	if p.concurrencyCap > configuredMax {
		return configuredMax
	}
	return p.concurrencyCap
}

func (h *HostRateLimiter) Penalize(host string, retryAfter time.Duration, explicit bool, now time.Time) time.Time {
	until, _ := h.ReportThrottle(host, 0, retryAfter, explicit, now)
	return until
}

func (h *HostRateLimiter) ReportThrottle(host string, currentCap int, retryAfter time.Duration, explicit bool, now time.Time) (time.Time, int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	p, ok := h.hosts[host]
	if !ok {
		p = &hostPenalty{concurrencyCap: UnknownHostInitialCap}
		h.hosts[host] = p
	}

	if p.concurrencyCap <= 0 {
		p.concurrencyCap = UnknownHostInitialCap
	}

	newEpisode := p.until.IsZero() || !now.Before(p.until)
	if newEpisode && currentCap > 0 {
		base := p.concurrencyCap
		if currentCap < base {
			base = currentCap
		}
		p.concurrencyCap = max(1, base/2)
		p.successfulBytes = 0
		p.successfulRanges = 0
	}
	p.lastThrottle = now

	if now.Sub(p.lastHit) > types.RateLimitPenaltyDecay {
		p.consecutive = 0
	}
	if newEpisode {
		p.consecutive++
	}
	p.lastHit = now

	var d time.Duration
	if explicit {
		d = min(retryAfter, time.Duration(1<<63-1)-types.RateLimitMaxBackoff)
	} else {
		d = types.RateLimitBaseBackoff * time.Duration(int64(1)<<min(max(p.consecutive-1, 0), 5))
	}

	if d < types.RateLimitMinBackoff {
		d = types.RateLimitMinBackoff
	}
	if !explicit && d > types.RateLimitMaxBackoff {
		d = types.RateLimitMaxBackoff
	}

	jitterRange := int64(float64(min(d, types.RateLimitMaxBackoff)) * types.RateLimitJitterFraction)
	if jitterRange > 0 {
		delta := rand.Int64N(jitterRange)
		if !explicit {
			delta = rand.Int64N(2*jitterRange) - jitterRange
		}
		d += time.Duration(delta)
	}
	if d < types.RateLimitMinBackoff {
		d = types.RateLimitMinBackoff
	}
	if !explicit && d > types.RateLimitMaxBackoff {
		d = types.RateLimitMaxBackoff
	}

	deadline := now.Add(d)
	if deadline.After(p.until) {
		p.until = deadline
	}

	h.cleanupLocked()
	h.notifyLocked()
	return p.until, p.concurrencyCap
}

func (h *HostRateLimiter) ReportProgressBytes(host string, bytes int64) {
	if bytes <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	p, ok := h.hosts[host]
	if !ok {
		p = &hostPenalty{concurrencyCap: UnknownHostInitialCap}
		h.hosts[host] = p
	}
	p.successfulBytes += bytes
	p.lastProgress = time.Now()
}

// ReportHealthyProgress recovers at most one connection per sustained healthy
// window. Large unfinished ranges can prove health without completing a task.
func (h *HostRateLimiter) ReportHealthyProgress(host string, bytes int64, configuredMax int, interval time.Duration, now time.Time) (int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.hosts[host]
	if p == nil {
		p = &hostPenalty{concurrencyCap: min(UnknownHostInitialCap, configuredMax), lastRecovery: now}
		h.hosts[host] = p
	}
	p.successfulBytes += bytes
	p.lastProgress = now
	if p.lastRecovery.IsZero() {
		p.lastRecovery = now
	}
	anchor := p.lastRecovery
	if p.lastThrottle.After(anchor) {
		anchor = p.lastThrottle
	}
	if interval > 0 && !now.Before(p.until) && now.Sub(anchor) >= interval &&
		p.successfulBytes >= RecoveryByteThreshold && p.concurrencyCap < configuredMax {
		p.concurrencyCap++
		p.successfulBytes = 0
		p.lastRecovery = now
		h.notifyLocked()
		return p.concurrencyCap, true
	}
	return min(p.concurrencyCap, configuredMax), false
}

func (h *HostRateLimiter) ReportCompletedRange(host string, configuredMax int, now time.Time) (int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	p, ok := h.hosts[host]
	if !ok {
		p = &hostPenalty{concurrencyCap: UnknownHostInitialCap}
		h.hosts[host] = p
	}

	if p.concurrencyCap <= 0 {
		p.concurrencyCap = UnknownHostInitialCap
	}

	p.successfulRanges++

	if p.successfulBytes >= RecoveryByteThreshold &&
		p.successfulRanges >= RecoveryRangeThreshold &&
		(p.lastThrottle.IsZero() || now.Sub(p.lastThrottle) >= RecoveryWindow) {
		if p.concurrencyCap < configuredMax {
			p.concurrencyCap++
			p.successfulBytes = 0
			p.successfulRanges = 0
			return p.concurrencyCap, true
		}
	}

	return p.concurrencyCap, false
}

func (h *HostRateLimiter) BlockedUntil(host string, now time.Time) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()

	p, ok := h.hosts[host]
	if !ok {
		return time.Time{}
	}
	if now.Before(p.until) {
		return p.until
	}
	return time.Time{}
}

func (h *HostRateLimiter) RecordSuccess(host string) {
	h.recordSuccess(host, time.Now())
}

func (h *HostRateLimiter) recordSuccess(host string, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	p, ok := h.hosts[host]
	if !ok || now.Before(p.until) {
		return
	}
	p.until = time.Time{}
	p.consecutive = 0
	p.lastHit = time.Time{}
}

func (h *HostRateLimiter) PickMirror(hosts []string, startIdx int, now time.Time) (int, time.Duration) {
	if len(hosts) == 0 {
		return 0, 0
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	firstFree := -1
	earliestIdx := -1
	var earliestDeadline time.Time

	n := len(hosts)
	for i := 0; i < n; i++ {
		idx := (startIdx + i) % n
		host := hosts[idx]
		p, ok := h.hosts[host]
		if !ok || now.After(p.until) {
			firstFree = idx
			break
		}
		if earliestIdx == -1 || p.until.Before(earliestDeadline) {
			earliestIdx = idx
			earliestDeadline = p.until
		}
	}

	if firstFree >= 0 {
		return firstFree, 0
	}

	wait := earliestDeadline.Sub(now)
	if wait < 0 {
		wait = 0
	}
	return earliestIdx, wait
}

func (h *HostRateLimiter) cleanupLocked() {
	now := time.Now()
	for host, p := range h.hosts {
		lastActivity := p.lastHit
		if p.lastProgress.After(lastActivity) {
			lastActivity = p.lastProgress
		}
		if p.lastThrottle.After(lastActivity) {
			lastActivity = p.lastThrottle
		}
		if p.activeRequests == 0 && now.After(p.until) && now.Sub(lastActivity) > 30*time.Minute {
			delete(h.hosts, host)
		}
	}
}

func ParseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}

	if n, err := strconv.ParseInt(header, 10, 64); err == nil {
		if n > int64((time.Duration(1<<63-1)-types.RateLimitMaxBackoff)/time.Second) {
			return time.Duration(1<<63-1) - types.RateLimitMaxBackoff, true
		}
		if n < 0 {
			return 0, false
		}
		return time.Duration(n) * time.Second, true
	}

	t, err := http.ParseTime(header)
	if err != nil {
		return 0, false
	}
	d := t.Sub(now)
	return d, true
}

func MirrorHost(rawurl string) string {
	u, err := url.Parse(rawurl)
	if err != nil {
		return rawurl
	}
	return u.Host
}
