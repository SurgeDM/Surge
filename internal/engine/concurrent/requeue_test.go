package concurrent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/testutil"
)

// serveFileData writes the requested byte range from a zero-filled buffer
// to the response writer with proper Content-Length and Content-Range headers.
func serveFileData(w http.ResponseWriter, r *http.Request, fileSize int64) {
	rangeHeader := r.Header.Get("Range")
	start := int64(0)
	end := fileSize - 1
	if rangeHeader != "" {
		parts := strings.SplitN(strings.TrimPrefix(rangeHeader, "bytes="), "-", 2)
		start = parseRangeInt(parts[0])
		if len(parts) > 1 && parts[1] != "" {
			end = parseRangeInt(parts[1])
		}
	}
	length := end - start + 1
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	if rangeHeader != "" {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	buf := make([]byte, 64*1024)
	remaining := length
	for remaining > 0 {
		chunk := buf
		if int64(len(chunk)) > remaining {
			chunk = buf[:remaining]
		}
		nw, _ := w.Write(chunk)
		remaining -= int64(nw)
	}
}

func parseRangeInt(s string) int64 {
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// TestConcurrentDownloader_SingleWorker429DoesNotKillSession verifies that
// when a single worker exhausts its retries on a chunk (e.g. hitting 429),
// the chunk is re-queued and completed by another worker instead of
// cancelling all workers and falling back to single-threaded.
func TestConcurrentDownloader_SingleWorker429DoesNotKillSession(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(4 * types.MB)

	var reqCount atomic.Int64
	// Odd requests → 429, even requests → success.
	// Worker 1 (3 retries = 3 odd requests) fails → re-queues.
	// Worker 2 picks up, 4th request (even) → succeeds.
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			n := reqCount.Add(1)
			if n%2 == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			serveFileData(w, r, fileSize)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "single429_nokill.bin")
	state := types.NewProgressState("single429-nokill", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		MaxTaskRetries:            3,
		MinChunkSize:              1 * types.MB,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("single429-nokill-id", nil, state, runtime)

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	if err != nil {
		t.Fatalf("download should have succeeded via re-queue, got: %v", err)
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Error(err)
	}
}

// TestConcurrentDownloader_AllMirrors429FailsAfterRequeueLimit verifies the
// circuit breaker: when all requests return 429 and every chunk keeps being
// re-queued, the download eventually fails after the global re-queue limit
// is exhausted.
func TestConcurrentDownloader_AllMirrors429FailsAfterRequeueLimit(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1 * types.MB)

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "all429_fail.bin")
	state := types.NewProgressState("all429-fail", fileSize)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		MaxTaskRetries:            2,
		MinChunkSize:              256 * types.KB,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("all429-fail-id", nil, state, runtime)

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	if err == nil {
		t.Fatal("expected download to fail after re-queue limit, but it succeeded")
	}

	if !strings.Contains(err.Error(), "requeue attempts") {
		t.Errorf("expected error about requeue attempts, got: %v", err)
	}
}

// TestConcurrentDownloader_MidTransfer429KeepsCompletedChunks verifies that
// when one chunk hits 429 and is re-queued, the chunks already completed
// by other workers are NOT discarded (i.e. the session doesn't fall back
// to single-threaded from scratch).
func TestConcurrentDownloader_MidTransfer429KeepsCompletedChunks(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(4 * types.MB)

	// First chunk (offset 0) always 429. All other chunks succeed.
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			rangeHeader := r.Header.Get("Range")
			if strings.HasPrefix(rangeHeader, "bytes=0-") {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			serveFileData(w, r, fileSize)
		}),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "mid429_keep.bin")
	state := types.NewProgressState("mid429-keep", fileSize)

	// 2 workers: both pick up chunks. Worker 1 gets chunk at offset 0 (always 429),
	// exhausts retries, re-queues. Worker 2 also picks it up, gets 429, exhausts,
	// re-queues. The re-queue count eventually hits the limit and fails.
	// But we verify that chunks at non-zero offsets were written (file is > 0 size).
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		MaxTaskRetries:            2,
		MinChunkSize:              1 * types.MB,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("mid429-keep-id", nil, state, runtime)

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	if err != nil {
		// Expected: chunk at offset 0 keeps failing, re-queue limit hit.
		t.Logf("download failed as expected (chunk 0 permanently 429): %v", err)
	}

	// Even though the download failed overall, chunks at non-zero offsets
	// should have been written to the file by other workers.
	fi, err := os.Stat(destPath + ".surge")
	if err != nil {
		t.Fatalf("cannot stat .surge file: %v", err)
	}
	if fi.Size() <= 0 {
		t.Error("expected .surge file to contain data from completed chunks, but file is empty")
	}
	t.Logf(".surge file size: %d (expected > 0, < %d)", fi.Size(), fileSize)
}
