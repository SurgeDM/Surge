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
		for range 500 {
			if err := bus.Publish(types.DownloadEvent{Type: types.EventSystem, Message: "unread observer"}); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		resync := false
		for range 16 {
			msg, ok := <-stream
			if !ok {
				t.Fatal("observer did not recover")
			}
			if msg.ResyncRequired {
				// A burst can overflow more than one subscription before the first
				// recovery notice is read. Drain those notices before asserting delivery
				// on the stable subscription, after ingress is quiescent.
				synctest.Wait()
				if bus.HasObservers() {
					resync = true
					break
				}
			}
		}
		if !resync {
			t.Fatal("missing explicit resync requirement")
		}
		if err := bus.Publish(types.DownloadEvent{Type: types.EventQueued, DownloadID: "after-resync"}); err != nil {
			t.Fatal(err)
		}
		// A notice for the stable subscription can already be in flight behind
		// the notice we just read. That is valid; the fresh event must follow it.
		for range 16 {
			select {
			case msg, ok := <-stream:
				if !ok {
					t.Fatal("recovered stream closed")
				}
				if msg.DownloadID == "after-resync" {
					return
				}
				if !msg.ResyncRequired {
					t.Fatalf("unexpected event after recovery: %+v", msg)
				}
			case <-time.After(time.Second):
				t.Fatal("fresh subscription lost its event")
			}
		}
		t.Fatal("recovery notices did not converge")

	})
}
