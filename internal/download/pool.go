package download

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
)

// activeDownload tracks a download that's currently running
type activeDownload struct {
	config types.DownloadConfig
	cancel context.CancelFunc
	// running is true while the worker goroutine is executing TUIDownload for this config.
	running atomic.Bool
}

type WorkerPool struct {
	taskChan     chan types.DownloadConfig
	progressCh   chan<- any
	progressDone chan struct{}                   // closed when progressCh must no longer be sent to
	downloads    map[string]*activeDownload      // Track active downloads for pause/resume
	queued       map[string]types.DownloadConfig // Track queued downloads
	mu           sync.RWMutex
	wg           sync.WaitGroup // We use this to wait for all active downloads to pause before exiting the program
	maxDownloads int
}

var (
	// gracefulShutdownPauseSoftTimeout controls when we emit a warning that
	// pausing is taking longer than expected. It is intentionally soft; shutdown
	// continues waiting for durable pause persistence.
	gracefulShutdownPauseSoftTimeout = 10 * time.Second
	// gracefulShutdownPausePollInterval controls how often shutdown rechecks pause state.
	gracefulShutdownPausePollInterval = 100 * time.Millisecond
	// gracefulShutdownPauseHardTimeout prevents indefinite shutdown hangs if a worker is stuck.
	gracefulShutdownPauseHardTimeout = 30 * time.Second
	// cancelStopWaitTimeout bounds how long Cancel waits for an active worker to exit.
	cancelStopWaitTimeout = 3 * time.Second
	// cancelStopPollInterval controls polling cadence while waiting for cancel to take effect.
	cancelStopPollInterval = 10 * time.Millisecond
)

func NewWorkerPool(progressCh chan<- any, maxDownloads int) *WorkerPool {
	if maxDownloads < 1 {
		maxDownloads = 3 // Default to 3 if invalid
	}
	pool := &WorkerPool{
		taskChan:     make(chan types.DownloadConfig, 100), // We make it buffered to avoid blocking add
		progressCh:   progressCh,
		progressDone: make(chan struct{}),
		downloads:    make(map[string]*activeDownload),
		queued:       make(map[string]types.DownloadConfig),
		maxDownloads: maxDownloads,
	}
	for i := 0; i < maxDownloads; i++ {
		go pool.worker()
	}
	return pool
}

// syncConfigFromState syncs Filename, DestPath, and Mirrors from the associated state.
func syncConfigFromState(cfg *types.DownloadConfig) {
	if cfg.State == nil {
		return
	}
	if fn := cfg.State.GetFilename(); fn != "" {
		cfg.Filename = fn
	}
	if dp := cfg.State.GetDestPath(); dp != "" {
		cfg.DestPath = dp
	}
	if ms := cfg.State.GetMirrors(); len(ms) > 0 {
		var urls []string
		for _, m := range ms {
			urls = append(urls, m.URL)
		}
		cfg.Mirrors = urls
	}
}

// resolveDestPath resolves the destination path consistently from config, state, and output bounds.
func resolveDestPath(cfg *types.DownloadConfig) string {
	destPath := cfg.DestPath
	if destPath == "" && cfg.State != nil {
		destPath = cfg.State.GetDestPath()
	}
	if destPath == "" && cfg.OutputPath != "" && cfg.Filename != "" {
		destPath = filepath.Join(cfg.OutputPath, cfg.Filename)
	}
	if destPath == "" {
		destPath = cfg.OutputPath // default fallback
	}
	return destPath
}

// Add adds a new download task to the pool
func (p *WorkerPool) Add(cfg types.DownloadConfig) {
	if cfg.ProgressCh == nil {
		cfg.ProgressCh = p.progressCh
	}
	p.mu.Lock()
	p.queued[cfg.ID] = cfg
	p.mu.Unlock()

	if !cfg.IsResume {
		p.trySendProgress(events.DownloadQueuedMsg{
			DownloadID: cfg.ID,
			Filename:   cfg.Filename,
			URL:        cfg.URL,
			DestPath:   resolveDestPath(&cfg),
			Mirrors:    append([]string(nil), cfg.Mirrors...),
		})
	}

	p.taskChan <- cfg
}

// HasDownload reports whether a download with the given URL is currently active or queued in the pool.
func (p *WorkerPool) HasDownload(url string) bool {
	p.mu.RLock()
	for _, ad := range p.downloads {
		if ad.config.URL == url {
			p.mu.RUnlock()
			return true
		}
	}
	for _, qd := range p.queued {
		if qd.URL == url {
			p.mu.RUnlock()
			return true
		}
	}
	p.mu.RUnlock()

	return false
}

