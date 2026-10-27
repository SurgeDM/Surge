package service

import (
	"context"
	"errors"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestFinalResultChecksArtifactAfterDurableCompletion(t *testing.T) {
	testutil.SetupStateDB(t)
	path := filepath.Join(t.TempDir(), "artifact.bin")
	entry := types.DownloadRecord{ID: "artifact", Status: "downloading", DestPath: path, Downloaded: 3, TotalSize: 3}
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewLocalDownloadService(nil)
	if err := store.AddToMasterList(entry); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetResult(entry.ID)
	if err != nil || got.Status == "completed" {
		t.Fatalf("uncommitted artifact: %+v %v", got, err)
	}
	entry.Status = "completed"
	if err := store.AddToMasterList(entry); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetResult(entry.ID); !errors.Is(err, ErrArtifactUnavailable) {
		t.Fatalf("missing final artifact accepted: %v", err)
	}
	if err := os.Rename(path+types.IncompleteSuffix, path); err != nil {
		t.Fatal(err)
	}
	got, err = svc.GetResult(entry.ID)
	if err != nil || got.Status != "completed" || got.DestPath != path {
		t.Fatalf("final result: %+v %v", got, err)
	}
	if err := os.WriteFile(path, []byte("changed size"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetResult(entry.ID); !errors.Is(err, ErrArtifactUnavailable) {
		t.Fatalf("changed artifact accepted: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetResult(entry.ID); !errors.Is(err, ErrArtifactUnavailable) {
		t.Fatalf("removed artifact accepted: %v", err)
	}
}

func TestFinalResultWaitsForLifecycleFinalization(t *testing.T) {
	testutil.SetupStateDB(t)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "3")
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("abc"))
		}
	}))
	defer fixture.Close()
	bus := orchestrator.NewEventBus()
	pool := scheduler.New(bus.InputCh, 1)
	mgr := orchestrator.NewLifecycleManager(pool, bus, config.DefaultSettings())
	t.Cleanup(mgr.Shutdown)
	svc := NewLocalDownloadService(mgr)
	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	id, err := svc.Add(fixture.URL, t.TempDir(), "artifact.bin", nil, nil, false, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Intentionally delay the persistence worker until the bytes are complete.
	var events []types.DownloadEvent
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
collect:
	for {
		select {
		case event := <-stream:
			events = append(events, event)
			if event.Type == types.EventComplete && event.DownloadID == id {
				break collect
			}
		case <-deadline.C:
			t.Fatal("download did not finish")
		}
	}
	pool.GracefulShutdown()
	before, err := svc.GetResult(id)
	if err != nil || before.Status == "completed" {
		t.Fatalf("completion preceded finalization: %+v %v", before, err)
	}
	ch := make(chan types.DownloadEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	mgr.StartEventWorker(ch)
	after, err := svc.GetResult(id)
	if err != nil || after.Status != "completed" {
		t.Fatalf("finalized result: %+v %v", after, err)
	}
	data, err := os.ReadFile(after.DestPath)
	if err != nil || string(data) != "abc" {
		t.Fatalf("artifact=%q err=%v", data, err)
	}
}
