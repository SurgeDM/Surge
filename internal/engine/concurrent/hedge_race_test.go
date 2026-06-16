package concurrent

import (
	"github.com/SurgeDM/Surge/internal/engine/types"
	"sync"
	"testing"
	"time"
)

// TestHedgeSharedMaxOffsetRace exercises concurrent hedging and pointer reads.
// It runs HedgeWork in parallel with a reader that repeatedly accesses
// ActiveTask.SharedMaxOffset to ensure there is no data race under -race.
func TestHedgeSharedMaxOffsetRace(t *testing.T) {
	var d ConcurrentDownloader
	d.activeTasks = make(map[int]*ActiveTask)

	active := &ActiveTask{}
	active.CurrentOffset.Store(0)
	active.StopAt.Store(1 << 20)

	d.activeTasks[0] = active

	queue := NewTaskQueue()

	var wg sync.WaitGroup
	wg.Add(2)

	// Hedge goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			d.HedgeWork(queue)
			time.Sleep(time.Microsecond)
		}
	}()

	// Reader goroutine: repeatedly read the shared pointer (mimics downloadTask access)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			// Read under the task's RLock, matching production downloadTask behaviour.
			active.SharedMaxOffsetMu.RLock()
			ptr := active.SharedMaxOffset
			active.SharedMaxOffsetMu.RUnlock()
			if ptr != nil {
				_ = ptr.Load()
				// harmless CAS attempt
				ptr.CompareAndSwap(ptr.Load(), ptr.Load())
			}
			time.Sleep(time.Microsecond)
		}
	}()

	wg.Wait()

	// Consume any hedged tasks to avoid leaking goroutines
	for {
		tsk, ok := queue.Pop()
		if !ok {
			break
		}
		_ = tsk // ignore
	}

	// Ensure the task type still matches expectations
	var sample types.Task
	_ = sample
}