// ActiveCount returns the number of currently active (downloading/pausing) downloads
func (p *WorkerPool) ActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, ad := range p.downloads {
		// Count if not completed and not fully paused
		if ad.config.State != nil && !ad.config.State.Done.Load() && !ad.config.State.IsPaused() {
			count++
		}
	}
	// Also count queued
	count += len(p.queued)
	return count
}

// GetAll returns all active download configs (for listing)
func (p *WorkerPool) GetAll() []types.DownloadConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var configs []types.DownloadConfig
	for _, ad := range p.downloads {
		cfg := ad.config
		syncConfigFromState(&cfg)
		configs = append(configs, cfg)
	}
	for _, cfg := range p.queued {
		configs = append(configs, cfg)
	}
	return configs
}

// Pause pauses a specific download by ID. Returns true if found and pause initiated (or already paused), false otherwise.
func (p *WorkerPool) Pause(downloadID string) bool {
	p.mu.RLock()
	ad, exists := p.downloads[downloadID]
	p.mu.RUnlock()

	if !exists || ad == nil {
		return false
	}

	// Set paused flag and cancel context
	if ad.config.State != nil {
		// Idempotency: If already paused, do nothing.
		if ad.config.State.IsPaused() {
			return true
		}
		// If transition is already in progress, still ensure worker context is canceled.
		if ad.config.State.IsPausing() {
			if ad.cancel != nil {
				ad.cancel()
			}
			return true
		}
		ad.config.State.SetPausing(true) // Mark as transitioning to pause
		ad.config.State.Pause()
	}
	// Always cancel worker context as a safety net (single downloader does not set state cancel itself).
	if ad.cancel != nil {
		ad.cancel()
	}

	// Send pause message is now exclusively handled by worker return paths
	// to ensure fully synchronized byte counts.
	return true
}

// PauseAll pauses all active downloads (for graceful shutdown)
func (p *WorkerPool) PauseAll() {
	p.mu.RLock()
	ids := make([]string, 0, len(p.downloads)) // This stores the uuids of the downloads to be paused
	for id, ad := range p.downloads {
		// Only pause downloads that are actually active (not already paused or done or pausing)
		if ad != nil && ad.config.State != nil && !ad.config.State.IsPaused() && !ad.config.State.Done.Load() && !ad.config.State.IsPausing() {
			ids = append(ids, id)
		}
	}
	p.mu.RUnlock()

	for _, id := range ids {
		p.Pause(id)
	}
}

// Cancel cancels and removes a download by ID
func (p *WorkerPool) Cancel(downloadID string) {
	p.mu.Lock()
	ad, activeExists := p.downloads[downloadID]
	qCfg, queuedExists := p.queued[downloadID]
	if activeExists {
		delete(p.downloads, downloadID)
	}
	if queuedExists {
		delete(p.queued, downloadID)
	}
	p.mu.Unlock()

	if !activeExists && !queuedExists {
		return
	}

	removedFilename := ""
	removedDestPath := ""
	removedCompleted := false
	if activeExists && ad != nil {
		removedFilename = ad.config.Filename
		removedDestPath = resolveDestPath(&ad.config)
		removedCompleted = ad.config.State != nil && ad.config.State.Done.Load()

		// Cancel the context to stop workers
		if ad.cancel != nil {
			ad.cancel()
		}

		// Best effort: wait for worker to exit so delete cleanup doesn't race with
		// downloader startup that can recreate the .surge file after removal.
		deadline := time.Now().Add(cancelStopWaitTimeout)
		for ad.running.Load() && time.Now().Before(deadline) {
			time.Sleep(cancelStopPollInterval)
		}

		// Mark as done to stop polling
		if ad.config.State != nil {
			ad.config.State.Done.Store(true)
		}
	} else if queuedExists {
		removedFilename = qCfg.Filename
		removedDestPath = resolveDestPath(&qCfg)
	}

	// Send removal message
	p.trySendProgress(events.DownloadRemovedMsg{
		DownloadID: downloadID,
		Filename:   removedFilename,
		DestPath:   removedDestPath,
		Completed:  removedCompleted,
	})
}

