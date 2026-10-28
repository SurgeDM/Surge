package service

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

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
		const burstEvents = 500
		for range burstEvents {
			if err := bus.Publish(types.DownloadEvent{Type: types.EventSystem, Message: "unread observer"}); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		resync, drained := false, false
		// The publisher has finished. Drain both recovery notices and burst events
		// until all stream goroutines are blocked with no pending delivery. An
		// active replacement subscription alone does not imply its queue is empty.
	drain:
		for range 2*burstEvents + 1 {
			synctest.Wait()
			select {
			case msg, ok := <-stream:
				if !ok {
					t.Fatal("observer did not recover")
				}
				resync = resync || msg.ResyncRequired
			default:
				drained = true
				break drain
			}
		}
		if !resync || !drained || !bus.HasObservers() {
			t.Fatalf("recovery did not settle: resync=%v drained=%v subscribed=%v", resync, drained, bus.HasObservers())
		}
		if err := bus.Publish(types.DownloadEvent{Type: types.EventQueued, DownloadID: "after-resync"}); err != nil {
			t.Fatal(err)
		}
		// All old traffic has been drained; only the fresh event can follow.
		select {
		case msg, ok := <-stream:
			if !ok || msg.DownloadID != "after-resync" {
				t.Fatalf("fresh event was not delivered: %+v, open=%v", msg, ok)
			}
		case <-time.After(time.Second):
			t.Fatal("fresh subscription lost its event")
		}
	})
}
