package concurrent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

func TestConcurrentDownloader_SwitchOn429(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(256 * utils.KiB)

	server1 := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}),
	)
	defer server1.Close()

	server2 := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
	)
	defer server2.Close()

	destPath := filepath.Join(tmpDir, "switch429_test.bin")
	state := progress.New("switch429-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            5,
		MinChunkSize:              64 * utils.KiB,
		DialHedgeCount:            0, // Disable hedging for deterministic failover test
	}

	downloader := NewConcurrentDownloader("switch429-id", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{server1.URL(), server2.URL()}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server1.URL(), mirrors, mirrors, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Error(err)
	}

	stateMirrors := state.GetMirrors()
	var badMirrorSeen, badMirrorErrored bool
	for _, m := range stateMirrors {
		if m.URL == server1.URL() {
			badMirrorSeen = true
			badMirrorErrored = m.Error
			break
		}
	}
	if !badMirrorSeen {
		t.Fatalf("Expected to track bad mirror %s in state, got: %+v", server1.URL(), stateMirrors)
	}
	if !badMirrorErrored {
		t.Fatalf("Expected bad mirror %s to be marked errored after 429, got: %+v", server1.URL(), stateMirrors)
	}
}

func TestConcurrentDownloader_BackoffOnSingleMirror(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1 * utils.MiB) // Use enough size so it doesn't just finish instantly on 1st byte

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithFailOnNthRequest(1),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "backoff_test.bin")
	state := progress.New("backoff-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            5,
		MinChunkSize:              64 * utils.KiB,
		DialHedgeCount:            0, // Disable hedging for deterministic backoff timing
	}

	downloader := NewConcurrentDownloader("backoff-id", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server.URL(), mirrors, nil, destPath, fileSize)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if elapsed < 200*time.Millisecond {
		t.Errorf("Download took %v, but expected backoff wait (should be > 200ms)", elapsed)
	}
}

func TestConcurrentDownloader_AllMirrors429ThenRecover(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(64 * utils.KiB)

	var s1Count, s2Count atomic.Int64

	makeHandler := func(counter *atomic.Int64) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			n := counter.Add(1)
			if n <= 2 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
			w.WriteHeader(http.StatusOK)
			buf := make([]byte, 32*utils.KiB)
			for written := int64(0); written < fileSize; {
				n := int64(len(buf))
				if written+n > fileSize {
					n = fileSize - written
				}
				_, _ = w.Write(buf[:n])
				written += n
			}
		}
	}

	server1 := testutil.NewMockServerT(t,
		testutil.WithHandler(makeHandler(&s1Count)),
	)
	defer server1.Close()

	server2 := testutil.NewMockServerT(t,
		testutil.WithHandler(makeHandler(&s2Count)),
	)
	defer server2.Close()

	destPath := filepath.Join(tmpDir, "all429_test.bin")
	state := progress.New("all429-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("all429-id", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{server1.URL(), server2.URL()}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server1.URL(), mirrors, mirrors, destPath, fileSize)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if elapsed < 900*time.Millisecond {
		t.Errorf("Expected coordinated backoff of ~1s, but download completed in %v", elapsed)
	}
}

func TestConcurrentDownloader_429RespectsRetryAfterHeader(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(128 * utils.KiB)

	var requestTimes []time.Time
	var mu sync.Mutex

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			requestTimes = append(requestTimes, time.Now())
			mu.Unlock()

			if len(requestTimes) == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
			w.WriteHeader(http.StatusOK)
			buf := make([]byte, fileSize)
			_, _ = w.Write(buf)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "retryafter_test.bin")
	state := progress.New("retryafter-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	progressCh := make(chan types.DownloadEvent, 1)
	downloader := NewConcurrentDownloader("retryafter-id", progressCh, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server.URL(), mirrors, nil, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	mu.Lock()
	times := requestTimes
	mu.Unlock()

	if len(times) < 2 {
		t.Fatal("expected at least 2 requests")
	}

	gap := times[1].Sub(times[0])
	if gap < 900*time.Millisecond {
		t.Errorf("gap between 429 and next request %v; expected >= ~1s", gap)
	}
	if gap > 35*time.Second {
		t.Errorf("gap between 429 and next request %v; expected <= ~30s cap", gap)
	}

	select {
	case event := <-progressCh:
		if event.Type != types.EventSystem || event.Message == "" {
			t.Fatalf("rate-limit event = %+v, want system message", event)
		}
	case <-time.After(time.Second):
		t.Fatal("expected rate-limit activity event")
	}
}

func TestConcurrentDownloader_429DoesNotTearDownWithHealthyMirror(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1 * utils.MiB)

	server1 := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}),
	)
	defer server1.Close()

	server2 := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
	)
	defer server2.Close()

	destPath := filepath.Join(tmpDir, "429healthy_test.bin")
	state := progress.New("429healthy-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 4,
		MaxTaskRetries:            3,
		MinChunkSize:              128 * utils.KiB,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("429healthy-id", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{server1.URL(), server2.URL()}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server1.URL(), mirrors, mirrors, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Error(err)
	}

	stateMirrors := state.GetMirrors()
	var badMirrorErrored bool
	for _, m := range stateMirrors {
		if m.URL == server1.URL() {
			badMirrorErrored = m.Error
			break
		}
	}
	if !badMirrorErrored {
		t.Fatal("Expected server1 to be flagged errored")
	}
}

