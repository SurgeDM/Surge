package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestEnqueue_RejectedAdmissionDoesNotPersistOrPublish(t *testing.T) {
	dir := testutil.SetupStateDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1")
	}))
	defer server.Close()
	pool := scheduler.New(make(chan types.DownloadEvent, 16), 1)
	pool.GracefulShutdown()
	bus := NewEventBus()
	events, _ := bus.Subscribe()
	mgr := NewLifecycleManager(pool, bus, nil)
	defer mgr.Shutdown()
	const id = "rejected-admission"
	_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL: server.URL, Path: dir, Filename: "rejected.bin",
		Headers: map[string]string{"Authorization": "test"},
	}, id)
	if !errors.Is(err, types.ErrIDExists) {
		t.Fatalf("error=%v, want ErrIDExists", err)
	}
	master, err := store.LoadMasterList()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range master.Downloads {
		if record.ID == id {
			t.Fatal("rejected task left persisted record")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "rejected.bin") + types.IncompleteSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected working file remains: %v", err)
	}
	mgr.Shutdown() // Drain publications before inspecting the subscriber.
	for event := range events {
		if event.Type == types.EventQueued && event.DownloadID == id {
			t.Fatal("rejected task published EventQueued")
		}
	}
}
