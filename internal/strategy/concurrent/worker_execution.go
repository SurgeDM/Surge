package concurrent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// taskResult describes scheduling policy, not a second progress counter. The
// transfer accounts for bytes on disk; remaining is snapshotted under RangeMu.
type taskResult struct {
	nextMirror        int
	err               error
	remaining         *types.Task
	requeue           bool
	preserveRemaining bool
	stop              bool
}

// executeTask consumes one gate permit and releases it on every exit. It removes
// the active task before returning a residual, using activeMu -> RangeMu order.
func (d *ConcurrentDownloader) executeTask(ctx context.Context, id int, task types.Task, mirrors, mirrorHosts []string, currentMirrorIdx int, file *os.File, buf []byte, client *http.Client, totalSize int64) (result taskResult) {
	defer func() { result.nextMirror = currentMirrorIdx }()
	if d.concurrencyGate != nil {
		defer d.concurrencyGate.release()
	}
	if d.State != nil && d.State.Bytes.VerifiedProgress.Load() >= totalSize {
		result.stop = true
		return
	}

	if d.State != nil {
		d.State.ActiveWorkers.Add(1)
		defer d.State.ActiveWorkers.Add(-1)
	}

	now := time.Now()
	activeTask := &ActiveTask{
		Task:        task,
		StartTime:   now,
		WindowStart: now,
	}
	activeTask.CurrentOffset.Store(task.Offset)
	activeTask.StopAt.Store(task.Offset + task.Length)
	activeTask.LastActivity.Store(now.UnixNano())

	d.activeMu.Lock()
	if d.State != nil && d.State.Bytes.VerifiedProgress.Load() >= totalSize {
		d.activeMu.Unlock()
		result.stop = true
		return
	}
	d.activeTasks[id] = activeTask
	d.activeMu.Unlock()

	defer func() {
		remaining := d.detachRemainingTask(id, activeTask)
		if result.err != nil || result.requeue {
			result.remaining = remaining
		}
	}()
	var lastErr error
	maxRetries := d.Runtime.GetMaxTaskRetries()
	genericAttempt := 0

	for {
		eligibleHosts, eligibleIndexes := d.eligibleWorkerMirrors(mirrors, mirrorHosts, currentMirrorIdx)
		if len(eligibleHosts) == 0 {
			result.err = types.ErrRangeUnsupported
			return
		}
		idx, wait := d.hostLimiter.PickMirror(eligibleHosts, 0, time.Now())
		currentMirrorIdx = eligibleIndexes[idx]
		if wait > 0 {
			if err := d.checkThrottleBudget(time.Now(), wait); err != nil {
				lastErr = err
				break
			}
			activeTask.WaitingOnLimiter.Store(true)
			if !interruptibleSleep(ctx, wait) {
				activeTask.WaitingOnLimiter.Store(false)
				result.err = ctx.Err()
				result.preserveRemaining = true
				return
			}
			activeTask.WaitingOnLimiter.Store(false)
			// Another response may have extended the shared cooldown.
			continue
		}
		currentURL := mirrors[currentMirrorIdx]

		attempt := d.executeTaskAttempt(ctx, id, currentURL, file, activeTask, buf, client, totalSize)
		lastErr = attempt.err

		// Disk full: fail immediately — no in-place retry, no mirror rotation,
		// no residual requeue. Stricter than permanent HTTP.
		// Stash RemainingTask off-queue so error-path saveStateSnapshot can
		// still persist the unfinished range after peers are cancelled.
		if types.IsInsufficientDiskSpace(lastErr) {
			result.err = lastErr
			result.preserveRemaining = true
			return
		}

		if ctx.Err() != nil {
			result.err = ctx.Err()
			result.preserveRemaining = true
			return
		}

		if attempt.cancelled && lastErr != nil {
			if d.completing.Load() {
				lastErr = nil
				break
			}
			currentMirrorIdx = (currentMirrorIdx + 1) % len(mirrors)
			utils.Debug("Worker %d: Health check cancelled task, rotating from mirror %s to %s", id, mirrors[(currentMirrorIdx+len(mirrors)-1)%len(mirrors)], mirrors[currentMirrorIdx])

			result.requeue = true
			lastErr = nil
			break
		}

		if lastErr == nil {
			d.soft403Mu.Lock()
			delete(d.forbiddenByMirror, currentURL)
			d.soft403Mu.Unlock()
			host := mirrorHosts[currentMirrorIdx]
			d.hostLimiter.RecordSuccess(host)
			if d.State != nil && !d.hostLimiter.AnyBlocked(mirrorHosts, time.Now()) {
				d.State.RateLimited.Store(false)
			}

			stopAt := activeTask.StopAt.Load()
			current := activeTask.CurrentOffset.Load()
			if current < task.Offset+task.Length && current >= stopAt {
				utils.Debug("Worker stopped early due to stealing")
			}
			break
		}

		if errors.Is(lastErr, errSoftForbidden) {
			now := time.Now()
			d.soft403Mu.Lock()
			if d.soft403Since.IsZero() {
				d.soft403Since = now
			}
			d.forbiddenByMirror[currentURL]++
			allForbidden := true
			for _, mirror := range mirrors {
				if !d.nonRangeMirrors[mirror] && d.forbiddenByMirror[mirror] < 3 {
					allForbidden = false
				}
			}
			confirmed := allForbidden && now.Sub(d.soft403Since) >= 5*time.Second
			d.soft403Mu.Unlock()
			if confirmed {
				lastErr = fmt.Errorf("all range-capable mirrors repeatedly returned 403 without progress: %w", types.ErrPermanentHTTP)
			} else {
				lastErr = &rateLimitError{retryAfter: 500 * time.Millisecond, explicit: true}
			}
		}

		var rlErr *rateLimitError
		if errors.As(lastErr, &rlErr) {
			if err := d.handleTaskThrottle(rlErr, mirrorHosts, currentMirrorIdx); err != nil {
				result.err = err
				return
			}
			d.ReportMirrorError(currentURL)
			currentMirrorIdx = (currentMirrorIdx + 1) % len(mirrors)
			result.requeue = true
			lastErr = nil
			break
		}

		if errors.Is(lastErr, types.ErrRangeUnsupported) {
			d.soft403Mu.Lock()
			d.nonRangeMirrors[currentURL] = true
			allUnsupported := true
			for _, mirror := range mirrors {
				if !d.nonRangeMirrors[mirror] {
					allUnsupported = false
				}
			}
			d.soft403Mu.Unlock()
			if allUnsupported {
				break
			}
			result.requeue = true
			currentMirrorIdx = (currentMirrorIdx + 1) % len(mirrors)
			lastErr = nil
			break
		}

		// Permanent HTTP (404/401/410/416): stop without burning
		// GetMaxTaskRetries or the single-mirror backoff. 403 stays
		// errSoftForbidden so confirmation-then-escalate still runs.
		if types.IsPermanentHTTPError(lastErr) {
			break
		}

		genericAttempt++
		if genericAttempt >= maxRetries {
			break
		}
		d.ReportMirrorError(mirrors[currentMirrorIdx])
		currentMirrorIdx = (currentMirrorIdx + 1) % len(mirrors)
		if len(mirrors) == 1 {
			activeTask.WaitingOnLimiter.Store(true)
			interruptibleSleep(ctx, time.Duration(1<<genericAttempt)*types.RetryBaseDelay)
			activeTask.WaitingOnLimiter.Store(false)
		}
		resumeOnRetryOffset(&task, activeTask)
	}

	if d.completing.Load() {
		result.requeue = false
		result.stop = true
		return
	}
	result.err = lastErr
	return
}

