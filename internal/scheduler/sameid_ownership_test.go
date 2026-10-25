package scheduler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
)

// newTestSchedulerWithoutWorkers returns a Scheduler with a pre-populated
// downloads map and no worker goroutines, so tests can drive Add and queue
// state deterministically.
func newTestSchedulerWithoutWorkers(configs map[string]types.DownloadRecord) *Scheduler {
	downloads := make(map[string]*activeDownload, len(configs))
	for id, cfg := range configs {
		downloads[id] = &activeDownload{config: cfg}
	}
	p := &Scheduler{
		downloads:        downloads,
		queued:           make(map[string]*queuedTask),
		downloadLimiters: make(map[string]*transport.RateLimiter),
		progressDone:     make(chan struct{}),
		progressCh:       make(chan types.DownloadEvent, 8),
	}
	p.taskCond = sync.NewCond(&p.mu)
	return p
}

// awaitShutdown asserts GracefulShutdown returns; a hang means a wait-group
// count leaked or was double-settled.
func awaitShutdown(t *testing.T, pool *Scheduler) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		pool.GracefulShutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("GracefulShutdown hung; a wait-group count leaked")
	}
}

// A duplicate Add for an ID that is already queued is refused outright: no
// map writes, no queueOrder append, no limiter churn, no wait-group count.
func TestScheduler_AddDuplicateQueuedRefused(t *testing.T) {
	pool := newTestSchedulerWithoutWorkers(nil)
	t.Cleanup(pool.GracefulShutdown)

	const id = "dup-queued"
	cfg1 := types.DownloadRecord{
		ID:       id,
		URL:      "http://example.com/first.bin",
		Filename: "first.bin",
	}
	cfg2 := types.DownloadRecord{
		ID:       id,
		URL:      "http://example.com/second.bin",
		Filename: "second.bin",
	}

	if !pool.Add(cfg1) {
		t.Fatal("first Add was refused")
	}

	pool.mu.RLock()
	limiterBefore := pool.downloadLimiters[id]
	pool.mu.RUnlock()

	if pool.Add(cfg2) {
		t.Fatal("duplicate Add was accepted")
	}

	pool.mu.RLock()
	qt := pool.queued[id]
	orderLen := len(pool.queueOrder)
	limiterAfter := pool.downloadLimiters[id]
	pool.mu.RUnlock()

	if qt == nil || qt.cfg.URL != cfg1.URL || qt.cfg.Filename != cfg1.Filename {
		t.Fatal("queued entry was replaced by the refused Add")
	}
	if orderLen != 1 {
		t.Fatalf("queueOrder len = %d, want 1", orderLen)
	}
	if limiterAfter != limiterBefore {
		t.Fatal("refused Add touched the per-download limiter")
	}
	if all := pool.GetAll(); len(all) != 1 {
		t.Fatalf("GetAll len = %d, want 1", len(all))
	}

	awaitShutdown(t, pool)
}

// A duplicate Add for an ID that already owns an active download is refused
// and leaves the registered entry untouched.
func TestScheduler_AddDuplicateActiveRefused(t *testing.T) {
	const id = "dup-active"
	pool := newTestSchedulerWithoutWorkers(map[string]types.DownloadRecord{
		id: {
			ID:       id,
			URL:      "http://example.com/active.bin",
			Filename: "active.bin",
		},
	})
	t.Cleanup(pool.GracefulShutdown)

	pool.mu.RLock()
	before := pool.downloads[id]
	pool.mu.RUnlock()

	if pool.Add(types.DownloadRecord{
		ID:       id,
		URL:      "http://example.com/impostor.bin",
		Filename: "impostor.bin",
	}) {
		t.Fatal("Add over an active ID was accepted")
	}

	pool.mu.RLock()
	after := pool.downloads[id]
	_, queued := pool.queued[id]
	orderLen := len(pool.queueOrder)
	pool.mu.RUnlock()

	if after != before {
		t.Fatal("active entry was replaced by the refused Add")
	}
	if queued || orderLen != 0 {
		t.Fatalf("refused Add left queue state: queued=%v orderLen=%d", queued, orderLen)
	}
}

