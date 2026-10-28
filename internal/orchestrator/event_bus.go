package orchestrator

import (
	"context"
	"sync"

	"github.com/SurgeDM/Surge/internal/types"
)

const eventQueueCapacity = 100

// EventBus separates reliable persistence from bounded, replaceable observers.
// InputCh is engine-owned ingress; producers must stop before Shutdown.
type EventBus struct {
	InputCh      chan types.DownloadEvent
	listenerMu   sync.Mutex
	listeners    map[*eventSubscription]struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	pubMu        sync.RWMutex
	pubWg        sync.WaitGroup
	shutdownOnce sync.Once
}

func NewEventBus() *EventBus {
	ctx, cancel := context.WithCancel(context.Background())
	bus := &EventBus{InputCh: make(chan types.DownloadEvent, eventQueueCapacity), listeners: make(map[*eventSubscription]struct{}), ctx: ctx, cancel: cancel}
	bus.wg.Add(1)
	go bus.broadcastLoop()
	return bus
}

func (bus *EventBus) broadcastLoop() {
	defer bus.wg.Done()
	for msg := range bus.InputCh {
		bus.broadcastMsg(msg)
	}
	bus.listenerMu.Lock()
	for sub := range bus.listeners {
		if sub.reliable {
			close(sub.critical)
		} else {
			sub.stop()
		}
	}
	bus.listeners = make(map[*eventSubscription]struct{})
	bus.listenerMu.Unlock()
}

func (bus *EventBus) broadcastMsg(msg types.DownloadEvent) {
	bus.listenerMu.Lock()
	listeners := make([]*eventSubscription, 0, len(bus.listeners))
	for sub := range bus.listeners {
		listeners = append(listeners, sub)
	}
	bus.listenerMu.Unlock()
	for _, sub := range listeners {
		if !sub.enqueue(msg) {
			sub.stop()
			bus.listenerMu.Lock()
			delete(bus.listeners, sub)
			bus.listenerMu.Unlock()
		}
	}
}

func isProgressEvent(msg types.DownloadEvent) bool {
	return msg.Type == types.EventProgress || msg.Type == types.EventBatchProgress
}

// Publish never waits for an observer. Progress can be replaced at the next
// sample; lifecycle publication can backpressure only on persistence work.
func (bus *EventBus) Publish(msg types.DownloadEvent) error {
	bus.pubMu.RLock()
	if bus.ctx.Err() != nil {
		bus.pubMu.RUnlock()
		return context.Canceled
	}
	bus.pubWg.Add(1)
	bus.pubMu.RUnlock()
	defer bus.pubWg.Done()
	if isProgressEvent(msg) {
		select {
		case bus.InputCh <- msg:
		default:
		}
		return nil
	}
	select {
	case bus.InputCh <- msg:
		return nil
	case <-bus.ctx.Done():
		return context.Canceled
	}
}

// Subscribe creates a bounded observer. Progress samples coalesce; overflowing
// lifecycle notifications close the stream so the client can resynchronize.
func (bus *EventBus) Subscribe() (<-chan types.DownloadEvent, func()) {
	return bus.subscribe(false, nil)
}

// SubscribeLifecycle is reserved for the persistence worker, which must consume
// until shutdown or call cleanup. It skips transient events and drains accepted
// lifecycle events on shutdown. Storage backpressure is intentional here.
func (bus *EventBus) SubscribeLifecycle() (<-chan types.DownloadEvent, func()) {
	return bus.subscribe(true, nil)
}

func (bus *EventBus) subscribe(reliable bool, options *types.ObserverOptions) (<-chan types.DownloadEvent, func()) {
	bus.pubMu.RLock()
	defer bus.pubMu.RUnlock()
	sub := newEventSubscription(reliable)
	if options != nil {
		copyOptions := *options
		copyOptions.IDs = nil
		sub.observer = &copyOptions
		sub.ids = make(map[string]bool, len(options.IDs))
		for _, id := range options.IDs {
			sub.ids[id] = true
		}
	}
	if bus.ctx.Err() != nil {
		close(sub.out)
		return sub.out, func() {}
	}
	bus.listenerMu.Lock()
	bus.listeners[sub] = struct{}{}
	bus.listenerMu.Unlock()
	bus.wg.Add(1)
	go func() { defer bus.wg.Done(); sub.run() }()
	cleanup := func() {
		sub.stop()
		bus.listenerMu.Lock()
		delete(bus.listeners, sub)
		bus.listenerMu.Unlock()
	}
	return sub.out, cleanup
}

// HasObservers avoids producing expensive progress snapshots for persistence alone.
func (bus *EventBus) HasObservers() bool {
	bus.listenerMu.Lock()
	defer bus.listenerMu.Unlock()
	for sub := range bus.listeners {
		if !sub.reliable {
			return true
		}
	}
	return false
}

func (bus *EventBus) Closed() bool { return bus.ctx.Err() != nil }

func (bus *EventBus) Shutdown() {
	bus.shutdownOnce.Do(func() {
		bus.pubMu.Lock()
		bus.cancel()
		bus.pubMu.Unlock()
		bus.pubWg.Wait()
		close(bus.InputCh)
	})
	bus.wg.Wait()
}

func (bus *EventBus) SubscribeObserver(options types.ObserverOptions) (<-chan types.DownloadEvent, func()) {
	return bus.subscribe(false, &options)
}

// ProgressInterest prevents lifecycle-only agents from producing progress or bitmaps.
func (bus *EventBus) ProgressInterest() (progress, chunks bool) {
	bus.listenerMu.Lock()
	defer bus.listenerMu.Unlock()
	for sub := range bus.listeners {
		if sub.reliable {
			continue
		}
		if sub.observer == nil {
			return true, true
		}
		progress = progress || sub.observer.Progress
		chunks = chunks || (sub.observer.Progress && sub.observer.Chunks)
	}
	return progress, chunks
}