func (d *ConcurrentDownloader) eligibleWorkerMirrors(mirrors, mirrorHosts []string, currentMirrorIdx int) ([]string, []int) {
	eligibleHosts := make([]string, 0, len(mirrors))
	eligibleIndexes := make([]int, 0, len(mirrors))
	d.soft403Mu.Lock()
	for i := range mirrors {
		idx := (currentMirrorIdx + i) % len(mirrors)
		if !d.nonRangeMirrors[mirrors[idx]] {
			eligibleHosts = append(eligibleHosts, mirrorHosts[idx])
			eligibleIndexes = append(eligibleIndexes, idx)
		}
	}
	d.soft403Mu.Unlock()
	return eligibleHosts, eligibleIndexes
}

type taskAttemptResult struct {
	err       error
	cancelled bool
}

func (d *ConcurrentDownloader) executeTaskAttempt(ctx context.Context, id int, currentURL string, file *os.File, activeTask *ActiveTask, buf []byte, client *http.Client, totalSize int64) taskAttemptResult {
	task := activeTask.Task
	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()
	now := time.Now()

	d.activeMu.Lock()
	activeTask.Cancel = taskCancel
	activeTask.StartTime = now
	d.activeMu.Unlock()

	activeTask.WindowStart = now
	activeTask.WindowBytes.Store(0)
	activeTask.LastActivity.Store(now.UnixNano())

	if d.State != nil {
		utils.Debug("Worker %d: Setting range %d-%d to Downloading", id, task.Offset, task.Offset+task.Length)
		d.State.UpdateChunkStatus(task.Offset, task.Length, types.ChunkDownloading)
	} else {
		utils.Debug("Worker %d: d.State is nil, cannot update chunk status", id)
	}

	taskStart := time.Now()
	err := d.downloadTask(taskCtx, currentURL, file, activeTask, buf, client, totalSize)

	wasExternallyCancelled := taskCtx.Err() != nil

	utils.Debug("Worker %d: Task offset=%d length=%d took %v", id, task.Offset, task.Length, time.Since(taskStart))

	return taskAttemptResult{err: err, cancelled: wasExternallyCancelled}
}

