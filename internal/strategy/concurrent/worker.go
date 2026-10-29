package concurrent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// writeAtFn is injectable so tests can simulate ENOSPC without a full disk.
var writeAtFn = func(f *os.File, b []byte, off int64) (int, error) {
	return f.WriteAt(b, off)
}

var errSoftForbidden = errors.New("unexpected status: 403")

// worker schedules ranges. executeTask owns each admitted task's lifecycle;
// queue disposition happens only after it has left the steal set.
func (d *ConcurrentDownloader) worker(ctx context.Context, id int, mirrors []string, file *os.File, queue *TaskQueue, totalSize int64, client *http.Client) error {
	bufPtr := d.bufPool.Get().(*[]byte)
	defer d.bufPool.Put(bufPtr)
	utils.Debug("Worker %d started", id)
	defer utils.Debug("Worker %d finished", id)
	currentMirrorIdx := id % len(mirrors)
	mirrorHosts := make([]string, len(mirrors))
	for i, mirror := range mirrors {
		mirrorHosts[i] = transport.MirrorHost(mirror)
	}
	for {
		if d.concurrencyGate != nil && !d.concurrencyGate.acquire(ctx) {
			return ctx.Err()
		}
		task, ok := queue.Pop()
		if !ok {
			if d.concurrencyGate != nil {
				d.concurrencyGate.release()
			}
			return nil
		}
		result := d.executeTask(ctx, id, task, mirrors, mirrorHosts, currentMirrorIdx, file, *bufPtr, client, totalSize)
		currentMirrorIdx = result.nextMirror
		if result.remaining != nil {
			if result.preserveRemaining {
				d.activeMu.Lock()
				d.abandonedRemaining = append(d.abandonedRemaining, *result.remaining)
				d.activeMu.Unlock()
			} else {
				queue.Push(*result.remaining)
			}
		}
		if result.err != nil {
			return result.err
		}
		if result.stop {
			return nil
		}
	}
}

// detachRemainingTask removes an active task from the steal set and snapshots
// its remaining range under the documented activeMu -> RangeMu lock order.
func (d *ConcurrentDownloader) detachRemainingTask(id int, active *ActiveTask) *types.Task {
	d.activeMu.Lock()
	defer d.activeMu.Unlock()
	delete(d.activeTasks, id)
	return active.RemainingTask()
}

// StealWork tries to split an active task from a busy worker
// It greedily targets the worker with the MOST remaining work.
func (d *ConcurrentDownloader) StealWork(queue *TaskQueue) bool {
	// Don't steal while every mirror host is in cooldown: the stolen range
	// would just be re-requested against a still-throttled host.
	if d.hostLimiter != nil {
		if _, wait := d.hostLimiter.PickMirror(d.workerHosts, 0, time.Now()); wait > 0 {
			utils.Debug("Balancer: steal suppressed, all worker hosts in cooldown")
			return false
		}
	}

	d.activeMu.Lock()
	defer d.activeMu.Unlock()

	splitFloor := d.Runtime.GetMinChunkSize()
	if d.Runtime.IsAdaptiveConcurrencyEnabled() &&
		(d.concurrencyGate == nil || !d.concurrencyGate.sawThrottle()) {
		splitFloor = types.AlignSize
	}

	bestID := -1
	var maxRemaining int64 = 0
	var bestActive *ActiveTask

	// Find the worker with the MOST remaining work
	for id, active := range d.activeTasks {
		remaining := active.RemainingBytes()
		if remaining > splitFloor && remaining > maxRemaining {
			maxRemaining = remaining
			bestID = id
			bestActive = active
		}
	}

	if bestID == -1 {
		return false
	}

	// Found the best candidate, now try to steal
	active := bestActive
	active.RangeMu.Lock()
	defer active.RangeMu.Unlock()
	current := active.CurrentOffset.Load()
	originalEnd := active.StopAt.Load()
	remaining := originalEnd - current
	if remaining <= 0 {
		return false
	}

	// Split in half, aligned to AlignSize
	splitSize := alignedSplitSize(remaining, splitFloor)
	if splitSize == 0 {
		return false
	}

	newStopAt := current + splitSize

	// Update the active task stop point
	active.StopAt.Store(newStopAt)

	stolenTask := types.Task{
		Offset: newStopAt,
		Length: originalEnd - newStopAt,
	}

	queue.Push(stolenTask)
	utils.Debug("Balancer: stole %s from worker %d (new range: %d-%d)",
		utils.FormatBytes(stolenTask.Length), bestID, stolenTask.Offset, stolenTask.Offset+stolenTask.Length)

	return true
}

func clampWriteToStopAt(offset, newlyWritten, stopAt int64) (int64, int64) {
	if offset > stopAt {
		excess := offset - stopAt
		offset = stopAt
		if newlyWritten > excess {
			newlyWritten -= excess
		} else {
			newlyWritten = 0
		}
	}
	return offset, newlyWritten
}

func resumeOnRetryOffset(task *types.Task, activeTask *ActiveTask) {
	current := activeTask.CurrentOffset.Load()
	origEnd := task.Offset + task.Length
	stopAt := activeTask.StopAt.Load()
	effectiveEnd := stopAt
	if origEnd < effectiveEnd {
		effectiveEnd = origEnd
	}
	length := effectiveEnd - current
	if length < 0 {
		length = 0
	}
	task.Offset = current
	task.Length = length
	activeTask.Task = *task
}
