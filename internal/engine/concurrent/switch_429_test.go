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

	"github.com/SurgeDM/Surge/internal/engine"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/testutil"
)

func TestConcurrentDownloader_SwitchOn429(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(256 * types.KB)

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
	state := types.NewProgressState("switch429-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            5,
		MinChunkSize:              64 * types.KB,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("switch429-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

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

	fileSize := int64(1 * types.MB)

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithFailOnNthRequest(1),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "backoff_test.bin")
	state := types.NewProgressState("backoff-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            5,
		MinChunkSize:              64 * types.KB,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("backoff-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

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

	fileSize := int64(64 * types.KB)

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
			buf := make([]byte, 32*types.KB)
			for written := int64(0); written < fileSize; {
				n := int64(len(buf))
				if written+n > fileSize {
					n = fileSize - written
				}
				w.Write(buf[:n])
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
	state := types.NewProgressState("all429-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("all429-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

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

	fileSize := int64(128 * types.KB)

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
			w.Write(buf)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "retryafter_test.bin")
	state := types.NewProgressState("retryafter-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("retryafter-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

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
}

func TestConcurrentDownloader_429DoesNotTearDownWithHealthyMirror(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1 * types.MB)

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
	state := types.NewProgressState("429healthy-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 4,
		MaxTaskRetries:            3,
		MinChunkSize:              128 * types.KB,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("429healthy-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

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

func TestConcurrentDownloader_503WithRetryAfterTreatedAsThrottle(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(128 * types.KB)

	var count atomic.Int64

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			n := count.Add(1)
			if n == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
			w.WriteHeader(http.StatusPartialContent)
			buf := make([]byte, fileSize)
			w.Write(buf)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "503_test.bin")
	state := types.NewProgressState("503-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("503-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

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

	if elapsed < 900*time.Millisecond {
		t.Errorf("Expected backoff after 503+Retry-After, but completed in %v", elapsed)
	}
}

func TestConcurrentDownloader_Persistent429ExhaustsBudget(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(64 * types.KB)

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
	state := types.NewProgressState("persistent429-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("persistent429-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

	mirrors := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	err := downloader.Download(ctx, server.URL(), mirrors, nil, destPath, fileSize)
	if err == nil {
		t.Fatal("expected download to fail after exhausting rate-limit budget")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate-limit error, got: %v", err)
	}
}

func TestConcurrentDownloader_Bare503IsGeneric(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(128 * types.KB)

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
			w.Write(buf)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "bare503_test.bin")
	state := types.NewProgressState("bare503-test", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		MaxTaskRetries:            3,
		MinChunkSize:              fileSize,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("bare503-id", nil, state, runtime)
	downloader.hostLimiter = engine.NewHostRateLimiter()

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
