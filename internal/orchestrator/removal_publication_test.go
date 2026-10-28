package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestRemovalPersistsWithUnreadObserver(t *testing.T) {
	root := setupEventStateDB(t)
	bus := NewEventBus()
	_, stopObserver := bus.Subscribe()
	defer stopObserver()
	core, _ := bus.SubscribeLifecycle()
	mgr := NewLifecycleManager(nil, bus, nil)
	defer mgr.Shutdown()
	done := make(chan struct{})
	go func() { defer close(done); mgr.StartEventWorker(core) }()
	path := filepath.Join(root, "removed.bin")
	entry := types.DownloadRecord{ID: "remove-id", URL: "https://example.com/file", DestPath: path, Filename: "removed.bin", Status: "paused"}
	if err := store.AddToMasterList(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveStateWithOptions(entry.URL, path, &entry, store.SaveStateOptions{SkipFileHash: true}); err != nil {
		t.Fatal(err)
	}
	for range eventQueueCapacity * 2 {
		if err := bus.Publish(types.DownloadEvent{Type: types.EventSystem, Message: "unread observer"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := mgr.Cancel(entry.ID); err != nil {
		t.Fatal(err)
	}
	mgr.Shutdown()
	<-done
	if got, err := store.GetDownload(entry.ID); err != nil || got != nil {
		t.Fatalf("deleted record survived: %+v %v", got, err)
	}
	if saved, err := store.LoadState(entry.URL, path); !errors.Is(err, os.ErrNotExist) || saved != nil {
		t.Fatalf("resume metadata survived: %+v %v", saved, err)
	}
	if states, err := store.LoadStates([]string{entry.ID}); err != nil || states[entry.ID] != nil {
		t.Fatalf("orphaned resume detail survived: %+v %v", states, err)
	}
	if _, err := os.Stat(path + types.IncompleteSuffix); !os.IsNotExist(err) {
		t.Fatalf("partial file survived: %v", err)
	}
}

func TestRemovalWaitsForQueuedPublicationWithoutBlockingOtherDownloads(t *testing.T) {
	root := setupEventStateDB(t)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "3")
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("abc"))
		}
	}))
	defer fixture.Close()
	bus := NewEventBus()
	pool := scheduler.New(bus.InputCh, 1)
	mgr := NewLifecycleManager(pool, bus, nil)
	core, _ := bus.SubscribeLifecycle()
	done := make(chan struct{})
	go func() { defer close(done); mgr.StartEventWorker(core) }()
	observer, stopObserver := bus.Subscribe()
	defer stopObserver()
	admitted, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releasePublication := func() { releaseOnce.Do(func() { close(release) }) }
	enqueued, removed, other := make(chan error, 1), make(chan error, 1), make(chan error, 1)
	removedStarted, otherStarted := false, false
	defer func() {
		releasePublication()
		if err := <-enqueued; err != nil {
			t.Errorf("enqueue during cleanup: %v", err)
		}
		if removedStarted {
			if err := <-removed; err != nil {
				t.Errorf("removal during cleanup: %v", err)
			}
		}
		if otherStarted {
			if err := <-other; err != nil {
				t.Errorf("unrelated removal during cleanup: %v", err)
			}
		}
		mgr.Shutdown()
		<-done
	}()
	mgr.beforeQueuedPublish = func() { close(admitted); <-release }
	const id = "publication-race"
	go func() {
		defer close(enqueued)
		_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{URL: fixture.URL, Path: root, Filename: "race.bin"}, id)
		enqueued <- err
	}()
	select {
	case <-admitted:
	case <-time.After(3 * time.Second):
		t.Fatal("download was not admitted")
	}
	removedStarted = true
	go func() { defer close(removed); removed <- mgr.Cancel(id) }()

	// The delayed announcement for this ID must not block unrelated operations
	// or reads of the scheduler while its same-ID removal waits.
	otherStarted = true
	go func() { defer close(other); pool.GetStatus(id); other <- mgr.Cancel("unrelated-id") }()
	select {
	case err := <-other:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("publication blocked other downloads")
	}
	select {
	case err := <-removed:
		t.Fatalf("removal overtook queued publication: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	releasePublication()
	select {
	case err := <-enqueued:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("enqueue did not finish")
	}
	select {
	case err := <-removed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("removal did not finish")
	}
	queuedSeen := false
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
observe:
	for {
		select {
		case event, ok := <-observer:
			if !ok {
				t.Fatal("observer closed before removal")
			}
			if event.DownloadID != id {
				continue
			}
			if event.Type == types.EventQueued {
				queuedSeen = true
			}
			if event.Type == types.EventRemoved {
				if !queuedSeen {
					t.Fatal("observer received removal before queued notification")
				}
				break observe
			}
		case <-deadline.C:
			t.Fatal("removal event was not delivered")
		}
	}
	mgr.Shutdown()
	<-done
	if entry, err := store.GetDownload(id); err != nil || entry != nil {
		t.Fatalf("late queue event resurrected deleted record: %+v %v", entry, err)
	}
	mgr.downloadLocksMu.Lock()
	remaining := len(mgr.downloadLocks)
	mgr.downloadLocksMu.Unlock()
	if remaining != 0 {
		t.Fatalf("retained locks for %d finished operations", remaining)
	}
}

func TestEnqueuePublicationBackpressureDoesNotHoldSchedulerLock(t *testing.T) {
	root := setupEventStateDB(t)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1")
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("x"))
		}
	}))
	defer fixture.Close()
	ctx, cancel := context.WithCancel(context.Background())
	// With no dispatcher, every publication blocks at ingress, as it would
	// when persistence fills its queues. Scheduler reads must still work.
	bus := &EventBus{InputCh: make(chan types.DownloadEvent), ctx: ctx, cancel: cancel, listeners: make(map[*eventSubscription]struct{})}
	pool := scheduler.New(bus.InputCh, 1)
	mgr := NewLifecycleManager(pool, bus, nil)
	admitted, enqueued := make(chan struct{}), make(chan error, 1)
	mgr.beforeQueuedPublish = func() { close(admitted) }
	var drainOnce sync.Once
	drained := make(chan struct{})
	unblock := func() {
		drainOnce.Do(func() {
			go func() {
				defer close(drained)
				for range bus.InputCh {
				}
			}()
		})
		cancel()
	}
	defer func() {
		unblock()
		if err := <-enqueued; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("enqueue during cleanup: %v", err)
		}
		mgr.Shutdown()
		<-drained
	}()
	go func() {
		defer close(enqueued)
		_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{URL: fixture.URL, Path: root, Filename: "blocked.bin"}, "blocked-id")
		enqueued <- err
	}()
	select {
	case <-admitted:
	case <-time.After(3 * time.Second):
		t.Fatal("enqueue did not reach publication")
	}
	read := make(chan struct{})
	go func() { pool.GetStatus("blocked-id"); close(read) }()
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Error("blocked event publisher retained the scheduler lock")
	}
	unblock()
	select {
	case err := <-enqueued:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("enqueue did not unblock")
	}
	select {
	case <-read:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler read did not unblock")
	}
}
