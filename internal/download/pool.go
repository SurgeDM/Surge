package download

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SurgeDM/Surge/internal/engine"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// activeDownload tracks a download that's currently running
type activeDownload struct {
	config types.DownloadConfig
	cancel context.CancelFunc
	// running is true while the worker goroutine is executing TUIDownload for this config.
	running atomic.Bool
}

// WorkerPool manages the download workers and tasks.
//
// Lock Ordering:
// If a code path must acquire both the pool's main mutex (p.mu) and a limiter's internal
// mutex (limiter.mu, via limiter.SetRate() or limiter.WaitN()), it MUST acquire p.mu first.
// Examples: Add, SetDownloadRateLimit, SetDefaultDownloadRateLimit.
// Never acquire p.mu while holding a limiter's internal mutex to prevent deadlocks.
type WorkerPool struct {
	taskChan     chan string
	progressCh   chan<- any
	progressDone chan struct{}                   // closed when progressCh must no longer be sent to
	downloads    map[string]*activeDownload      // Track active downloads for pause/resume
	queued       map[string]types.DownloadConfig // Track queued downloads
	mu           sync.RWMutex
	wg           sync.WaitGroup // We use this to wait for all active downloads to pause before exiting the program
	maxDownloads int

	globalLimiter               *engine.RateLimiter
	downloadLimiters            map[string]*engine.RateLimiter
	defaultDownloadRateLimitBps int64
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
		taskChan:         make(chan string, 100), // We make it buffered to avoid blocking add
		progressCh:       progressCh,
		progressDone:     make(chan struct{}),
		downloads:        make(map[string]*activeDownload),
		queued:           make(map[string]types.DownloadConfig),
		maxDownloads:     maxDownloads,
		globalLimiter:    engine.NewRateLimiter(0, 0),
		downloadLimiters: make(map[string]*engine.RateLimiter),
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
	if _, totalSize, _, _, _, _ := cfg.State.GetProgress(); totalSize > 0 {
		cfg.TotalSize = totalSize
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

// Add adds a new download task to the pool. The caller (LifecycleManager) is
// responsible for emitting any lifecycle events (e.g. DownloadQueuedMsg).
func (p *WorkerPool) Add(cfg types.DownloadConfig) {
	if cfg.ProgressCh == nil {
		cfg.ProgressCh = p.progressCh
	}
	p.mu.Lock()
	p.ensureLimiterForConfigLocked(&cfg)
	p.queued[cfg.ID] = cfg
	p.wg.Add(1)
	p.mu.Unlock()

	p.taskChan <- cfg.ID
}

func (p *WorkerPool) ensureLimiterForConfigLocked(cfg *types.DownloadConfig) {
	if cfg == nil || cfg.ID == "" {
		return
	}

	if p.globalLimiter == nil {
		p.globalLimiter = engine.NewRateLimiter(0, 0)
	}
	if p.downloadLimiters == nil {
		p.downloadLimiters = make(map[string]*engine.RateLimiter)
	}

	// If state already carries an explicit rate, prefer it over cfg default.
	if cfg.State != nil {
		if stateRate, stateSet := cfg.State.GetRateLimit(); stateSet {
			cfg.RateLimitBps = stateRate
			cfg.RateLimitSet = true
		}
	}

	rate := cfg.RateLimitBps
	if !cfg.RateLimitSet {
		rate = p.defaultDownloadRateLimitBps
		cfg.RateLimitBps = rate
	}
	if cfg.State != nil {
		cfg.State.SetRateLimit(rate, cfg.RateLimitSet)
	}

	limiter := p.downloadLimiters[cfg.ID]
	if limiter == nil {
		limiter = engine.NewRateLimiter(rate, rateLimiterBurst(rate))
		p.downloadLimiters[cfg.ID] = limiter
	} else {
		limiter.SetRate(rate, rateLimiterBurst(rate))
	}

	if cfg.Limiter == nil {
		cfg.Limiter = engine.NewMultiLimiter(p.globalLimiter, limiter)
	}
}

func rateLimiterBurst(rate int64) int64 {
	if rate <= 0 {
		return 0
	}
	return rate
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

	configs := make([]types.DownloadConfig, 0, len(p.downloads)+len(p.queued))
	for _, ad := range p.downloads {
		cfg := ad.config
		cfg.Limiter = nil
		syncConfigFromState(&cfg)
		configs = append(configs, cfg)
	}
	for _, cfg := range p.queued {
		cfg.Limiter = nil
		configs = append(configs, cfg)
	}
	return configs
}

// Pause pauses a specific download by ID. Returns true if found and pause initiated
// (or already paused), false otherwise. Pure mechanical operation - no events emitted.
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

// SetGlobalRateLimit updates the global rate limiter (bytes/sec). Use 0 to disable.
func (p *WorkerPool) SetGlobalRateLimit(rate int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.globalLimiter == nil {
		p.globalLimiter = engine.NewRateLimiter(0, 0)
	}
	// All per-download MultiLimiters hold a pointer to this globalLimiter,
	// so updating the rate here propagates to all active downloads instantly.
	p.globalLimiter.SetRate(rate, rateLimiterBurst(rate))
}

// SetDefaultDownloadRateLimit updates the default per-download rate limit (bytes/sec).
func (p *WorkerPool) SetDefaultDownloadRateLimit(rate int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.defaultDownloadRateLimitBps = rate
	if p.downloadLimiters == nil {
		p.downloadLimiters = make(map[string]*engine.RateLimiter)
	}

	for id, cfg := range p.queued {
		if cfg.RateLimitSet {
			continue
		}
		cfg.RateLimitBps = rate
		p.queued[id] = cfg
		if cfg.State != nil {
			cfg.State.SetRateLimit(rate, false)
		}
		limiter := p.downloadLimiters[id]
		if limiter == nil {
			// Note: ensureLimiterForConfigLocked guarantees all active/queued downloads have a limiter.
			// This nil branch is defensive and should be unreachable in practice.
			p.downloadLimiters[id] = engine.NewRateLimiter(rate, rateLimiterBurst(rate))
		} else {
			limiter.SetRate(rate, rateLimiterBurst(rate))
		}
	}
	for id, ad := range p.downloads {
		if ad.config.RateLimitSet {
			continue
		}
		ad.config.RateLimitBps = rate
		if ad.config.State != nil {
			ad.config.State.SetRateLimit(rate, false)
		}
		limiter := p.downloadLimiters[id]
		if limiter == nil {
			// Note: ensureLimiterForConfigLocked guarantees all active/queued downloads have a limiter.
			// This nil branch is defensive and should be unreachable in practice.
			p.downloadLimiters[id] = engine.NewRateLimiter(rate, rateLimiterBurst(rate))
		} else {
			limiter.SetRate(rate, rateLimiterBurst(rate))
		}
	}
}

// SetDownloadRateLimit updates a specific download's rate limit (bytes/sec).
func (p *WorkerPool) SetDownloadRateLimit(downloadID string, rate int64) bool {
	if downloadID == "" {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	found := false
	if ad, ok := p.downloads[downloadID]; ok {
		ad.config.RateLimitBps = rate
		ad.config.RateLimitSet = true
		if ad.config.State != nil {
			ad.config.State.SetRateLimit(rate, true)
		}
		found = true
	}
	if cfg, ok := p.queued[downloadID]; ok {
		cfg.RateLimitBps = rate
		cfg.RateLimitSet = true
		if cfg.State != nil {
			cfg.State.SetRateLimit(rate, true)
		}
		p.queued[downloadID] = cfg
		found = true
	}

	if !found {
		return false
	}

	if p.downloadLimiters == nil {
		p.downloadLimiters = make(map[string]*engine.RateLimiter)
	}
	limiter := p.downloadLimiters[downloadID]
	if limiter == nil {
		p.downloadLimiters[downloadID] = engine.NewRateLimiter(rate, rateLimiterBurst(rate))
	} else {
		limiter.SetRate(rate, rateLimiterBurst(rate))
	}
	return true
}

// ClearDownloadRateLimit removes a specific download's override so it inherits the current default.
func (p *WorkerPool) ClearDownloadRateLimit(downloadID string) bool {
	if downloadID == "" {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	defaultRate := p.defaultDownloadRateLimitBps

	found := false
	if ad, ok := p.downloads[downloadID]; ok {
		ad.config.RateLimitBps = defaultRate
		ad.config.RateLimitSet = false
		if ad.config.State != nil {
			ad.config.State.SetRateLimit(defaultRate, false)
		}
		found = true
	}
	if cfg, ok := p.queued[downloadID]; ok {
		cfg.RateLimitBps = defaultRate
		cfg.RateLimitSet = false
		if cfg.State != nil {
			cfg.State.SetRateLimit(defaultRate, false)
		}
		p.queued[downloadID] = cfg
		found = true
	}

	if !found {
		return false
	}

	if p.downloadLimiters == nil {
		p.downloadLimiters = make(map[string]*engine.RateLimiter)
	}
	limiter := p.downloadLimiters[downloadID]
	if limiter == nil {
		p.downloadLimiters[downloadID] = engine.NewRateLimiter(defaultRate, rateLimiterBurst(defaultRate))
	} else {
		limiter.SetRate(defaultRate, rateLimiterBurst(defaultRate))
	}
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

// Cancel cancels and removes a download by ID. Returns metadata about what was
// removed so the caller (LifecycleManager) can emit events and handle cleanup.
// No events are emitted by the pool itself.
func (p *WorkerPool) Cancel(downloadID string) types.CancelResult {
	p.mu.Lock()
	ad, activeExists := p.downloads[downloadID]
	qCfg, queuedExists := p.queued[downloadID]
	if activeExists {
		delete(p.downloads, downloadID)
	}
	if queuedExists {
		delete(p.queued, downloadID)
	}
	if activeExists || queuedExists {
		delete(p.downloadLimiters, downloadID)
	}
	p.mu.Unlock()

	if !activeExists && !queuedExists {
		return types.CancelResult{}
	}

	result := types.CancelResult{Found: true, WasQueued: queuedExists && !activeExists}

	if activeExists && ad != nil {
		result.Filename = ad.config.Filename
		result.DestPath = resolveDestPath(&ad.config)
		result.Completed = ad.config.State != nil && ad.config.State.Done.Load()

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
		result.Filename = qCfg.Filename
		result.DestPath = resolveDestPath(&qCfg)
	}

	return result
}

// ExtractPausedConfig atomically removes a paused download from the pool and returns
// its config (with state cleared for re-enqueue) so the LifecycleManager can resume it.
// Returns nil if the download is not found, not paused, or still transitioning (pausing).
func (p *WorkerPool) ExtractPausedConfig(downloadID string) *types.DownloadConfig {
	p.mu.Lock()
	ad, exists := p.downloads[downloadID]
	if !exists || ad == nil {
		p.mu.Unlock()
		return nil
	}

	// Cannot extract if still pausing or not actually paused
	if ad.config.State == nil || !ad.config.State.IsPaused() || ad.config.State.IsPausing() {
		p.mu.Unlock()
		return nil
	}

	// Sync latest filename/path/mirrors from live state before handing off
	syncConfigFromState(&ad.config)

	cfg := ad.config
	delete(p.downloads, downloadID)
	delete(p.downloadLimiters, downloadID)
	p.mu.Unlock()

	cfg.Limiter = nil
	if cfg.State != nil {
		cfg.State.Resume()
	}
	return &cfg
}

// UpdateURL updates the in-memory URL of a download by ID.
// The caller (LifecycleManager) is responsible for persisting the change to the DB.
// It fails if the download is actively downloading (not paused or errored).
func (p *WorkerPool) UpdateURL(downloadID string, newURL string) error {
	p.mu.Lock()
	ad, exists := p.downloads[downloadID]
	_, qExists := p.queued[downloadID]

	if qExists {
		p.mu.Unlock()
		return types.ErrQueuedUpdate
	}

	if exists && ad != nil {
		if ad.config.State != nil && !ad.config.State.IsPaused() {
			if ad.running.Load() {
				p.mu.Unlock()
				return types.ErrActiveUpdate
			}
		}
		ad.config.URL = newURL
		if ad.config.State != nil {
			ad.config.State.SetURL(newURL)
		}
	}
	p.mu.Unlock()

	return nil
}

func (p *WorkerPool) worker() {
	for id := range p.taskChan {
		p.mu.RLock()
		cfg, stillQueued := p.queued[id]
		p.mu.RUnlock()
		if !stillQueued {
			// Canceled while waiting in queue.
			p.wg.Done()
			continue
		}

		// Create cancellable context
		ctx, cancel := context.WithCancel(context.Background())

		// Ensure Runtime is initialized before exposing to GetAll
		if cfg.Runtime == nil {
			cfg.Runtime = types.DefaultRuntimeConfig()
		}

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
		cfg, stillQueued = p.queued[id]
		if !stillQueued {
			p.mu.Unlock()
			cancel()
			p.wg.Done()
			continue
		}
		ad.config = cfg // Ensure ad.config has the latest state from queue
		delete(p.queued, cfg.ID)
		p.downloads[cfg.ID] = ad

		// Make a local copy for TUIDownload to mutate safely
		localCfg := ad.config
		p.mu.Unlock()

		err := TUIDownload(ctx, &localCfg)
		ad.running.Store(false)

		// Sync back mutated fields cleanly under lock
		p.mu.Lock()
		if _, exists := p.downloads[localCfg.ID]; exists {
			ad.config.TotalSize = localCfg.TotalSize
			ad.config.Runtime = localCfg.Runtime
		}
		p.mu.Unlock()

		// Logic:
		// 1. If Pause() was called: State.IsPaused() is true. We keep the task in p.downloads (so it can be resumed).
		// 2. If finished/error: We remove from p.downloads.

		isPaused := localCfg.State != nil && localCfg.State.IsPaused()

		// Clear "Pausing" transition state now that worker has exited
		if localCfg.State != nil {
			localCfg.State.SetPausing(false)
		}

		if isPaused {
			utils.Debug("WorkerPool: Download %s paused cleanly", localCfg.ID)
			// If paused, we keep it in downloads map for potential resume via ExtractPausedConfig
		} else if err != nil {
			if localCfg.State != nil {
				localCfg.State.SetError(err)
			}
			// Note: DownloadErrorMsg is already emitted by TUIDownload on the same progressCh.
			// Clean up errored download from tracking (don't save to .surge)
			p.mu.Lock()
			delete(p.downloads, localCfg.ID)
			delete(p.downloadLimiters, localCfg.ID)
			p.mu.Unlock()

		} else {
			// Only mark as done if not paused
			if localCfg.State != nil {
				localCfg.State.Done.Store(true)
			}
			// Note: DownloadCompleteMsg is sent by the progress reporter when it detects Done=true

			// Clean up from tracking
			p.mu.Lock()
			delete(p.downloads, localCfg.ID)
			delete(p.downloadLimiters, localCfg.ID)
			p.mu.Unlock()
		}
		// If paused, we keep it in downloads map for potential resume
		p.wg.Done()
	}
}

// GetStatus returns the status of an active download
func (p *WorkerPool) GetStatus(id string) *types.DownloadStatus {
	var adURL, adFilename, adDestPath string
	var adRateLimitBps int64
	var adRateLimitSet bool
	var adState *types.ProgressState

	p.mu.RLock()
	ad, exists := p.downloads[id]
	qCfg, qExists := p.queued[id]
	if exists {
		adURL = ad.config.URL
		adFilename = ad.config.Filename
		adDestPath = ad.config.DestPath
		adRateLimitBps = ad.config.RateLimitBps
		adRateLimitSet = ad.config.RateLimitSet
		adState = ad.config.State
	}
	p.mu.RUnlock()

	if !exists && !qExists {
		return nil
	}

	if qExists {
		return &types.DownloadStatus{
			ID:           id,
			URL:          qCfg.URL,
			Filename:     qCfg.Filename,
			DestPath:     resolveDestPath(&qCfg),
			Status:       "queued",
			Downloaded:   0,
			TotalSize:    0, // Metadata not yet fetched
			RateLimit:    qCfg.RateLimitBps,
			RateLimitSet: qCfg.RateLimitSet,
		}
	}

	state := adState
	if state == nil {
		return nil
	}

	// Use state filename/destpath if available (thread-safe)
	filename := adFilename
	if str := state.GetFilename(); str != "" {
		filename = str
	}

	// Calculate progress and speed (thread-safe)
	downloaded, totalSize, _, sessionElapsed, _, sessionStart := state.GetProgress()

	status := &types.DownloadStatus{
		ID:           id,
		URL:          adURL,
		Filename:     filename,
		TotalSize:    totalSize,
		Downloaded:   downloaded,
		Status:       "downloading",
		RateLimit:    adRateLimitBps,
		RateLimitSet: adRateLimitSet,
	}
	if dp := state.GetDestPath(); dp != "" {
		status.DestPath = dp
	} else {
		status.DestPath = adDestPath
	}

	if state.IsPausing() {
		status.Status = "pausing"
	} else if state.IsPaused() {
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

// GracefulShutdown pauses all downloads and waits for them to save state
func (p *WorkerPool) GracefulShutdown() {
	p.PauseAll()

	// Discard all queued-but-not-yet-started downloads so that idle workers
	// do not pick them up and begin downloading after shutdown is initiated.
	// Workers already guard against this with the p.queued check at loop entry,
	// so clearing the map here is sufficient; draining taskChan is belt-and-suspenders.
	p.mu.Lock()
	for id := range p.queued {
		delete(p.queued, id)
	}
	p.mu.Unlock()

	// Drain taskChan to discard any configs that were already written into the
	// buffered channel but not yet consumed by a worker.
drainLoop:
	for {
		select {
		case <-p.taskChan:
			p.wg.Done()
		default:
			break drainLoop
		}
	}

	// Wait for any downloads in "Pausing" state to finish transitioning
	// This ensures we don't exit while a database write is pending/active
	ticker := time.NewTicker(gracefulShutdownPausePollInterval)
	defer ticker.Stop()
	start := time.Now()
	warned := false

	for {
		p.mu.Lock()
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
		p.mu.Unlock()

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