// Add after shutdown is refused — a post-sweep task could never be settled
// and would leak its wait-group count.
func TestScheduler_AddAfterShutdownRefused(t *testing.T) {
	ch := make(chan types.DownloadEvent, 8)
	pool := New(ch, 1)
	pool.GracefulShutdown()

	if pool.Add(types.DownloadRecord{ID: "late", URL: "http://example.com/late.bin"}) {
		t.Fatal("Add after GracefulShutdown was accepted")
	}

	pool.mu.RLock()
	_, queued := pool.queued["late"]
	orderLen := len(pool.queueOrder)
	pool.mu.RUnlock()
	if queued || orderLen != 0 {
		t.Fatalf("post-shutdown Add left queue state: queued=%v orderLen=%d", queued, orderLen)
	}
}

// The requeue write must not clobber a same-ID queued replacement: a worker
// that returns from a failed run and finds the ID re-owned by a queued task
// settles its own wait group silently instead of overwriting it.
func TestScheduler_RequeueKeepsSameIDReplacement(t *testing.T) {
	ch := make(chan types.DownloadEvent, 16)
	pool := New(ch, 1)

	oldGate := pool.workerDoneGate.Load()
	t.Cleanup(func() { pool.workerDoneGate.Store(oldGate) })
	t.Cleanup(pool.GracefulShutdown)

	// Connection refused is a fast transport error with no HTTP status, so it
	// fails RunDownload immediately while staying retryable at the scheduler
	// level — 5xx statuses would be retried inside the strategy first and the
	// worker would not reach the done gate within the timeout.
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverAURL := serverA.URL
	serverA.Close()
	serverB := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1")
		_, _ = w.Write([]byte("x"))
	}))
	defer serverB.Close()

	const id = "requeue-keep"
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "b.bin")+types.IncompleteSuffix, nil, 0o644); err != nil {
		t.Fatalf("failed to pre-create incomplete file: %v", err)
	}

	// Pin the worker between RunDownload return and terminal bookkeeping.
	// Inside the window: Cancel drops the finished entry and Add queues a
	// same-ID replacement (maxDownloads=1, so nothing else can claim it) that
	// the stale requeue must not overwrite.
	doneGateRan := make(chan struct{})
	gateFn := func() {
		pool.workerDoneGate.Store(nil)
		pool.Cancel(id)
		if !pool.Add(types.DownloadRecord{
			ID:            id,
			URL:           serverB.URL,
			OutputPath:    tmpDir,
			Filename:      "b.bin",
			ProgressState: progress.New(id, 0),
			Runtime:       &types.RuntimeConfig{},
		}) {
			t.Error("replacement Add was refused")
		}
		close(doneGateRan)
	}
	pool.workerDoneGate.Store(&gateFn)

	if !pool.Add(types.DownloadRecord{
		ID:            id,
		URL:           serverAURL,
		OutputPath:    tmpDir,
		Filename:      "a.bin",
		ProgressState: progress.New(id, 0),
		Runtime:       &types.RuntimeConfig{},
	}) {
		t.Fatal("initial Add was refused")
	}

	select {
	case <-doneGateRan:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never reached the post-download window")
	}

	// The stale worker refuses the requeue, settles its own wait group, and
	// then claims the replacement itself — serverB must start, with no stale
	// EventQueued/EventError from the dead task on the shared channel. qt1's
	// own EventStarted (serverA) is expected and skipped.
	startDeadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.DownloadID != id {
				continue
			}
			switch ev.Type {
			case types.EventStarted:
				if ev.URL == serverB.URL {
					goto started
				}
			case types.EventQueued, types.EventError:
				t.Fatalf("stale task emitted %+v into the replacement's stream", ev)
			}
		case <-startDeadline:
			t.Fatal("replacement task never started")
		}
	}
started:

	awaitShutdown(t, pool)
}

