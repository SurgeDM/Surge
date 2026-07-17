package concurrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestWorkerRetryOnTransientError(t *testing.T) {
	// A server that returns a reset-like error on the first two attempts, then succeeds
	var attemptCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attemptCount.Add(1)
		if attempt <= 2 {
			// Simulate a transient network error by explicitly hijacking and closing the connection
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close() // Forces a "connection reset by peer" or "EOF"
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 3rd attempt succeeds
		w.Header().Set("Content-Range", "bytes 0-4/5")
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	d := NewConcurrentDownloader("test-id", nil, nil, types.DefaultRuntimeConfig())
	d.State = progress.New("test-id", 5)

	queue := NewTaskQueue()
	queue.Push(types.Task{Offset: 0, Length: 5})

	tmpFile, err := os.CreateTemp("", "surge-test-worker-retry")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	client := server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1 task in queue, worker should try to download it
	err = d.worker(ctx, 0, []string{server.URL}, tmpFile, queue, 5, client)
	if err != nil {
		t.Fatalf("expected worker to succeed eventually, but got: %v", err)
	}

	if attempts := attemptCount.Load(); attempts < 3 {
		t.Fatalf("expected at least 3 attempts due to transient errors, got %d", attempts)
	}

	b, _ := os.ReadFile(tmpFile.Name())
	if string(b[:5]) != "hello" {
		t.Errorf("expected 'hello' in file, got %q", string(b[:5]))
	}
}

func TestSaveProgressOnError(t *testing.T) {
	progCh := make(chan types.DownloadEvent, 10)

	rt := types.DefaultRuntimeConfig()
	state := progress.New("test-id", 100)
	d := NewConcurrentDownloader("test-id", progCh, state, rt)

	queue := NewTaskQueue()
	queue.Push(types.Task{Offset: 0, Length: 50})
	queue.Push(types.Task{Offset: 50, Length: 50})

	d.saveProgressOnError("some/dest/path.txt", 100, queue, []string{"http://mirror1"})

	close(progCh)
	var pausedEvent *types.DownloadEvent
	for event := range progCh {
		if event.Type == types.EventPaused {
			// Create a local copy to point to
			e := event
			pausedEvent = &e
			break
		}
	}

	if pausedEvent == nil {
		t.Fatal("expected EventPaused to be sent by saveProgressOnError")
	}

	if pausedEvent.State == nil {
		t.Fatal("expected paused event to contain a state snapshot")
	}

	if len(pausedEvent.State.Tasks) != 2 {
		t.Errorf("expected 2 remaining tasks in saved state, got %d", len(pausedEvent.State.Tasks))
	}
}
