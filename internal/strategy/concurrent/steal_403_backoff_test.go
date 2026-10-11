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

func TestStealWork_BackedOffWorkerNotStolenFrom(t *testing.T) {
	const (
		workerID  = 1
		current   = 0
		remaining = 16 * types.AlignSize
		minChunk  = 4 * types.AlignSize
	)
	originalEnd := int64(current + remaining)

	runtime := &types.RuntimeConfig{
		MinChunkSize:                minChunk,
		AdaptiveConcurrencyInterval: time.Second,
	}
	downloader := NewConcurrentDownloader("steal-backoff", nil, nil, runtime)
	active := &ActiveTask{Task: types.Task{Offset: 0, Length: originalEnd}}
	active.CurrentOffset.Store(current)
	active.StopAt.Store(originalEnd)
	downloader.activeTasks[workerID] = active

	// Set worker in 403 backoff state with a future deadline.
	backoffDeadline := time.Now().Add(500 * time.Millisecond)
	downloader.SetWorkerBackoff(workerID, backoffDeadline)
	active.SetBackoff(backoffDeadline)

	queue := NewTaskQueue()

	// StealWork must not steal from the backed-off worker.
	if downloader.StealWork(queue) {
		t.Fatal("StealWork succeeded on a worker in backoff state")
	}
	if queue.Len() != 0 {
		t.Fatalf("queue length = %d, want 0", queue.Len())
	}
	if got := active.StopAt.Load(); got != originalEnd {
		t.Fatalf("active StopAt = %d, want %d", got, originalEnd)
	}

	// Clear backoff to simulate duration elapsed.
	downloader.ClearWorkerBackoff(workerID)
	active.ClearBackoff()

	// StealWork should now succeed.
	if !downloader.StealWork(queue) {
		t.Fatal("StealWork failed after backoff duration elapsed")
	}
	if queue.Len() != 1 {
		t.Fatalf("queue length = %d, want 1", queue.Len())
	}
	stolen := queue.DrainRemaining()[0]
	newStopAt := active.StopAt.Load()
	if stolen.Offset != newStopAt {
		t.Fatalf("stolen offset %d does not match new stop %d", stolen.Offset, newStopAt)
	}
	if newStopAt+stolen.Length != originalEnd {
		t.Fatalf("split ranges do not cover original end: stop=%d len=%d orig=%d", newStopAt, stolen.Length, originalEnd)
	}
}

func TestStealWork_MultipleWorkers_PrefersHealthyOverBackedOff(t *testing.T) {
	const minChunk = 4 * types.AlignSize
	runtime := &types.RuntimeConfig{
		MinChunkSize:                minChunk,
		AdaptiveConcurrencyInterval: time.Second,
	}
	downloader := NewConcurrentDownloader("steal-multi", nil, nil, runtime)

	// Worker 1 has more remaining bytes (32 blocks), but is backing off due to 403.
	active1 := &ActiveTask{Task: types.Task{Offset: 0, Length: 32 * types.AlignSize}}
	active1.CurrentOffset.Store(0)
	active1.StopAt.Store(32 * types.AlignSize)
	deadline := time.Now().Add(time.Hour)
	downloader.SetWorkerBackoff(1, deadline)
	active1.SetBackoff(deadline)
	downloader.activeTasks[1] = active1

	// Worker 2 has fewer remaining bytes (16 blocks), but is healthy.
	active2 := &ActiveTask{Task: types.Task{Offset: 32 * types.AlignSize, Length: 16 * types.AlignSize}}
	active2.CurrentOffset.Store(32 * types.AlignSize)
	active2.StopAt.Store(48 * types.AlignSize)
	downloader.activeTasks[2] = active2

	queue := NewTaskQueue()

	// StealWork should steal from worker 2, bypassing worker 1.
	if !downloader.StealWork(queue) {
		t.Fatal("StealWork failed when healthy worker was available")
	}

	if got := active1.StopAt.Load(); got != 32*types.AlignSize {
		t.Fatalf("backed-off worker 1 StopAt was modified: %d", got)
	}

	if got := active2.StopAt.Load(); got >= 48*types.AlignSize {
		t.Fatalf("healthy worker 2 StopAt was not reduced: %d", got)
	}

	tasks := queue.DrainRemaining()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 stolen task, got %d", len(tasks))
	}
	if tasks[0].Offset != active2.StopAt.Load() {
		t.Fatalf("stolen task offset %d does not match worker 2 stop %d", tasks[0].Offset, active2.StopAt.Load())
	}
}

func TestStealWork_WaitingOnLimiterWorkerNotStolenFrom(t *testing.T) {
	const (
		workerID  = 1
		remaining = 16 * types.AlignSize
		minChunk  = 4 * types.AlignSize
	)
	runtime := &types.RuntimeConfig{
		MinChunkSize:                minChunk,
		AdaptiveConcurrencyInterval: time.Second,
	}
	downloader := NewConcurrentDownloader("steal-limiter", nil, nil, runtime)
	active := &ActiveTask{Task: types.Task{Offset: 0, Length: remaining}}
	active.CurrentOffset.Store(0)
	active.StopAt.Store(remaining)
	active.WaitingOnLimiter.Store(true)
	downloader.activeTasks[workerID] = active

	queue := NewTaskQueue()
	if downloader.StealWork(queue) {
		t.Fatal("StealWork succeeded on a worker with WaitingOnLimiter=true")
	}

	active.WaitingOnLimiter.Store(false)
	if !downloader.StealWork(queue) {
		t.Fatal("StealWork failed after WaitingOnLimiter cleared")
	}
}