// Resume resumes a paused download by ID. Returns true if found and resumed (or already running), false otherwise.
func (p *WorkerPool) Resume(downloadID string) bool {
	p.mu.RLock()
	ad, exists := p.downloads[downloadID]
	p.mu.RUnlock()

	if !exists || ad == nil {
		return false
	}

	// Prevent race: Don't resume if still pausing
	if ad.config.State != nil && ad.config.State.IsPausing() {
		utils.Debug("Resume ignored: download %s is still pausing", downloadID)
		return false
	}

	// Idempotency: If already running (not paused), do nothing
	if ad.config.State != nil && !ad.config.State.IsPaused() {
		utils.Debug("Resume ignored: download %s is already running", downloadID)
		return true
	}

	// Clear paused flag and reset session start to avoid speed spikes/dips checks
	if ad.config.State != nil {
		ad.config.State.Resume()
		ad.config.State.SyncSessionStart()
		syncConfigFromState(&ad.config)
	}

	// Hydrate resume config from persisted pause snapshot when available.
	if ad.config.URL != "" && ad.config.DestPath != "" {
		if saved, err := state.LoadState(ad.config.URL, ad.config.DestPath); err == nil && saved != nil {
			ad.config.SavedState = saved
			if saved.TotalSize > 0 {
				ad.config.TotalSize = saved.TotalSize
			}
			if len(saved.Tasks) > 0 {
				ad.config.SupportsRange = true
			}
		}
	}

	// Re-queue the download
	ad.config.IsResume = true
	p.Add(ad.config)

	// Send resume message
	p.trySendProgress(events.DownloadResumedMsg{
		DownloadID: downloadID,
		Filename:   ad.config.Filename,
	})
	return true
}

// UpdateURL updates the URL of a download by ID.
// It fails if the download is actively downloading (not paused or errored).
func (p *WorkerPool) UpdateURL(downloadID string, newURL string) error {
	p.mu.RLock()
	ad, exists := p.downloads[downloadID]
	_, qExists := p.queued[downloadID]
	p.mu.RUnlock()

	if qExists {
		return fmt.Errorf("cannot update URL for a queued download, please cancel or wait for it to start")
	}

	if exists && ad != nil {
		// If it exists in the active pool, it must be paused
		if ad.config.State != nil && !ad.config.State.IsPaused() {
			if ad.running.Load() {
				return fmt.Errorf("download is currently active, please pause it before updating the URL")
			}
		}

		// Update the active download's config
		ad.config.URL = newURL
		if ad.config.State != nil {
			ad.config.State.SetURL(newURL)
		}
	}

	return state.UpdateURL(downloadID, newURL)
}

func (p *WorkerPool) worker() {
	for cfg := range p.taskChan {
		p.mu.RLock()
		_, stillQueued := p.queued[cfg.ID]
		p.mu.RUnlock()
		if !stillQueued {
			// Canceled while waiting in queue.
			continue
		}

		p.wg.Add(1)
		// Create cancellable context
		ctx, cancel := context.WithCancel(context.Background())

		// Register active download
		ad := &activeDownload{
			config: cfg,
			cancel: cancel,
		}
		if ad.config.State != nil {
			ad.config.State.SetCancelFunc(cancel)
		}
		ad.running.Store(true)
		p.mu.Lock()
		delete(p.queued, cfg.ID)
		p.downloads[cfg.ID] = ad
		p.mu.Unlock()

		err := TUIDownload(ctx, &ad.config)
		ad.running.Store(false)

		// Logic:
		// 1. If Pause() was called: State.IsPaused() is true. We keep the task in p.downloads (so it can be resumed).
		// 2. If finished/error: We remove from p.downloads.

		isPaused := ad.config.State != nil && ad.config.State.IsPaused()

		// Clear "Pausing" transition state now that worker has exited
		if ad.config.State != nil {
			ad.config.State.SetPausing(false)
		}

		if isPaused {
			utils.Debug("WorkerPool: Download %s paused cleanly", cfg.ID)
			// If paused, we keep it in downloads map for potential resume
		} else if err != nil {
			if cfg.State != nil {
				cfg.State.SetError(err)
			}
			p.trySendProgress(events.DownloadErrorMsg{
				DownloadID: cfg.ID,
				Filename:   cfg.Filename,
				DestPath:   resolveDestPath(&cfg),
				Err:        err,
			})
			// Clean up errored download from tracking (don't save to .surge)
			p.mu.Lock()
			delete(p.downloads, cfg.ID)
			p.mu.Unlock()

		} else {
			// Only mark as done if not paused
			if cfg.State != nil {
				cfg.State.Done.Store(true)
			}
			// Note: DownloadCompleteMsg is sent by the progress reporter when it detects Done=true

			// Clean up from tracking
			p.mu.Lock()
			delete(p.downloads, cfg.ID)
			p.mu.Unlock()
		}
		// If paused, we keep it in downloads map for potential resume
		p.wg.Done()
	}
}

