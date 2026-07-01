package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// EventBus handles broadcasting events from the orchestrator to all listeners.
type EventBus struct {
	InputCh    chan types.DownloadEvent
	listeners  []chan types.DownloadEvent
	listenerMu    sync.Mutex
	unsubscribeCh chan chan types.DownloadEvent
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func NewEventBus() *EventBus {
	ctx, cancel := context.WithCancel(context.Background())
	eb := &EventBus{
		InputCh:       make(chan types.DownloadEvent, 100),
		listeners:     make([]chan types.DownloadEvent, 0),
		unsubscribeCh: make(chan chan types.DownloadEvent, 10),
		ctx:           ctx,
		cancel:        cancel,
	}
	eb.wg.Add(1)
	go eb.broadcastLoop()
	return eb
}

func (eb *EventBus) broadcastLoop() {
	defer eb.wg.Done()
	for {
		select {
		case <-eb.ctx.Done():
			// Clean up on shutdown
			eb.listenerMu.Lock()
			for _, ch := range eb.listeners {
				close(ch)
			}
			eb.listeners = nil
			eb.listenerMu.Unlock()
			return
		case msg, ok := <-eb.InputCh:
			if !ok {
				eb.listenerMu.Lock()
				for _, ch := range eb.listeners {
					close(ch)
				}
				eb.listeners = nil
				eb.listenerMu.Unlock()
				return
			}

			eb.listenerMu.Lock()
			listenersCopy := make([]chan types.DownloadEvent, len(eb.listeners))
			copy(listenersCopy, eb.listeners)
			eb.listenerMu.Unlock()

			isProgress := false
			if msg.Type == types.EventProgress || msg.Type == types.EventBatchProgress {
				isProgress = true
			}

			for _, ch := range listenersCopy {
				func() {
					defer func() { _ = recover() }()
					if isProgress {
						select {
						case ch <- msg:
						default:
						}
					} else {
						select {
						case ch <- msg:
						case <-time.After(1 * time.Second):
							utils.Debug("Dropped critical event due to slow client")
						}
					}
				}()
			}
			
		case chToClose := <-eb.unsubscribeCh:
			eb.listenerMu.Lock()
			for i, listener := range eb.listeners {
				if listener == chToClose {
					eb.listeners = append(eb.listeners[:i], eb.listeners[i+1:]...)
					close(chToClose)
					break
				}
			}
			eb.listenerMu.Unlock()
		}
	}
}

// Publish emits an event into the bus.
func (eb *EventBus) Publish(msg types.DownloadEvent) error {
	select {
	case <-eb.ctx.Done():
		return context.Canceled
	case eb.InputCh <- msg:
		return nil
	case <-time.After(1 * time.Second):
		return context.DeadlineExceeded
	}
}

// Subscribe returns a channel that receives events.
func (eb *EventBus) Subscribe() (<-chan types.DownloadEvent, func()) {
	outCh := make(chan types.DownloadEvent, 100)
	eb.listenerMu.Lock()
	eb.listeners = append(eb.listeners, outCh)
	eb.listenerMu.Unlock()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			select {
			case eb.unsubscribeCh <- outCh:
			case <-eb.ctx.Done():
			}
		})
	}
	return outCh, cleanup
}

func (eb *EventBus) Shutdown() {
	eb.cancel()
	eb.wg.Wait()
}
