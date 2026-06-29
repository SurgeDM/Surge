package concurrent

import (
	"sync"
	"sync/atomic"

	"github.com/SurgeDM/Surge/internal/engine/types"
)

// TaskQueue is a thread-safe work-stealing queue
type TaskQueue struct {
	tasks       []types.Task
	head        int
	mu          sync.Mutex
	cond        *sync.Cond
	done        bool
	idleWorkers atomic.Int64 // Atomic counter for idle workers
	waiting     atomic.Int64 // Number of workers currently waiting on cond
	size        atomic.Int64 // Queue size to avoid lock contention in Len callers
	// workerIdleCh is written (non-blocking) whenever a worker transitions to idle.
	// Consumers (e.g. the balancer) can select on WorkerIdleCh() for zero-latency
	// notification instead of polling on a fixed ticker.
	workerIdleCh chan struct{}
}

func NewTaskQueue() *TaskQueue {
	tq := &TaskQueue{
		// Buffer of 1 so the sending worker never blocks; excess signals are
		// coalesced (one pending notification is enough to wake the balancer).
		workerIdleCh: make(chan struct{}, 1),
	}
	tq.cond = sync.NewCond(&tq.mu)
	return tq
}

// WorkerIdleCh returns a channel that receives a value whenever a worker
// transitions from busy to idle (i.e. Pop blocks because the queue is empty).
// The channel has a buffer of 1 – signals are coalesced, not queued, so the
// receiver must re-check IdleWorkers() after waking.
func (q *TaskQueue) WorkerIdleCh() <-chan struct{} {
	return q.workerIdleCh
}

func (q *TaskQueue) Push(t types.Task) {
	q.mu.Lock()
	q.tasks = append(q.tasks, t)
	q.size.Add(1)
	q.signalWaitingWorkersLocked(1)
	q.mu.Unlock()
}

func (q *TaskQueue) PushMultiple(tasks []types.Task) {
	if len(tasks) == 0 {
		return
	}

	q.mu.Lock()
	q.tasks = append(q.tasks, tasks...)
	q.size.Add(int64(len(tasks)))
	q.signalWaitingWorkersLocked(len(tasks))
	q.mu.Unlock()
}

func (q *TaskQueue) Pop() (types.Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.tasks) == q.head && !q.done {
		q.idleWorkers.Add(1)
		q.waiting.Add(1)
		// Notify the balancer that a new idle worker is available.
		// Non-blocking send coalesces concurrent notifications into one.
		select {
		case q.workerIdleCh <- struct{}{}:
		default:
		}
		q.cond.Wait()
		q.waiting.Add(-1)
		q.idleWorkers.Add(-1)
	}

	if len(q.tasks) == q.head {
		return types.Task{}, false
	}

	t := q.tasks[q.head]
	q.head++
	q.size.Add(-1)
	if q.head > len(q.tasks)/2 {

		// slice instead of copy to avoid allocation
		q.tasks = q.tasks[q.head:]
		q.head = 0
	}
	return t, true
}

func (q *TaskQueue) Close() {
	q.mu.Lock()
	q.done = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *TaskQueue) Len() int {
	return int(q.size.Load())
}

func (q *TaskQueue) IdleWorkers() int64 {
	return q.idleWorkers.Load()
}

// DrainRemaining returns all remaining tasks in the queue (used for pause/resume)
func (q *TaskQueue) DrainRemaining() []types.Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.head >= len(q.tasks) {
		return nil
	}

	remaining := make([]types.Task, len(q.tasks)-q.head)
	copy(remaining, q.tasks[q.head:])
	q.tasks = nil
	q.head = 0
	q.size.Store(0)
	return remaining
}

func (q *TaskQueue) signalWaitingWorkersLocked(maxSignals int) {
	if maxSignals <= 0 {
		return
	}

	waiting := int(q.waiting.Load())
	if waiting <= 0 {
		return
	}

	if maxSignals > waiting {
		maxSignals = waiting
	}

	for i := 0; i < maxSignals; i++ {
		q.cond.Signal()
	}
}