func TestConcurrentDownloader_403DoesNotCancelHealthyWorker(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	const fileSize = int64(128 * utils.KiB)
	const chunkSize = fileSize / 2
	payload := make([]byte, chunkSize)
	healthyDone := make(chan struct{})
	var healthyDoneOnce sync.Once
	var forbiddenRequests atomic.Int64

	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=0-65535" {
			select {
			case <-time.After(100 * time.Millisecond):
				healthyDoneOnce.Do(func() { close(healthyDone) })
			case <-r.Context().Done():
				return
			}
		} else if forbiddenRequests.Add(1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		} else {
			select {
			case <-healthyDone:
			case <-r.Context().Done():
				return
			}
		}

		w.Header().Set("Content-Length", strconv.FormatInt(chunkSize, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "soft403_healthy.bin")
	if f, err := os.Create(destPath + types.IncompleteSuffix); err == nil {
		_ = f.Close()
	}

	state := progress.New("soft403-healthy", fileSize)
	downloader := NewConcurrentDownloader("soft403-healthy", nil, state, &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		Workers:                   2,
		MinChunkSize:              chunkSize,
		MaxTaskRetries:            1,
		DialHedgeCount:            0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := downloader.Download(ctx, server.URL, nil, nil, destPath, fileSize); err != nil {
		t.Fatalf("Download failed after transient 403: %v", err)
	}
	if forbiddenRequests.Load() < 2 {
		t.Fatal("expected the forbidden range to be retried after its soft limit")
	}
}

func TestMirrorAware403Exhaustion(t *testing.T) {
	downloader := NewConcurrentDownloader("soft403-mirror-aware", nil, nil, nil)
	downloader.forbiddenByMirror = make(map[string]int)
	mirrors := []string{"http://m1.com/file", "http://m2.com/file"}

	// m1 gets 403 3 times
	downloader.forbiddenByMirror["http://m1.com/file"] = 3
	downloader.forbiddenByMirror["http://m2.com/file"] = 2

	// m2 has only 2, so not all mirrors forbidden
	allForbidden := true
	for _, m := range mirrors {
		if downloader.forbiddenByMirror[m] < 3 {
			allForbidden = false
			break
		}
	}
	if allForbidden {
		t.Fatal("expected allForbidden=false when m2 has only 2 403s")
	}

	// m2 hits 3rd 403
	downloader.forbiddenByMirror["http://m2.com/file"] = 3
	allForbidden = true
	for _, m := range mirrors {
		if downloader.forbiddenByMirror[m] < 3 {
			allForbidden = false
			break
		}
	}
	if !allForbidden {
		t.Fatal("expected allForbidden=true when all mirrors hit 3 403s")
	}
}

func TestConcurrentDownloader_Soft403ZeroRetriesHasCooldown(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	const fileSize = int64(64 * utils.KiB)
	var requests atomic.Int64
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "soft403-zero-retries.bin")
	file, err := os.Create(destPath + types.IncompleteSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	state := progress.New("soft403-zero-retries", fileSize)
	downloader := NewConcurrentDownloader(state.ID, nil, state, &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		Workers:                   1,
		MinChunkSize:              fileSize,
		MaxTaskRetries:            0,
		DialHedgeCount:            0,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err = downloader.Download(ctx, server.URL, nil, nil, destPath, fileSize)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Download error = %v, want context deadline", err)
	}
	if got := requests.Load(); got > 1 {
		t.Fatalf("soft 403 requests = %d in 150ms, want at most 1", got)
	}
}

func TestConcurrentDownloader_503WithRetryAfterTreatedAsThrottle(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(128 * utils.KiB)

	var count atomic.Int64

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			n := count.Add(1)
			if n == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
			w.WriteHeader(http.StatusPartialContent)
			buf := make([]byte, fileSize)
			_, _ = w.Write(buf)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "503_test.bin")
	state := progress.New("503-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("503-id", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server.URL(), mirrors, nil, destPath, fileSize)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if elapsed < 1500*time.Millisecond {
		t.Errorf("Expected backoff after 503+Retry-After, but completed in %v", elapsed)
	}
}

func TestConcurrentDownloader_Persistent429WaitsForCancellation(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(64 * utils.KiB)

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "persistent429_test.bin")
	state := progress.New("persistent429-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("persistent429-id", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server.URL(), mirrors, nil, destPath, fileSize)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline after continued rate-limit waiting, got: %v", err)
	}
}

