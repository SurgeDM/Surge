package orchestrator

import (
	"sync"

	"github.com/SurgeDM/Surge/internal/types"
)

// Each observer owns its writer goroutine. None of these channels are closed by
// a producer except the reliable queue, after engine ingress has been drained.
type eventSubscription struct {
	reliable bool
	observer *types.ObserverOptions
	ids      map[string]bool
	critical chan types.DownloadEvent
	progress chan types.DownloadEvent
	out      chan types.DownloadEvent
	done     chan struct{}
	stopOnce sync.Once
}

func newEventSubscription(reliable bool) *eventSubscription {
	return &eventSubscription{reliable: reliable, critical: make(chan types.DownloadEvent, eventQueueCapacity), progress: make(chan types.DownloadEvent, 1), out: make(chan types.DownloadEvent), done: make(chan struct{})}
}

func (sub *eventSubscription) stop() { sub.stopOnce.Do(func() { close(sub.done) }) }

func persistenceEvent(msg types.DownloadEvent) bool {
	switch msg.Type {
	case types.EventStarted, types.EventQueued, types.EventComplete, types.EventPaused, types.EventError, types.EventRemoved:
		return true
	default:
		return false
	}
}

// Only the bus dispatch loop calls enqueue. No observer send waits on output.
func (sub *eventSubscription) enqueue(msg types.DownloadEvent) bool {
	select {
	case <-sub.done:
		return false
	default:
	}
	if sub.observer != nil {
		var keep bool
		msg, keep = sub.filter(msg)
		if !keep {
			return true
		}
	}
	if sub.reliable {
		if !persistenceEvent(msg) {
			return true
		}
		select {
		case sub.critical <- msg:
			return true
		case <-sub.done:
			return false
		}
	}
	if isProgressEvent(msg) {
		// Aggregated progress is a complete active-job sample. Keep the latest frame,
		// not a backlog of stale rates/bitmaps. Individual ticks are best effort too.
		select {
		case <-sub.progress:
		default:
		}
		select {
		case sub.progress <- msg:
		default:
		}
		return true
	}
	// A lifecycle transition supersedes any still-pending progress frame.
	select {
	case <-sub.progress:
	default:
	}
	select {
	case sub.critical <- msg:
		return true
	default:
		return false
	}
}

func (sub *eventSubscription) run() {
	defer close(sub.out)
	for {
		select {
		case <-sub.done:
			return
		default:
		}
		var msg types.DownloadEvent
		var ok bool
		// Lifecycle notifications take priority over replaceable progress.
		select {
		case msg, ok = <-sub.critical:
			if !ok {
				return
			}
		default:
			select {
			case <-sub.done:
				return
			case msg, ok = <-sub.critical:
				if !ok {
					return
				}
			case msg = <-sub.progress:
			}
		}
		select {
		case sub.out <- msg:
		case <-sub.done:
			return
		}
	}
}

// Filter before queueing, so unrelated work never consumes observer capacity.
func (sub *eventSubscription) filter(msg types.DownloadEvent) (types.DownloadEvent, bool) {
	if msg.Type == types.EventBatchProgress {
		if !sub.observer.Progress {
			return msg, false
		}
		batch := make([]types.DownloadEvent, 0, len(msg.BatchEvents))
		for _, item := range msg.BatchEvents {
			if filtered, ok := sub.filter(item); ok {
				batch = append(batch, filtered)
			}
		}
		msg.BatchEvents = batch
		return msg, len(batch) > 0
	}
	if msg.ResyncRequired {
		return msg, true
	}
	if msg.Type == types.EventSystem || msg.Type == types.EventRequest || msg.Type == types.EventBatchRequest {
		return msg, false
	}
	if len(sub.ids) > 0 && !sub.ids[msg.DownloadID] {
		return msg, false
	}
	if isProgressEvent(msg) && !sub.observer.Progress {
		return msg, false
	}
	if !sub.observer.Chunks {
		msg.ChunkBitmap = nil
		msg.ChunkProgress = nil
		msg.BitmapWidth = 0
		msg.ChunkSize = 0
	}
	return msg, true
}
