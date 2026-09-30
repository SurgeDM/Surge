package concurrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestBare503ConnectionLimitPreservesHealthyStreams(t *testing.T) {
	dir, cleanup := initTestState(t)
	defer cleanup()
	payload := bytes.Repeat([]byte("FileQ regression payload\n"), 32768)
	var active, throttles, canceled atomic.Int32
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if active.Add(1) > 4 {
			active.Add(-1)
			throttles.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		defer active.Add(-1)
		start, end, err := parseInclusiveByteRange(r.Header.Get("Range"), int64(len(payload)))
		if err != nil {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		select {
		case <-time.After(250 * time.Millisecond):
		case <-r.Context().Done():
			canceled.Add(1)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer server.Close()
	path := filepath.Join(dir, "limited.bin")
	if err := os.WriteFile(path+types.IncompleteSuffix, nil, 0600); err != nil {
		t.Fatal(err)
	}
	d := NewConcurrentDownloader("limited", nil, progress.New("limited", int64(len(payload))), &types.RuntimeConfig{
		Workers: 8, MaxConnectionsPerDownload: 8, MinChunkSize: 32768,
		AdaptiveConcurrencyInterval: 15 * time.Second,
	})
	d.hostLimiter = transport.NewHostRateLimiter()
	host := transport.MirrorHost(server.URL)
	// Simulate a host previously proven capable of eight connections.
	now := time.Now().Add(-2 * time.Minute)
	for i := 0; i < 5; i++ {
		d.hostLimiter.ReportHealthyProgress(host, 1<<20, 8, time.Second, now.Add(time.Duration(i)*20*time.Second))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Download(ctx, server.URL, nil, nil, path, int64(len(payload))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path + types.IncompleteSuffix)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download content mismatch: %v", err)
	}
	if throttles.Load() == 0 || canceled.Load() != 0 {
		t.Fatalf("throttles=%d canceled healthy requests=%d", throttles.Load(), canceled.Load())
	}
	if cap := d.hostLimiter.ConcurrencyCap(host, 8); cap > 4 {
		t.Fatalf("learned cap = %d, want <=4", cap)
	}
}

func TestIgnoredRangeMirrorDoesNotDiscardHealthyMirror(t *testing.T) {
	dir, cleanup := initTestState(t)
	defer cleanup()
	const size = int64(128 * 1024)
	bad := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, size))
	}))
	defer bad.Close()
	good := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveRequestedRange(w, r, size)
	}))
	defer good.Close()
	path := filepath.Join(dir, "mirrors.bin")
	if err := os.WriteFile(path+types.IncompleteSuffix, nil, 0600); err != nil {
		t.Fatal(err)
	}
	d := NewConcurrentDownloader("mirrors", nil, progress.New("mirrors", size), &types.RuntimeConfig{
		Workers: 2, MaxConnectionsPerDownload: 2, MinChunkSize: 32768,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Download(ctx, bad.URL, []string{good.URL}, []string{good.URL}, path, size); err != nil {
		t.Fatal(err)
	}
}

func TestPersistent503CannotResetThrottleBudget(t *testing.T) {
	dir, cleanup := initTestState(t)
	defer cleanup()
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	path := filepath.Join(dir, "unavailable.bin")
	if err := os.WriteFile(path+types.IncompleteSuffix, nil, 0600); err != nil {
		t.Fatal(err)
	}
	d := NewConcurrentDownloader("unavailable", nil, progress.New("unavailable", 65536), nil)
	d.ImportThrottleState(&types.DownloadRecord{ThrottleEpisodeStart: time.Now().Add(-11 * time.Minute)})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := d.Download(ctx, server.URL, nil, nil, path, 65536)
	if !errors.Is(err, types.ErrRetryBudgetExceeded) {
		t.Fatalf("error=%v, want exhausted throttle budget", err)
	}
}

func TestTwoDownloadsShareLearnedHostConnectionBudget(t *testing.T) {
	dir, cleanup := initTestState(t)
	defer cleanup()
	const size = int64(128 * 1024)
	var active, peak atomic.Int32
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); n > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, n) {
				break
			}
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		serveRequestedRange(w, r, size)
	}))
	defer server.Close()
	limiter := transport.NewHostRateLimiter()
	limiter.ReportThrottle(transport.MirrorHost(server.URL), 4, time.Second, true, time.Now().Add(-3*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("shared-host-%d", i)
		path := filepath.Join(dir, id)
		if err := os.WriteFile(path+types.IncompleteSuffix, nil, 0600); err != nil {
			t.Fatal(err)
		}
		d := NewConcurrentDownloader(id, nil, progress.New(id, size), &types.RuntimeConfig{
			Workers: 4, MaxConnectionsPerDownload: 4, MinChunkSize: 32768,
			AdaptiveConcurrencyInterval: time.Minute,
		})
		d.hostLimiter = limiter
		go func() { results <- d.Download(ctx, server.URL, nil, nil, path, size) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if peak.Load() > 2 || peak.Load() == 0 {
		t.Fatalf("aggregate host requests=%d, want <= learned cap 2", peak.Load())
	}
}
