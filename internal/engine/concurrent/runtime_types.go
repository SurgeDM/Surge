package concurrent

import (
	"errors"
	"sync/atomic"

	"github.com/surge-downloader/surge/internal/engine/types"
)

const maxLineageStalls = 3

var (
	errIncompleteRange = errors.New("incomplete range download")
	errRangeProtocol   = errors.New("range protocol violation")
)

// RetryDisposition tells the downloader how to handle a worker-reported result.
type RetryDisposition uint8

const (
	DispositionRequeue RetryDisposition = iota
	DispositionAbort
	DispositionDrop
)

// queuedTask is a concurrent-internal envelope that keeps runtime scheduling
// metadata out of persisted task state.
type queuedTask struct {
	task      types.Task
	lineageID uint64
}

func (q queuedTask) withTask(task types.Task) queuedTask {
	q.task = task
	return q
}

type workerResult struct {
	qTask       queuedTask
	err         error
	disposition RetryDisposition
}

type pendingResultCounter struct {
	count atomic.Int64
}

func (p *pendingResultCounter) Begin() {
	if p == nil {
		return
	}
	p.count.Add(1)
}

func (p *pendingResultCounter) Complete() {
	if p == nil {
		return
	}
	p.count.Add(-1)
}

func (p *pendingResultCounter) Requeue(q *TaskQueue, task queuedTask) {
	if p == nil {
		q.Push(task)
		return
	}
	q.Push(task)
	p.count.Add(-1)
}

func (p *pendingResultCounter) Load() int64 {
	if p == nil {
		return 0
	}
	return p.count.Load()
}

type lineageState struct {
	highestProgress int64
	stallCount      int
}

type RetryTracker struct {
	states    map[uint64]*lineageState
	maxStalls int
}

func NewRetryTracker(maxStalls int) *RetryTracker {
	if maxStalls <= 0 {
		maxStalls = maxLineageStalls
	}
	return &RetryTracker{
		states:    make(map[uint64]*lineageState),
		maxStalls: maxStalls,
	}
}

func (t *RetryTracker) Evaluate(qTask queuedTask) RetryDisposition {
	if t == nil {
		return DispositionRequeue
	}

	state, ok := t.states[qTask.lineageID]
	if !ok {
		t.states[qTask.lineageID] = &lineageState{
			highestProgress: qTask.task.Offset,
			stallCount:      1,
		}
		return DispositionRequeue
	}

	if qTask.task.Offset > state.highestProgress {
		state.highestProgress = qTask.task.Offset
		state.stallCount = 1
		return DispositionRequeue
	}

	state.stallCount++
	if state.stallCount > t.maxStalls {
		return DispositionAbort
	}

	return DispositionRequeue
}

func unwrapQueuedTasks(tasks []queuedTask) []types.Task {
	if len(tasks) == 0 {
		return nil
	}

	plain := make([]types.Task, 0, len(tasks))
	for _, qTask := range tasks {
		plain = append(plain, qTask.task)
	}
	return plain
}

func loadVerifiedBytes(state *types.ProgressState, fallback *atomic.Int64) int64 {
	if state != nil {
		return state.VerifiedProgress.Load()
	}
	if fallback != nil {
		return fallback.Load()
	}
	return 0
}