// GetStatus returns the status of an active download
func (p *WorkerPool) GetStatus(id string) *types.DownloadStatus {
	p.mu.RLock()
	ad, exists := p.downloads[id]
	qCfg, qExists := p.queued[id]
	p.mu.RUnlock()

	if !exists && !qExists {
		return nil
	}

	if qExists {
		return &types.DownloadStatus{
			ID:         id,
			URL:        qCfg.URL,
			Filename:   qCfg.Filename,
			DestPath:   resolveDestPath(&qCfg),
			Status:     "queued",
			Downloaded: 0,
			TotalSize:  0, // Metadata not yet fetched
		}
	}

	state := ad.config.State
	if state == nil {
		return nil
	}

	// Use state filename/destpath if available (thread-safe)
	filename := ad.config.Filename
	if str := state.GetFilename(); str != "" {
		filename = str
	}

	// Calculate progress and speed (thread-safe)
	downloaded, totalSize, _, sessionElapsed, _, sessionStart := state.GetProgress()

	status := &types.DownloadStatus{
		ID:         id,
		URL:        ad.config.URL,
		Filename:   filename,
		TotalSize:  totalSize,
		Downloaded: downloaded,
		Status:     "downloading",
	}
	if dp := state.GetDestPath(); dp != "" {
		status.DestPath = dp
	} else {
		status.DestPath = ad.config.DestPath
	}

	if ad.config.State.IsPausing() {
		status.Status = "pausing"
	} else if ad.config.State.IsPaused() {
		status.Status = "paused"
	} else if state.Done.Load() {
		status.Status = "completed"
	}

	if err := state.GetError(); err != nil {
		status.Status = "error"
		status.Error = err.Error()
	}

	// Calculate progress
	if status.TotalSize > 0 {
		status.Progress = float64(status.Downloaded) * 100 / float64(status.TotalSize)
	}

	// Calculate speed (MB/s) only for active downloads.
	if status.Status == "downloading" {
		sessionDownloaded := downloaded - sessionStart
		if sessionElapsed.Seconds() > 0 && sessionDownloaded > 0 {
			bytesPerSec := float64(sessionDownloaded) / sessionElapsed.Seconds()
			status.Speed = bytesPerSec / float64(types.MB)
		}
	}

	return status
}

// trySendProgress sends msg on progressCh unless progressDone has been closed,
// preventing a panic from sending on a closed channel after shutdown.
func (p *WorkerPool) trySendProgress(msg any) {
	if p.progressCh == nil {
		return
	}
	select {
	case <-p.progressDone:
		return
	default:
	}
	select {
	case <-p.progressDone:
		return
	case p.progressCh <- msg:
	}
}

// GracefulShutdown pauses all downloads and waits for them to save state
func (p *WorkerPool) GracefulShutdown() {
	// Persist queued downloads first so they don't disappear on process shutdown.
	// These entries may not have started yet, so they do not have a .surge state snapshot.
	p.persistQueuedForShutdown()

	p.PauseAll()

	// Wait for any downloads in "Pausing" state to finish transitioning
	// This ensures we don't exit while a database write is pending/active
	ticker := time.NewTicker(gracefulShutdownPausePollInterval)
	defer ticker.Stop()
	start := time.Now()
	warned := false

	for {
		p.mu.RLock()
		stillPausing := false
		for _, ad := range p.downloads {
			if ad.config.State != nil && ad.config.State.IsPausing() {
				// If no worker is running this download anymore, pausing is stale.
				// Normalize it so shutdown can proceed.
				if !ad.running.Load() {
					ad.config.State.SetPausing(false)
					continue
				}
				stillPausing = true
				break
			}
		}
		p.mu.RUnlock()

		if !stillPausing {
			break
		}

		if !warned && time.Since(start) >= gracefulShutdownPauseSoftTimeout {
			utils.Debug("GracefulShutdown: downloads still pausing after %v, continuing to wait for durable pause", gracefulShutdownPauseSoftTimeout)
			warned = true
		}
		if time.Since(start) >= gracefulShutdownPauseHardTimeout {
			utils.Debug("GracefulShutdown: forcing exit from pausing wait after hard timeout %v", gracefulShutdownPauseHardTimeout)
			break
		}
		<-ticker.C
	}

	p.wg.Wait() // Blocks until all workers call Done()

	// Signal that progressCh must no longer be sent to, then close taskChan
	// so worker goroutines exit their range loop.
	close(p.progressDone)
	close(p.taskChan)
}

func (p *WorkerPool) persistQueuedForShutdown() {
	// No-op: queued items are persisted when Add emits DownloadQueuedMsg,
	// so shutdown only needs to drain active workers.
}
