package orchestrator

import (
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestLatePauseCannotRegressCompletedRecord(t *testing.T) {
	setupEventStateDB(t)
	entry := types.DownloadRecord{ID: "finished", Status: "completed", TotalSize: 123, Downloaded: 123, CompletedAt: 42}
	if err := store.AddToMasterList(entry); err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range []*types.DownloadRecord{nil, {Downloaded: 10}} {
		dispatchLifecycleEvent(t, types.DownloadEvent{Type: types.EventPaused, DownloadID: entry.ID, Downloaded: 10, State: snapshot})
		got, err := store.GetDownload(entry.ID)
		if err != nil || got.Status != "completed" || got.Downloaded != 123 || got.CompletedAt != 42 {
			t.Fatalf("completion regressed: %+v, %v", got, err)
		}
	}
}

func TestRepeatedPauseOfStoppedRecordsDoesNotPublish(t *testing.T) {
	setupEventStateDB(t)
	bus := NewEventBus()
	pool := scheduler.New(bus.InputCh, 1)
	mgr := NewLifecycleManager(pool, bus, config.DefaultSettings())
	t.Cleanup(mgr.Shutdown)
	ch, cleanup := bus.Subscribe()
	t.Cleanup(cleanup)
	for _, status := range []string{"completed", "paused", "error"} {
		if err := store.AddToMasterList(types.DownloadRecord{ID: status, Status: status}); err != nil {
			t.Fatal(err)
		}
		if err := mgr.Pause(status); err != nil {
			t.Fatal(err)
		}
		if err := mgr.Pause(status); err != nil {
			t.Fatal(err)
		}
	}
	if err := bus.Publish(types.DownloadEvent{Type: types.EventSystem, Message: "barrier"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-ch:
			if event.Type == types.EventPaused {
				t.Fatalf("published a spurious pause for %s", event.DownloadID)
			}
			if event.Message == "barrier" {
				return
			}
		case <-deadline.C:
			t.Fatal("event barrier was not delivered")
		}
	}
}
