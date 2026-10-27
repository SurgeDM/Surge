package service

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestLocalObserverOverflowRequiresResyncAndResubscribes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := orchestrator.NewEventBus()
		defer bus.Shutdown()
		manager := orchestrator.NewLifecycleManager(nil, bus, nil)
		service := NewLocalDownloadService(manager)
		stream, cleanup, err := service.StreamEvents(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		for range 500 {
			if err := bus.Publish(types.DownloadEvent{Type: types.EventSystem, Message: "unread observer"}); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		resync := false
		for range 3 {
			msg, ok := <-stream
			if !ok {
				t.Fatal("observer did not recover")
			}
			if msg.ResyncRequired {
				resync = true
				break
			}
		}
		if !resync {
			t.Fatal("missing explicit resync requirement")
		}
		if err := bus.Publish(types.DownloadEvent{Type: types.EventQueued, DownloadID: "after-resync"}); err != nil {
			t.Fatal(err)
		}
		msg := <-stream
		if msg.DownloadID != "after-resync" {
			t.Fatalf("fresh subscription not receiving events: %+v", msg)
		}
	})
}