// Terminal bookkeeping must not delete a same-ID active download that
// registered after this worker's claim; map deletes are pointer-gated. A
// follow-up task claimed by the released worker is the deterministic barrier
// that its terminal section has already run while the replacement is parked.
func TestScheduler_TerminalDeleteKeepsRegisteredReplacement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"success", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1")
			_, _ = w.Write([]byte("x"))
		}},
		{"permanent-error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan types.DownloadEvent, 16)
			pool := New(ch, 2)

			oldGate := pool.workerDoneGate.Load()
			t.Cleanup(func() { pool.workerDoneGate.Store(oldGate) })
			t.Cleanup(pool.GracefulShutdown)

			serverA := testutil.NewHTTPServerT(t, tc.handler)
			defer serverA.Close()
			serverB := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "1")
				_, _ = w.Write([]byte("x"))
			}))
			defer serverB.Close()

			const id = "terminal-aba"
			tmpDir := t.TempDir()
			for _, name := range []string{"a.bin", "b.bin", "c.bin"} {
				if err := os.WriteFile(filepath.Join(tmpDir, name)+types.IncompleteSuffix, nil, 0o644); err != nil {
					t.Fatalf("failed to pre-create incomplete file: %v", err)
				}
			}

			// Unbuffered: qt2's EventStarted send completing proves ad2 is
			// registered, and worker2 stays inside RunDownload — ad2 alive —
			// until the test drains the follow-up sends.
			gateCh2 := make(chan types.DownloadEvent)
			doneGateHit := make(chan struct{})
			release := make(chan struct{})
			gateFn := func() {
				pool.workerDoneGate.Store(nil)
				close(doneGateHit)
				<-release
			}
			pool.workerDoneGate.Store(&gateFn)
			var releaseOnce sync.Once
			releaseWorker := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseWorker()

			if !pool.Add(types.DownloadRecord{
				ID:            id,
				URL:           serverA.URL,
				OutputPath:    tmpDir,
				Filename:      "a.bin",
				ProgressState: progress.New(id, 0),
				Runtime:       &types.RuntimeConfig{},
			}) {
				t.Fatal("initial Add was refused")
			}

			select {
			case <-doneGateHit:
			case <-time.After(5 * time.Second):
				t.Fatal("worker never reached the post-download window")
			}

			// Worker1 is parked post-RunDownload. Drop the stale entry and
			// queue the same-ID replacement; the idle second worker claims it.
			pool.Cancel(id)
			if !pool.Add(types.DownloadRecord{
				ID:            id,
				URL:           serverB.URL,
				OutputPath:    tmpDir,
				Filename:      "b.bin",
				ProgressState: progress.New(id, 0),
				ProgressCh:    gateCh2,
				Runtime:       &types.RuntimeConfig{},
			}) {
				t.Fatal("replacement Add was refused")
			}

			select {
			case ev := <-gateCh2:
				if ev.Type != types.EventStarted || ev.DownloadID != id || ev.URL != serverB.URL {
					t.Fatalf("first replacement send = %+v, want EventStarted for %s at %s", ev, id, serverB.URL)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("replacement worker never sent EventStarted")
			}
			// ad2 is registered now; worker2 is (or is about to be) parked on
			// the next gateCh2 send, so ad2 stays in the map.

			// Queue a follow-up the parked worker1 will claim after its
			// terminal section — its EventStarted is the barrier proving the
			// terminal path ran while ad2 was registered.
			const id3 = "terminal-aba-barrier"
			if !pool.Add(types.DownloadRecord{
				ID:            id3,
				URL:           serverB.URL,
				OutputPath:    tmpDir,
				Filename:      "c.bin",
				ProgressState: progress.New(id3, 0),
				Runtime:       &types.RuntimeConfig{},
			}) {
				t.Fatal("barrier Add was refused")
			}
			releaseWorker()

			barrierDeadline := time.After(10 * time.Second)
			for {
				select {
				case ev := <-ch:
					if ev.Type == types.EventStarted && ev.DownloadID == id3 {
						goto barrier
					}
				case <-barrierDeadline:
					t.Fatal("released worker never claimed the barrier task")
				}
			}
		barrier:

			pool.mu.RLock()
			survivor := pool.downloads[id]
			pool.mu.RUnlock()
			if survivor == nil {
				t.Fatal("stale terminal cleanup deleted the same-ID replacement")
			}
			if survivor.config.URL != serverB.URL {
				t.Fatalf("surviving entry URL = %q, want replacement %q", survivor.config.URL, serverB.URL)
			}

			// Drain qt2's channel so worker2's sends never park, until its
			// RunDownload returns and ad.done closes.
			for {
				select {
				case <-gateCh2:
				case <-survivor.done:
					goto drained
				}
			}
		drained:

			awaitShutdown(t, pool)
		})
	}
}
