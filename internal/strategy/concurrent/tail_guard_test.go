package concurrent

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

func TestStealWork_TailGuardThresholds(t *testing.T) {
	const minChunk = 16 * types.AlignSize

	newDownloaderWithWorkers := func(adaptive bool, tasks map[int]int64) (*ConcurrentDownloader, map[int]*ActiveTask) {
		runtime := &types.RuntimeConfig{MinChunkSize: minChunk}
		if adaptive {
			runtime.AdaptiveConcurrencyInterval = time.Second
		}
		downloader := NewConcurrentDownloader("tail-guard-test", nil, nil, runtime)
		activeMap := make(map[int]*ActiveTask)
		for id, rem := range tasks {
			active := &ActiveTask{Task: types.Task{Offset: 0, Length: rem}}
			active.CurrentOffset.Store(0)
			active.StopAt.Store(rem)
			downloader.activeTasks[id] = active
			activeMap[id] = active
		}
		return downloader, activeMap
	}

	t.Run("single worker early phase blocked", func(t *testing.T) {
		downloader, activeMap := newDownloaderWithWorkers(true, map[int]int64{
			1: 32 * types.AlignSize,
		})
		queue := NewTaskQueue()

		if downloader.StealWork(queue) {
			t.Fatal("StealWork succeeded during early phase when totalRemaining >= activeWorkers*defaultMinChunk")
		}
		if queue.Len() != 0 {
			t.Fatalf("queue length = %d, want 0", queue.Len())
		}
		if activeMap[1].StopAt.Load() != 32*types.AlignSize {
			t.Fatalf("active worker StopAt was modified to %d", activeMap[1].StopAt.Load())
		}
	})

	t.Run("multiple workers early phase blocked", func(t *testing.T) {
		downloader, activeMap := newDownloaderWithWorkers(true, map[int]int64{
			1: 20 * types.AlignSize,
			2: 20 * types.AlignSize,
		})
		queue := NewTaskQueue()

		// totalRemaining = 40 * AlignSize >= 2 * 16 * AlignSize (32)
		if downloader.StealWork(queue) {
			t.Fatal("StealWork succeeded when multiple workers had totalRemaining >= activeWorkers*defaultMinChunk")
		}
		if queue.Len() != 0 {
			t.Fatalf("queue length = %d, want 0", queue.Len())
		}
		for id, active := range activeMap {
			if active.StopAt.Load() != 20*types.AlignSize {
				t.Fatalf("worker %d StopAt modified to %d", id, active.StopAt.Load())
			}
		}
	})

	t.Run("exact threshold boundary blocked", func(t *testing.T) {
		// totalRemaining = 32 * AlignSize == 2 * 16 * AlignSize
		downloader, _ := newDownloaderWithWorkers(true, map[int]int64{
			1: 16 * types.AlignSize,
			2: 16 * types.AlignSize,
		})
		queue := NewTaskQueue()

		if downloader.StealWork(queue) {
			t.Fatal("StealWork succeeded at exact threshold boundary")
		}
	})

	t.Run("tail phase allowed with multiple workers", func(t *testing.T) {
		// totalRemaining = 20 * AlignSize < 2 * 16 * AlignSize (32)
		downloader, activeMap := newDownloaderWithWorkers(true, map[int]int64{
			1: 10 * types.AlignSize,
			2: 10 * types.AlignSize,
		})
		queue := NewTaskQueue()

		if !downloader.StealWork(queue) {
			t.Fatal("StealWork failed in tail phase when totalRemaining < activeWorkers*defaultMinChunk")
		}
		if queue.Len() != 1 {
			t.Fatalf("queue length = %d, want 1", queue.Len())
		}
		stolenTasks := queue.DrainRemaining()
		stolen := stolenTasks[0]
		if stolen.Length == 0 {
			t.Fatal("stolen task length is zero")
		}

		// Verify that exactly one worker had its StopAt adjusted
		var modifiedCount int
		for _, active := range activeMap {
			if active.StopAt.Load() != 10*types.AlignSize {
				modifiedCount++
			}
		}
		if modifiedCount != 1 {
			t.Fatalf("modified worker count = %d, want 1", modifiedCount)
		}
	})

	t.Run("tail phase allowed with single worker approaching EOF", func(t *testing.T) {
		// totalRemaining = 8 * AlignSize < 1 * 16 * AlignSize (16)
		downloader, activeMap := newDownloaderWithWorkers(true, map[int]int64{
			1: 8 * types.AlignSize,
		})
		queue := NewTaskQueue()

		if !downloader.StealWork(queue) {
			t.Fatal("StealWork failed for single worker approaching EOF")
		}
		if queue.Len() != 1 {
			t.Fatalf("queue length = %d, want 1", queue.Len())
		}
		newStop := activeMap[1].StopAt.Load()
		if newStop >= 8*types.AlignSize {
			t.Fatalf("worker StopAt %d was not reduced", newStop)
		}
	})

	t.Run("zero active workers returns false", func(t *testing.T) {
		runtime := &types.RuntimeConfig{MinChunkSize: minChunk}
		downloader := NewConcurrentDownloader("tail-guard-zero", nil, nil, runtime)
		queue := NewTaskQueue()

		if downloader.StealWork(queue) {
			t.Fatal("StealWork succeeded with zero active workers")
		}
	})

	t.Run("workers with zero remaining work not counted", func(t *testing.T) {
		runtime := &types.RuntimeConfig{MinChunkSize: minChunk}
		downloader := NewConcurrentDownloader("tail-guard-zero-rem", nil, nil, runtime)
		active := &ActiveTask{Task: types.Task{Offset: 0, Length: 100}}
		active.CurrentOffset.Store(100)
		active.StopAt.Store(100)
		downloader.activeTasks[1] = active
		queue := NewTaskQueue()

		if downloader.StealWork(queue) {
			t.Fatal("StealWork succeeded when worker had zero remaining bytes")
		}
	})

	t.Run("non-empty queue returns false without stealing", func(t *testing.T) {
		downloader, activeMap := newDownloaderWithWorkers(true, map[int]int64{
			1: 8 * types.AlignSize,
		})
		queue := NewTaskQueue()
		queue.Push(types.Task{Offset: 1000, Length: 2000})

		if downloader.StealWork(queue) {
			t.Fatal("StealWork succeeded when queue was non-empty")
		}
		if activeMap[1].StopAt.Load() != 8*types.AlignSize {
			t.Fatalf("active task StopAt modified when queue was non-empty")
		}
	})

	t.Run("nil queue returns false", func(t *testing.T) {
		downloader, _ := newDownloaderWithWorkers(true, map[int]int64{
			1: 8 * types.AlignSize,
		})
		if downloader.StealWork(nil) {
			t.Fatal("StealWork succeeded with nil queue")
		}
	})
}

