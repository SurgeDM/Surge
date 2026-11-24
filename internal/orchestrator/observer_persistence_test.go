package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestLifecyclePersistsCompletionWithUnreadObserver(t *testing.T) {
	root := setupEventStateDB(t)
	bus := NewEventBus()
	_, cleanup := bus.Subscribe()
	defer cleanup()
	core, stopCore := bus.SubscribeLifecycle()
	defer stopCore()
	settings := config.DefaultSettings()
	settings.General.DownloadCompleteNotification.Value = false
	manager := NewLifecycleManager(nil, bus, settings)
	done := make(chan struct{})
	go func() { defer close(done); manager.StartEventWorker(core) }()
	path := filepath.Join(root, "complete.bin")
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("verified bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	for range eventQueueCapacity * 2 {
		if err := bus.Publish(types.DownloadEvent{Type: types.EventSystem, Message: "fill slow observer"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []types.DownloadEvent{
		{Type: types.EventStarted, DownloadID: "persisted-id", URL: "https://example.com/file", Filename: "complete.bin", DestPath: path, Total: 14},
		{Type: types.EventComplete, DownloadID: "persisted-id", Filename: "complete.bin", Total: 14, Elapsed: time.Second},
	} {
		if err := bus.Publish(event); err != nil {
			t.Fatal(err)
		}
	}
	bus.Shutdown()
	<-done
	record, err := store.GetDownload("persisted-id")
	if err != nil || record == nil || record.Status != "completed" {
		t.Fatalf("completion not durable: %+v, %v", record, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "verified bytes" {
		t.Fatalf("file not finalized: %q, %v", data, err)
	}
}

func TestObserversReceiveCompletionOnlyAfterFinalization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		event  types.EventType
	}{
		{"promoted", "completed", types.EventComplete},
		{"missing working file", "error", types.EventError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := setupEventStateDB(t)
			path := filepath.Join(root, "file.bin")
			if tc.event == types.EventComplete {
				if err := os.WriteFile(path+types.IncompleteSuffix, []byte("data"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			bus := NewEventBus()
			defer bus.Shutdown()
			observer, stopObserver := bus.Subscribe()
			defer stopObserver()
			core, stopCore := bus.SubscribeLifecycle()
			defer stopCore()

			receive := func() types.DownloadEvent {
				t.Helper()
				select {
				case event := <-observer:
					return event
				case <-time.After(5 * time.Second):
					t.Fatal("timed out waiting for observer")
					return types.DownloadEvent{}
				}
			}
			for _, event := range []types.DownloadEvent{
				{Type: types.EventStarted, DownloadID: "file", Filename: "file.bin", DestPath: path, Total: 4},
				{Type: types.EventComplete, DownloadID: "file", Total: 4},
				{Type: types.EventSystem, Message: "barrier"},
			} {
				if err := bus.Publish(event); err != nil {
					t.Fatal(err)
				}
			}
			if receive().Type != types.EventStarted || receive().Message != "barrier" {
				t.Fatal("observer received transfer completion before persistence")
			}

			settings := config.DefaultSettings()
			settings.General.DownloadCompleteNotification.Value = false
			manager := NewLifecycleManager(nil, bus, settings)
			done := make(chan struct{})
			go func() { defer close(done); manager.StartEventWorker(core) }()
			defer func() { stopCore(); <-done }()

			event := receive()
			if event.Type != tc.event || event.DestPath != path {
				t.Fatalf("unexpected lifecycle result: %+v", event)
			}
			record, err := store.GetDownload("file")
			if err != nil || record == nil || record.Status != tc.status {
				t.Fatalf("unexpected persisted result: %+v, %v", record, err)
			}
		})
	}
}