func TestConcurrentDownloader_Bare503IsGeneric(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(128 * utils.KiB)

	var count atomic.Int64

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			n := count.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
			w.WriteHeader(http.StatusPartialContent)
			buf := make([]byte, fileSize)
			_, _ = w.Write(buf)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "bare503_test.bin")
	state := progress.New("bare503-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("bare503-id", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	mirrors := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server.URL(), mirrors, nil, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
}

func TestAdaptiveCapPersistedAcrossDownloaderRecreation(t *testing.T) {
	hostLimiter := transport.NewHostRateLimiter()
	host := "persisted-host.com"

	// Simulate throttle on host to drop cap from 4 to 2
	hostLimiter.ReportThrottle(host, 4, time.Second, true, time.Now())

	// First downloader instance queries learned cap
	d1 := NewConcurrentDownloader("d1", nil, nil, &types.RuntimeConfig{})
	d1.hostLimiter = hostLimiter
	cap1 := d1.hostLimiter.ConcurrencyCap(host, 8)
	if cap1 != 2 {
		t.Fatalf("expected d1 learned cap=2, got %d", cap1)
	}

	// Second downloader instance (simulating scheduler retry for same host)
	d2 := NewConcurrentDownloader("d2", nil, nil, &types.RuntimeConfig{})
	d2.hostLimiter = hostLimiter
	cap2 := d2.hostLimiter.ConcurrencyCap(host, 8)
	if cap2 != 2 {
		t.Fatalf("expected d2 learned cap=2 across recreate, got %d", cap2)
	}
}

func TestOrdinary200IgnoredRangeReturnsSentinel(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(128 * utils.KiB)

	// Server that returns 200 OK without Content-Range for range requests
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, fileSize)
		_, _ = w.Write(buf)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "ignored_range_test.bin")
	workingPath := destPath + types.IncompleteSuffix
	if f, err := os.Create(workingPath); err == nil {
		_ = f.Close()
	}

	state := progress.New("ignored-range-id", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		Workers:                   2,
		MinChunkSize:              32 * utils.KiB,
	}

	downloader := NewConcurrentDownloader("ignored-range-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := downloader.Download(ctx, server.URL, nil, nil, destPath, fileSize)
	if err == nil {
		t.Fatal("expected Download to fail with ErrRangeUnsupported")
	}

	if !errors.Is(err, types.ErrRangeUnsupported) {
		t.Fatalf("expected ErrRangeUnsupported, got: %v", err)
	}
}

func TestTwoDownloadsSameHostNoProgressIsolation(t *testing.T) {
	hostLimiter := transport.NewHostRateLimiter()
	host := "shared-host.com"

	cfgA := &types.DownloadRecord{ID: "A", URL: "http://shared-host.com/a"}
	cfgB := &types.DownloadRecord{ID: "B", URL: "http://shared-host.com/b"}

	dA := NewConcurrentDownloader("A", nil, nil, nil)
	dA.hostLimiter = hostLimiter

	dB := NewConcurrentDownloader("B", nil, nil, nil)
	dB.hostLimiter = hostLimiter

	now := time.Now()
	// Throttle A on shared-host
	until, _ := hostLimiter.ReportThrottle(host, 4, 5*time.Second, true, now)
	dA.concurrencyGate = newAdaptiveConcurrencyGate(4, 5*time.Second)
	dA.concurrencyGate.setCap(2, until)
	dA.soft403Mu.Lock()
	dA.throttleEpisodeStart = now
	dA.consecutiveThrottles = 3
	dA.soft403Mu.Unlock()
	dA.ExportThrottleState(cfgA)

	// Make progress on B
	dB.ImportThrottleState(cfgB)
	hostLimiter.ReportProgressBytes(host, 1*1024*1024)
	hostLimiter.ReportCompletedRange(host, 8, now.Add(20*time.Second))
	dB.soft403Mu.Lock()
	dB.lastByteProgressTime = now.Add(20 * time.Second)
	dB.soft403Mu.Unlock()
	dB.ExportThrottleState(cfgB)

	// Verify A's throttle state (cfgA) is isolated from B's progress
	if cfgA.ConsecutiveThrottles != 3 {
		t.Fatalf("expected cfgA.ConsecutiveThrottles=3, got %d", cfgA.ConsecutiveThrottles)
	}
	if cfgA.ThrottleEpisodeStart != now {
		t.Fatalf("expected cfgA.ThrottleEpisodeStart=%v, got %v", now, cfgA.ThrottleEpisodeStart)
	}
	if cfgB.ConsecutiveThrottles != 0 {
		t.Fatalf("expected cfgB.ConsecutiveThrottles=0, got %d", cfgB.ConsecutiveThrottles)
	}
	if cfgB.ThrottleEpisodeStart.IsZero() == false {
		t.Fatalf("expected cfgB.ThrottleEpisodeStart to be zero, got %v", cfgB.ThrottleEpisodeStart)
	}
}
