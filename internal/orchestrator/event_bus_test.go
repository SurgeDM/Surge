package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestEventBus_BasicPubSub(t *testing.T) {
	eb := NewEventBus()
	defer eb.Shutdown()

	sub, cleanup := eb.Subscribe()
	defer cleanup()

	msg := types.DownloadEvent{Message: "test message"}
	err := eb.Publish(msg)
	if err != nil {
		t.Fatalf("expected nil error on publish, got %v", err)
	}

	select {
	case received := <-sub:
		if received.Message != msg.Message {
			t.Errorf("expected %v, got %v", msg, received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	eb := NewEventBus()
	defer eb.Shutdown()

	sub1, cleanup1 := eb.Subscribe()
	defer cleanup1()

	sub2, cleanup2 := eb.Subscribe()
	defer cleanup2()

	msg := types.DownloadEvent{Message: "broadcast"}
	_ = eb.Publish(msg)

	for i, sub := range []<-chan types.DownloadEvent{sub1, sub2} {
		select {
		case received := <-sub:
			if received.Message != msg.Message {
				t.Errorf("subscriber %d expected %v, got %v", i+1, msg, received)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("subscriber %d timed out", i+1)
		}
	}
}

func TestEventBus_SlowObserversNeverWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := newEventSubscription(false)
		for range eventQueueCapacity {
			sub.critical <- types.DownloadEvent{Type: types.EventQueued}
		}
		eb := &EventBus{listeners: map[*eventSubscription]struct{}{sub: {}}}
		start := time.Now()
		eb.broadcastMsg(types.DownloadEvent{Type: types.EventProgress, Downloaded: 1})
		eb.broadcastMsg(types.DownloadEvent{Type: types.EventProgress, Downloaded: 2})
		if got := (<-sub.progress).Downloaded; got != 2 {
			t.Fatalf("latest progress=%d", got)
		}
		eb.broadcastMsg(types.DownloadEvent{Type: types.EventComplete})
		if elapsed := time.Since(start); elapsed != 0 {
			t.Fatalf("observer wait=%v", elapsed)
		}
		select {
		case <-sub.done:
		default:
			t.Fatal("overflowing observer was not disconnected")
		}
		if len(eb.listeners) != 0 {
			t.Fatal("disconnected observer retained")
		}
	})
}

func TestEventBus_ShutdownCleanly(t *testing.T) {
	eb := NewEventBus()

	sub, cleanup := eb.Subscribe()
	defer cleanup()

	eb.Shutdown()

	// Should not be able to publish after shutdown
	err := eb.Publish(types.DownloadEvent{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled on publish after shutdown, got %v", err)
	}

	// Channels should be closed
	select {
	case _, ok := <-sub:
		if ok {
			t.Error("expected channel to be closed after shutdown")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for subscriber channel to close")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus()
	defer eb.Shutdown()

	_, cleanup := eb.Subscribe()

	eb.listenerMu.Lock()
	count := len(eb.listeners)
	eb.listenerMu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 listener, got %d", count)
	}

	cleanup()

	// Wait for the asynchronous unsubscribe to be processed by broadcastLoop
	for i := 0; i < 10; i++ {
		eb.listenerMu.Lock()
		count = len(eb.listeners)
		eb.listenerMu.Unlock()
		if count == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if count != 0 {
		t.Fatalf("expected 0 listeners after cleanup, got %d", count)
	}

	// Should be safe to call cleanup multiple times (sync.Once)
	cleanup()
}

func TestEventBus_SaturatedObserverDoesNotDelayLifecycleOrHealthyClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := NewEventBus()
		slow, stopSlow := bus.Subscribe()
		defer stopSlow()
		healthy, stopHealthy := bus.Subscribe()
		defer stopHealthy()
		core, stopCore := bus.SubscribeLifecycle()
		defer stopCore()
		persisted := make(chan int, 1)
		go func() {
			count := 0
			for range core {
				count++
			}
			persisted <- count
		}()
		start := time.Now()
		const events = 500
		for index := range events {
			msg := types.DownloadEvent{Type: types.EventQueued, Downloaded: int64(index)}
			if err := bus.Publish(msg); err != nil {
				t.Fatal(err)
			}
			select {
			case got, ok := <-healthy:
				if !ok || got.Downloaded != int64(index) {
					t.Fatalf("healthy observer lost lifecycle event %d", index)
				}
			case <-time.After(time.Second):
				t.Fatal("healthy observer blocked behind slow client")
			}
		}
		if elapsed := time.Since(start); elapsed != 0 {
			t.Fatalf("observer added wait time: %v", elapsed)
		}
		for range slow {
		} // Overflow closes even an observer that never read before.
		bus.Shutdown()
		if got := <-persisted; got != events {
			t.Fatalf("persisted %d events, want %d", got, events)
		}
		after, cleanup := bus.Subscribe()
		defer cleanup()
		if _, ok := <-after; ok {
			t.Fatal("subscription after shutdown is not closed")
		}
	})
}

func TestEventBus_ConcurrentCleanupAndShutdown(t *testing.T) {
	bus := NewEventBus()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				_, cleanup := bus.Subscribe()
				_ = bus.Publish(types.DownloadEvent{Type: types.EventProgress})
				cleanup()
				cleanup()
			}
		}()
	}
	bus.Shutdown()
	wg.Wait()
}

func BenchmarkEventBusSaturatedObserver(b *testing.B) {
	sub := newEventSubscription(false)
	bus := &EventBus{listeners: map[*eventSubscription]struct{}{sub: {}}}
	msg := types.DownloadEvent{Type: types.EventBatchProgress}
	b.ReportAllocs()
	for b.Loop() {
		bus.broadcastMsg(msg)
	}
}