func (d *ConcurrentDownloader) handleTaskThrottle(rlErr *rateLimitError, mirrorHosts []string, currentMirrorIdx int) error {
	retryAfter := rlErr.retryAfter
	explicit := rlErr.explicit
	host := mirrorHosts[currentMirrorIdx]
	now := time.Now()
	reportedCap := 0
	if d.concurrencyGate != nil && d.Runtime.IsAdaptiveConcurrencyEnabled() {
		reportedCap = d.concurrencyGate.currentCap()
	}
	// Preserve the host deadline even when this download cannot wait.
	until, _ := d.hostLimiter.ReportThrottle(host, reportedCap, retryAfter, explicit, now)

	d.soft403Mu.Lock()
	if d.throttleEpisodeStart.IsZero() {
		d.throttleEpisodeStart = now
	}
	elapsedThrottle := now.Sub(d.throttleEpisodeStart)
	remainingBudget := 10*time.Minute - elapsedThrottle
	if explicit && retryAfter > remainingBudget {
		d.soft403Mu.Unlock()
		return fmt.Errorf("throttled with Retry-After %v exceeding remaining budget %v: %w", retryAfter, remainingBudget, types.ErrRetryBudgetExceeded)
	}
	if elapsedThrottle > 10*time.Minute {
		d.soft403Mu.Unlock()
		return fmt.Errorf("no download progress for %v due to sustained rate limiting: %w", elapsedThrottle, types.ErrRetryBudgetExceeded)
	}
	d.soft403Mu.Unlock()

	gateCap := 0
	if d.concurrencyGate != nil {
		gateCap = d.concurrencyGate.currentCap()
	}
	if d.concurrencyGate != nil {
		if !d.Runtime.IsAdaptiveConcurrencyEnabled() {
			d.concurrencyGate.setCap(gateCap, until)
		} else {
			d.concurrencyGate.setCap(d.hostLimiter.ConcurrencyCapForHosts(mirrorHosts, d.Runtime.GetMaxConnectionsPerDownload()), until)
		}
	}
	if d.State == nil || !d.State.RateLimited.Swap(true) {
		wait := time.Until(until).Round(time.Second)
		message := fmt.Sprintf("Rate limited by %s; cooling down for %s", host, wait)
		utils.Debug("%s", message)
		if d.ProgressChan != nil {
			select {
			case d.ProgressChan <- types.DownloadEvent{Type: types.EventSystem, DownloadID: d.ID, Message: message}:
			default:
			}
		}
	}
	return nil
}