func TestStealWork_HostCooldownSuppressesStealing(t *testing.T) {
	const (
		workerID  = 1
		remaining = 16 * types.AlignSize
		minChunk  = 4 * types.AlignSize
	)
	runtime := &types.RuntimeConfig{
		MinChunkSize:                minChunk,
		AdaptiveConcurrencyInterval: time.Second,
	}
	downloader := NewConcurrentDownloader("steal-cooldown", nil, nil, runtime)
	downloader.hostLimiter = transport.NewHostRateLimiter()
	downloader.URL = "http://example.com/file"
	downloader.workerHosts = []string{"example.com"}

	active := &ActiveTask{Task: types.Task{Offset: 0, Length: remaining}}
	active.CurrentOffset.Store(0)
	active.StopAt.Store(remaining)
	downloader.activeTasks[workerID] = active

	// Simulate 403 cooldown on host.
	now := time.Now()
	downloader.hostLimiter.ReportThrottle("example.com", 2, 2*time.Second, true, now)

	queue := NewTaskQueue()
	if downloader.StealWork(queue) {
		t.Fatal("StealWork succeeded while host cooldown was active")
	}

	// Reset host limiter to simulate cooldown expired.
	downloader.hostLimiter = transport.NewHostRateLimiter()
	if !downloader.StealWork(queue) {
		t.Fatal("StealWork failed after host cooldown cleared")
	}
}

func TestStealWork_TailGuardPreventsEarlyAIMDStealing(t *testing.T) {
	const minChunk = 4 * types.AlignSize
	runtime := &types.RuntimeConfig{
		MinChunkSize:                minChunk,
		AdaptiveConcurrencyInterval: time.Second,
	}
	downloader := NewConcurrentDownloader("steal-tail-guard", nil, nil, runtime)

	// Worker 1 has 8 blocks (> minChunk)
	active1 := &ActiveTask{Task: types.Task{Offset: 0, Length: 8 * types.AlignSize}}
	active1.CurrentOffset.Store(0)
	active1.StopAt.Store(8 * types.AlignSize)
	downloader.activeTasks[1] = active1

	// Worker 2 has 2 blocks (< minChunk)
	active2 := &ActiveTask{Task: types.Task{Offset: 8 * types.AlignSize, Length: 2 * types.AlignSize}}
	active2.CurrentOffset.Store(8 * types.AlignSize)
	active2.StopAt.Store(10 * types.AlignSize)
	downloader.activeTasks[2] = active2

	// activeWorkers = 2. Total remaining = 10 * types.AlignSize.
	// int64(activeWorkers)*minChunk = 2 * 4 = 8 * types.AlignSize.
	// totalRemaining (10) >= 8, so early AIMD stealing below minChunk is suppressed.
	// StealWork should steal from worker 1 (remaining 8 > 4), NOT worker 2 (remaining 2 <= 4).
	queue := NewTaskQueue()
	if !downloader.StealWork(queue) {
		t.Fatal("StealWork failed on worker with remaining > minChunk")
	}
	if got := active2.StopAt.Load(); got != 10*types.AlignSize {
		t.Fatalf("worker 2 with small chunk was stolen from: %d", got)
	}
}

func TestStealWork_LargeRemainingTaskPeersIdleStealsSuccessfully(t *testing.T) {
	const (
		remaining = 32 * types.AlignSize
		minChunk  = 4 * types.AlignSize
	)
	runtime := &types.RuntimeConfig{
		MinChunkSize: minChunk,
	}
	downloader := NewConcurrentDownloader("steal-healthy-large", nil, nil, runtime)
	active := &ActiveTask{Task: types.Task{Offset: 0, Length: remaining}}
	active.CurrentOffset.Store(0)
	active.StopAt.Store(remaining)
	downloader.activeTasks[0] = active

	queue := NewTaskQueue()
	if !downloader.StealWork(queue) {
		t.Fatal("StealWork failed on healthy large remaining task with idle peers")
	}
	if queue.Len() != 1 {
		t.Fatalf("queue length = %d, want 1", queue.Len())
	}
}

func TestStealWork_NilOrNonEmptyQueueReturnsFalse(t *testing.T) {
	downloader := NewConcurrentDownloader("steal-queue-checks", nil, nil, nil)
	active := &ActiveTask{Task: types.Task{Offset: 0, Length: 16 * types.AlignSize}}
	active.CurrentOffset.Store(0)
	active.StopAt.Store(16 * types.AlignSize)
	downloader.activeTasks[0] = active

	if downloader.StealWork(nil) {
		t.Fatal("StealWork succeeded with nil queue")
	}

	queue := NewTaskQueue()
	queue.Push(types.Task{Offset: 0, Length: 100})
	if downloader.StealWork(queue) {
		t.Fatal("StealWork succeeded with non-empty queue")
	}
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
