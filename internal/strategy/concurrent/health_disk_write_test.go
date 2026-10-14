package concurrent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestHealth_DiskWriteDoesNotCountAsNetworkStall(t *testing.T) {
	const stallTimeout = 20 * time.Millisecond
	data := []byte("downloaded data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(data)-1, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data)
	}))
	defer server.Close()

	file, err := os.Create(filepath.Join(t.TempDir(), "download"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close download file: %v", err)
		}
	}()

	runtime := &types.RuntimeConfig{StallTimeout: stallTimeout}
	d := NewConcurrentDownloader("disk-write-stall", nil, progress.New("disk-write-stall", int64(len(data))), runtime)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	active := &ActiveTask{
		Task:      types.Task{Offset: 0, Length: int64(len(data))},
		StartTime: time.Now().Add(-time.Second),
		Cancel:    cancel,
	}
	active.StopAt.Store(int64(len(data)))
	active.LastActivity.Store(time.Now().UnixNano())
	d.activeTasks[0] = active

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	writeDone := make(chan error, 1)
	taskFinished := false
	originalWriteAt := writeAtFn
	writeAtFn = func(f *os.File, b []byte, off int64) (int, error) {
		close(writeStarted)
		<-releaseWrite
		return f.WriteAt(b, off)
	}
	defer func() {
		close(releaseWrite)
		if !taskFinished {
			select {
			case err := <-writeDone:
				if err != nil {
					t.Errorf("downloadTask: %v", err)
				}
				if active.WritingToDisk.Load() {
					t.Error("worker still marked as writing after disk write completed")
				}
			case <-time.After(2 * time.Second):
				t.Error("downloadTask did not finish after disk write was released")
			}
		}
		writeAtFn = originalWriteAt
	}()

	go func() {
		writeDone <- d.downloadTask(ctx, server.URL, file, active, make([]byte, len(data)), server.Client(), int64(len(data)))
	}()
	select {
	case <-writeStarted:
	case err := <-writeDone:
		taskFinished = true
		t.Fatalf("downloadTask exited before disk write: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("downloadTask did not reach disk write")
	}

	// Model a long disk wait without making the test depend on wall-clock timing.
	active.LastActivity.Store(time.Now().Add(-2 * stallTimeout).UnixNano())
	d.checkWorkerHealth()
	if ctx.Err() != nil {
		t.Error("health monitor cancelled a worker blocked on disk write, despite a completed network read")
	}
}