func TestConcurrentDownloader_403BackedOffWorkerDoesNotPreemptHealthyWorker(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	const chunkSize = int64(128 * utils.KiB)
	const fileSize = 2 * chunkSize
	payload := make([]byte, chunkSize)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	var worker0Preempted atomic.Bool
	var forbiddenCount atomic.Int64
	var healthyCompleted atomic.Bool

	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")

		// Primary range for worker 0 (first half)
		if rangeHeader == "bytes=0-131071" {
			// Simulate steady throughput while peer encounters 403
			time.Sleep(150 * time.Millisecond)
			w.Header().Set("Content-Length", strconv.FormatInt(chunkSize, 10))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload)
			healthyCompleted.Store(true)
			return
		}

		// If a split occurs, an unexpected sub-range will be requested
		if rangeHeader == "bytes=0-65535" || rangeHeader == "bytes=65536-131071" {
			worker0Preempted.Store(true)
			w.Header().Set("Content-Length", strconv.FormatInt(chunkSize/2, 10))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:chunkSize/2])
			return
		}

		// Range for worker 1 (second half): simulate transient 403 rate limits
		if forbiddenCount.Add(1) <= 2 {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// After backoff, serve second half
		w.Header().Set("Content-Length", strconv.FormatInt(chunkSize, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "tail_guard_403.bin")
	if f, err := os.Create(destPath + types.IncompleteSuffix); err == nil {
		_ = f.Close()
	}

	state := progress.New("tail-guard-403", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		Workers:                   2,
		MinChunkSize:              chunkSize,
		MaxTaskRetries:            3,
		DialHedgeCount:            0,
	}

	downloader := NewConcurrentDownloader("tail-guard-403", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := downloader.Download(ctx, server.URL, nil, nil, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if worker0Preempted.Load() {
		t.Fatal("healthy worker was preempted and split by backed-off worker during early phase")
	}

	if !healthyCompleted.Load() {
		t.Fatal("healthy worker did not complete full initial range")
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Fatalf("file size verification failed: %v", err)
	}
}

func TestConcurrentDownloader_TailSplittingApproachingEOF(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	const minChunk = int64(64 * utils.KiB)
	const fileSize = 2 * minChunk
	payload := make([]byte, fileSize)
	for i := range payload {
		payload[i] = byte(i % 199)
	}

	var splitOccurred atomic.Bool

	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")

		var start, end int64
		if _, err := parseByteRange(rangeHeader, &start, &end); err == nil {
			length := end - start + 1
			// Check if sub-range split occurred near EOF
			if length < minChunk {
				splitOccurred.Store(true)
			}
			w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[start : end+1])
			return
		}

		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	destPath := filepath.Join(tmpDir, "tail_guard_eof.bin")
	if f, err := os.Create(destPath + types.IncompleteSuffix); err == nil {
		_ = f.Close()
	}

	state := progress.New("tail-guard-eof", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload:   2,
		Workers:                     2,
		MinChunkSize:                minChunk,
		AdaptiveConcurrencyInterval: time.Second,
		DialHedgeCount:              0,
	}

	downloader := NewConcurrentDownloader("tail-guard-eof", nil, state, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := downloader.Download(ctx, server.URL, nil, nil, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Fatalf("file size verification failed: %v", err)
	}
}

func parseByteRange(header string, start, end *int64) (bool, error) {
	if len(header) < 6 || header[:6] != "bytes=" {
		return false, nil
	}
	rangeStr := header[6:]
	sep := -1
	for i, c := range rangeStr {
		if c == '-' {
			sep = i
			break
		}
	}
	if sep < 0 {
		return false, nil
	}
	s, err := strconv.ParseInt(rangeStr[:sep], 10, 64)
	if err != nil {
		return false, err
	}
	e, err := strconv.ParseInt(rangeStr[sep+1:], 10, 64)
	if err != nil {
		return false, err
	}
	*start = s
	*end = e
	return true, nil
}
