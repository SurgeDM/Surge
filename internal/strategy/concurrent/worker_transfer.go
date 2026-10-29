package concurrent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// downloadTask downloads a single byte range and writes to file at offset
func (d *ConcurrentDownloader) downloadTask(ctx context.Context, rawurl string, file *os.File, activeTask *ActiveTask, buf []byte, client *http.Client, totalSize int64) error {
	if d.Runtime.IsAdaptiveConcurrencyEnabled() {
		activeTask.WaitingOnLimiter.Store(true)
		release, err := d.hostLimiter.AcquireRequest(ctx, transport.MirrorHost(rawurl), d.Runtime.GetMaxConnectionsPerDownload())
		activeTask.WaitingOnLimiter.Store(false)
		if err != nil {
			return err
		}
		defer release()
		activeTask.LastActivity.Store(time.Now().UnixNano())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return err
	}

	task := activeTask.Task

	// Apply custom headers first (from browser extension: cookies, auth, referer, etc.)
	for key, val := range d.Headers {
		// Set the range ourselves and request unencoded file bytes.
		if !strings.EqualFold(key, "Range") && !strings.EqualFold(key, "Accept-Encoding") {
			req.Header.Set(key, val)
		}
	}

	// Set User-Agent from config only if not provided in custom headers
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", d.Runtime.GetUserAgent())
	}
	// Range header is always set for partial downloads (overrides any browser Range header)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", task.Offset, task.Offset+task.Length-1))
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			utils.Debug("Error closing response body: %v", err)
		}
	}()
	// Both explicit rate limits and bare 503s require shared host backoff.
	if resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusServiceUnavailable {
		ra, ok := transport.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		return &rateLimitError{retryAfter: ra, explicit: ok}
	}

	// Validate status code
	if resp.StatusCode == http.StatusOK {
		// Valid only if we requested the full file
		// If we wanted a partial range but got the whole file (200), that's an error because we can't handle the full stream at a non-zero offset
		if task.Offset != 0 || task.Length != totalSize {
			return types.ErrRangeUnsupported
		}
	} else if resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusForbidden {
			return errSoftForbidden
		}
		if types.IsPermanentHTTPStatus(resp.StatusCode) {
			return fmt.Errorf("unexpected status: %d: %w", resp.StatusCode, types.ErrPermanentHTTP)
		}
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Batching State
	var pendingBytes int64
	var pendingStart int64 = -1
	lastUpdate := time.Now()
	batchSizeThreshold := int64(types.WorkerBatchSize)
	batchTimeThreshold := types.WorkerBatchInterval

	// Helper to flush pending updates to global state
	flushUpdates := func() {
		if pendingBytes > 0 {
			if d.State != nil {
				// Update Chunk Map (Global Lock)
				d.State.UpdateChunkStatus(pendingStart, pendingBytes, types.ChunkCompleted)

				// Update Downloaded Counter (Atomic)
				d.State.Bytes.Downloaded.Add(pendingBytes)
			}
			now := time.Now()
			d.soft403Mu.Lock()
			d.throttleEpisodeStart = time.Time{}
			d.soft403Since = time.Time{}
			clear(d.forbiddenByMirror)
			d.soft403Mu.Unlock()

			host := transport.MirrorHost(rawurl)
			if d.Runtime.IsAdaptiveConcurrencyEnabled() {
				_, recovered := d.hostLimiter.ReportHealthyProgress(host, pendingBytes, d.Runtime.GetMaxConnectionsPerDownload(), d.Runtime.GetAdaptiveConcurrencyInterval(), now)
				if recovered && d.concurrencyGate != nil {
					d.concurrencyGate.setCap(d.hostLimiter.ConcurrencyCapForHosts(d.workerHosts, d.Runtime.GetMaxConnectionsPerDownload()), time.Time{})
				}
			} else {
				d.hostLimiter.ReportProgressBytes(host, pendingBytes)
			}

			pendingBytes = 0
			pendingStart = -1
			lastUpdate = now
		}
	}
	// Ensure we flush whatever we have on exit
	defer flushUpdates()

	// Read and write at offset
	offset := task.Offset
	for {
		// Check if we should stop
		stopAt := activeTask.StopAt.Load()
		if offset >= stopAt {
			// Stealing happened, stop here
			return nil
		}

		// Calculate how much to read to fill buffer or hit stopAt/EOF
		// We want to fill buf as much as possible to minimize WriteAt calls

		// Limit by remaining length to stopAt
		remaining := stopAt - offset
		if remaining <= 0 {
			return nil
		}

		readSize := int64(len(buf))
		if readSize > remaining {
			readSize = remaining
		}

		readSoFar := 0
		var readErr error

		for readSoFar < int(readSize) {
			n, err := resp.Body.Read(buf[readSoFar:readSize])
			if n > 0 {
				readSoFar += n
				// CONTINUOUS HEALTH KEEPALIVE:
				// Update LastActivity directly off the TCP socket instead of waiting for the buffer
				// to completely fill and hit disk. This prevents the Health Monitor from killing
				// workers on slightly slower networks during the 500KB buffer acquisition.
				activeTask.LastActivity.Store(time.Now().UnixNano())
			}
			if err != nil {
				readErr = err
				break
			}
			if n == 0 {
				readErr = io.ErrUnexpectedEOF
				break
			}
		}

		if readSoFar > 0 {
			if d.Limiter != nil {
				// Reset stall clock before the wait so the health monitor measures
				// time from when throttling begins, not from the last network read.
				activeTask.LastActivity.Store(time.Now().UnixNano())
				activeTask.WaitingOnLimiter.Store(true)
				err := d.Limiter.WaitN(ctx, int64(readSoFar))
				activeTask.WaitingOnLimiter.Store(false)
				if err != nil {
					return err
				}

				// Refresh again after the wait to keep the stall clock current.
				activeTask.LastActivity.Store(time.Now().UnixNano())
			}

			// Disk writes can block for longer than the network stall timeout.
			// Include time waiting for RangeMu, since no network read happens there either.
			diskWriteStart := time.Now()
			activeTask.WritingToDisk.Store(true)
			activeTask.RangeMu.Lock()
			// Recheck the boundary while excluding StealWork. Without this lock,
			// stealing between the check and WriteAt could overlap the new task.
			currentStopAt := activeTask.StopAt.Load()
			if offset+int64(readSoFar) > currentStopAt {
				readSoFar = int(currentStopAt - offset)
				if readSoFar <= 0 {
					activeTask.RangeMu.Unlock()
					activeTask.LastActivity.Store(time.Now().UnixNano())
					activeTask.WritingToDisk.Store(false)
					return nil // stolen completely
				}
			}

			_, writeErr := writeAtFn(file, buf[:readSoFar], offset)
			if writeErr != nil {
				activeTask.RangeMu.Unlock()
				activeTask.LastActivity.Store(time.Now().UnixNano())
				activeTask.WritingToDisk.Store(false)
				return fmt.Errorf("write error: %w", writeErr)
			}

			now := time.Now()
			// The slow-worker comparison measures network throughput, so remove
			// time spent waiting for the write lock and disk from its sample window.
			activeTask.WindowStart = activeTask.WindowStart.Add(now.Sub(diskWriteStart))
			offset += int64(readSoFar)
			newlyWritten := int64(readSoFar)

			offset, newlyWritten = clampWriteToStopAt(offset, newlyWritten, activeTask.StopAt.Load())
			activeTask.CurrentOffset.Store(offset)
			activeTask.RangeMu.Unlock()

			offset, newlyWritten = clampWriteToStopAt(offset, newlyWritten, activeTask.StopAt.Load())
			activeTask.WindowBytes.Add(newlyWritten)
			activeTask.LastActivity.Store(now.UnixNano())
			activeTask.WritingToDisk.Store(false)

			// Calculate effective contribution
			if newlyWritten > 0 {
				if pendingStart == -1 {
					pendingStart = offset - newlyWritten
				}
				pendingBytes += newlyWritten
			}

			// Check thresholds
			if pendingBytes >= batchSizeThreshold || now.Sub(lastUpdate) >= batchTimeThreshold {
				flushUpdates()
			}

			// Update EMA speed using sliding window (2 second window)
			// This relies on WindowBytes which is updated atomically above, so independent of batching
			windowElapsed := now.Sub(activeTask.WindowStart).Seconds()
			if windowElapsed >= 2.0 {
				windowBytes := activeTask.WindowBytes.Swap(0)
				recentSpeed := float64(windowBytes) / windowElapsed

				activeTask.SpeedMu.Lock()
				alpha := d.Runtime.GetSpeedEmaAlpha()
				if alpha <= 0 || activeTask.Speed == 0 {
					// Alpha 0 disables smoothing and uses the latest measured speed directly.
					activeTask.Speed = recentSpeed
				} else {
					activeTask.Speed = (1-alpha)*activeTask.Speed + alpha*recentSpeed
				}
				activeTask.SpeedMu.Unlock()

				activeTask.WindowStart = now // Reset window
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read error: %w", readErr)
		}
	}

	// Clean io.EOF with offset still short of StopAt is a truncated body,
	// not success. UnexpectedEOF stays on the read-error path above.
	stopAt := activeTask.StopAt.Load()
	if offset < stopAt {
		return fmt.Errorf("early EOF: read up to %d, expected %d", offset, stopAt)
	}

	return nil
}
