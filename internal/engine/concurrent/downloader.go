package concurrent

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/engine"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// ConcurrentDownloader handles multi-connection downloads
type ConcurrentDownloader struct {
	ProgressChan chan<- any           // Channel for events (start/complete/error)
	ID           string               // Download ID
	State        *types.ProgressState // Shared state for TUI polling
	activeTasks  map[int]*ActiveTask
	activeMu     sync.Mutex
	URL          string // For pause/resume
	DestPath     string // For pause/resume
	Runtime      *types.RuntimeConfig
	TotalSize    int64
	bufPool      sync.Pool
	Headers      map[string]string // Custom HTTP headers from browser (cookies, auth, etc.)
}

// NewConcurrentDownloader creates a new concurrent downloader with all required parameters
func NewConcurrentDownloader(id string, progressCh chan<- any, progState *types.ProgressState, runtime *types.RuntimeConfig) *ConcurrentDownloader {
	if runtime == nil {
		runtime = &types.RuntimeConfig{
			MaxConnectionsPerHost: types.PerHostMax,
			MinChunkSize:          types.MinChunk,
			WorkerBufferSize:      types.WorkerBuffer,
			DialHedgeCount:        types.DialHedgeCount,
		}
	}

	return &ConcurrentDownloader{
		ID:           id,
		ProgressChan: progressCh,
		State:        progState,
		activeTasks:  make(map[int]*ActiveTask),
		Runtime:      runtime,
		bufPool: sync.Pool{
			New: func() any {
				// Use configured buffer size
				size := runtime.GetWorkerBufferSize()
				buf := make([]byte, size)
				return &buf
			},
		},
	}
}

// getInitialConnections returns the starting number of connections based on file size
func (d *ConcurrentDownloader) getInitialConnections(fileSize int64) int {
	maxConns := d.Runtime.GetMaxConnectionsPerHost()
	minChunkSize := d.Runtime.GetMinChunkSize() // e.g., 1MB or 5MB

	if fileSize <= 0 {
		return 1
	}

	// 1. Calculate ideal workers using the Square Root heuristic
	// Convert to float first to avoid integer truncation on small files
	sizeMB := float64(fileSize) / float64(types.MB)
	calculatedWorkers := int(math.Round(math.Sqrt(sizeMB)))

	// 2. Hard constraint: Don't create chunks smaller than MinChunkSize
	// If file is 20MB and MinChunk is 10MB, we strictly can't have more than 2 workers
	if minChunkSize > 0 {
		maxPossibleChunks := int(fileSize / minChunkSize)
		if maxPossibleChunks < 1 {
			maxPossibleChunks = 1
		}
		if calculatedWorkers > maxPossibleChunks {
			calculatedWorkers = maxPossibleChunks
		}
	}

	// 3. Safety Floors and Ceilings
	if calculatedWorkers < 1 {
		return 1
	}
	if calculatedWorkers > maxConns {
		return maxConns
	}

	return calculatedWorkers
}

// ReportMirrorError marks a mirror as having an error in the state
func (d *ConcurrentDownloader) ReportMirrorError(url string) {
	if d.State == nil {
		return
	}

	mirrors := d.State.GetMirrors()
	changed := false
	for i, m := range mirrors {
		if m.URL == url && !m.Error {
			mirrors[i].Error = true
			changed = true
			break
		}
	}

	if changed {
		d.State.SetMirrors(mirrors)
	}
}

// calculateChunkSize determines optimal chunk size
func (d *ConcurrentDownloader) calculateChunkSize(fileSize int64, numConns int) int64 {
	// Safety check
	if numConns <= 0 {
		return d.Runtime.GetMinChunkSize() // Fallback
	}

	chunkSize := fileSize / int64(numConns)

	// Clamp to min from config (but not max - we want large chunks)
	minChunk := d.Runtime.GetMinChunkSize()

	if chunkSize < minChunk {
		chunkSize = minChunk
	}

	// Align to 4KB
	chunkSize = (chunkSize / types.AlignSize) * types.AlignSize
	if chunkSize == 0 {
		chunkSize = types.AlignSize
	}

	return chunkSize
}

// determineChunkSize decides the strategy (Sequential vs Parallel)
func (d *ConcurrentDownloader) determineChunkSize(fileSize int64, numConns int) int64 {
	if d.Runtime.SequentialDownload {
		// Sequential mode: Use small fixed chunks (MinChunkSize) to ensure strict ordering
		chunkSize := d.Runtime.GetMinChunkSize()
		if chunkSize <= 0 {
			chunkSize = 2 * types.MB // Default 2MB if not configured
		}
		// Align to 4KB
		chunkSize = (chunkSize / types.AlignSize) * types.AlignSize
		if chunkSize == 0 {
			chunkSize = types.AlignSize
		}
		return chunkSize
	}

	// Parallel mode: Use large shards
	return d.calculateChunkSize(fileSize, numConns)
}

// createTasks generates initial task queue from file size and chunk size
func createTasks(fileSize, chunkSize int64) []types.Task {
	if chunkSize <= 0 {
		return nil
	}

	// preallocate slice capacity
	count := (fileSize + chunkSize - 1) / chunkSize
	tasks := make([]types.Task, 0, int(count))

	for offset := int64(0); offset < fileSize; offset += chunkSize {
		length := chunkSize
		if offset+length > fileSize {
			length = fileSize - offset
		}
		tasks = append(tasks, types.Task{Offset: offset, Length: length})
	}
	return tasks
}

func (d *ConcurrentDownloader) applyClientSettings(client *http.Client) {
	// Preserve headers on redirects for authenticated downloads
	// By default, Go strips sensitive headers (Cookie, Authorization) on cross-domain redirects.
	// Since these headers were explicitly provided by the browser for this download, we forward them.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		// Copy headers from original request to redirect request
		if len(via) > 0 {
			utils.CopyRedirectHeaders(req, via[0])
		}
		// Re-apply explicit custom headers down the redirect chain
		for key, val := range d.Headers {
			if key != "Range" {
				req.Header.Set(key, val)
			}
		}
		return nil
	}
}

// Download downloads a file using multiple concurrent connections
// Uses pre-probed metadata (file size already known)
func (d *ConcurrentDownloader) Download(ctx context.Context, rawurl string, candidateMirrors []string, activeMirrors []string, destPath string, fileSize int64) error {
	utils.Debug("ConcurrentDownloader.Download: %s -> %s (size: %d, mirrors: %d)", rawurl, destPath, fileSize, len(activeMirrors))

	// Store URL and path for pause/resume (final path without .surge)
	d.URL = rawurl
	d.DestPath = destPath

	// Initialize mirror status in state
	if d.State != nil {
		d.State.SetURL(rawurl)
		d.State.SetDestPath(destPath)

		var statuses []types.MirrorStatus
		// Add primary
		statuses = append(statuses, types.MirrorStatus{URL: rawurl, Active: true})

		// Add active mirrors (marked active)
		activeMap := make(map[string]bool)
		for _, m := range activeMirrors {
			activeMap[m] = true
			if m != rawurl {
				statuses = append(statuses, types.MirrorStatus{URL: m, Active: true})
			}
		}

		// Add inactive/failed mirrors (from candidate list that aren't active)
		for _, m := range candidateMirrors {
			if !activeMap[m] && m != rawurl {
				// Mark as Error since they failed probing (passed as candidates but not active)
				statuses = append(statuses, types.MirrorStatus{URL: m, Active: false, Error: true})
			}
		}

		d.State.SetMirrors(statuses)
	}

	// Working file has .surge suffix until download completes
	workingPath := destPath + types.IncompleteSuffix

	// Create cancellable context for pause support
	downloadCtx, cancel := context.WithCancel(ctx)

	// Helper synchronization
	var wgHelpers sync.WaitGroup
	// Ensure we wait for helpers to finish; run wait AFTER cancel (LIFO: cancel runs first)
	defer wgHelpers.Wait()
	defer cancel()

	if d.State != nil {
		d.State.SetCancelFunc(cancel)
	}

	// Acquire a single stable transport for the entire download session.
	var proxyURL, customDNS string
	if d.Runtime != nil {
		proxyURL = d.Runtime.ProxyURL
		customDNS = d.Runtime.CustomDNS
	}

	// Standardize on PoolMaxConnsPerHost for probes to match the eventual download path
	transport := engine.DefaultNetworkPool.AcquireTransport(proxyURL, customDNS, types.PoolMaxConnsPerHost)
	defer engine.DefaultNetworkPool.ReleaseTransport(transport)

	bootstrapClient := &http.Client{Transport: transport}
	d.applyClientSettings(bootstrapClient)

	// Determine connections and chunk size
	if fileSize <= 0 {
		discoveredSize, err := d.bootstrapMetadata(downloadCtx, bootstrapClient, rawurl)
		if err != nil {
			return err
		}
		fileSize = discoveredSize
	}
	d.TotalSize = fileSize

	numConns := d.getInitialConnections(fileSize)
	chunkSize := d.determineChunkSize(fileSize, numConns)

	// Create tuned HTTP client for concurrent downloads reusing the same transport
	client := &http.Client{Transport: transport}
	d.applyClientSettings(client)

	// Initialize chunk visualization
	if d.State != nil {
		d.State.InitBitmap(fileSize, chunkSize)
	}

	// Prepare list of targets for workers AND pre-warming
	var workerMirrors []string
	workerMirrors = append(workerMirrors, rawurl)
	for _, v := range activeMirrors {
		if v != rawurl {
			workerMirrors = append(workerMirrors, v)
		}
	}
	if len(workerMirrors) == 0 {
		workerMirrors = []string{rawurl}
	}

	// HEDGED DIALING: Pre-warm connections to bypass slow dials
	hedgeCount := d.Runtime.GetDialHedgeCount()
	if hedgeCount > 0 {
		utils.Debug("Pre-warming %d connections (hedge=%d)", numConns, hedgeCount)
		d.prewarmConnections(downloadCtx, client, numConns, hedgeCount, workerMirrors)
	}

	// Open existing output file with .surge suffix (must be created by processing layer)
	outFile, err := os.OpenFile(workingPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open working file: %w", err)
	}
	defer func() {
		if err := outFile.Close(); err != nil {
			utils.Debug("Error closing file: %v", err)
		}
	}()
	finalizeCompletedDownload := func() error {
		// Final sync
		if err := outFile.Sync(); err != nil {
			return fmt.Errorf("failed to sync file: %w", err)
		}

		// Close file before renaming
		_ = outFile.Close()

		return nil
	}

	tasks := createTasks(fileSize, chunkSize)

	// Check for saved state BEFORE truncating (resume case)
	savedState, err := state.LoadState(rawurl, destPath)
	isResume := err == nil && savedState != nil && len(savedState.Tasks) > 0

	if isResume {
		// Resume: use saved tasks and restore downloaded counter
		tasks = savedState.Tasks
		if d.State != nil {
			d.State.Downloaded.Store(savedState.Downloaded)
			d.State.VerifiedProgress.Store(savedState.Downloaded)
			// Restore elapsed time from previous sessions
			d.State.SetSavedElapsed(time.Duration(savedState.Elapsed))
			// Fix speed spike: sync session start so we don't count previous bytes as new speed
			d.State.SyncSessionStart()

			// RESTORE CHUNK BITMAP if available
			if len(savedState.ChunkBitmap) > 0 && savedState.ActualChunkSize > 0 {
				d.State.RestoreBitmap(savedState.ChunkBitmap, savedState.ActualChunkSize)

				// Reconstruct internal progress from remaining tasks to ensure partial chunks are handled correctly
				d.State.RecalculateProgress(savedState.Tasks)
				// Keep counters aligned after reconstruction to avoid session speed spikes.
				d.State.Downloaded.Store(d.State.VerifiedProgress.Load())
				d.State.SyncSessionStart()

				utils.Debug("Restored chunk map: size %d", savedState.ActualChunkSize)
			}
		}
		utils.Debug("Resuming from saved state: %d tasks, %d bytes downloaded", len(tasks), savedState.Downloaded)
	} else {
		// Fresh download: preallocate file and create new tasks
		if err := outFile.Truncate(fileSize); err != nil {
			return fmt.Errorf("failed to preallocate file: %w", err)
		}
		// Robustness: ensure state counter starts at 0 for fresh download
		if d.State != nil {
			d.State.Downloaded.Store(0)
			d.State.SyncSessionStart()
		}
	}
	queue := NewTaskQueue()
	queue.PushMultiple(tasks)

	// Start balancer goroutine for dynamic chunk splitting
	balancerCtx, cancelBalancer := context.WithCancel(downloadCtx)
	defer cancelBalancer()

	wgHelpers.Add(1)
	go func() {
		defer wgHelpers.Done()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-balancerCtx.Done():
				return
			case <-ticker.C:
				// Aggressively fill idle workers
				// Continue splitting/stealing as long as we have idle workers and are making progress
				for queue.IdleWorkers() > 0 {
					didWork := false
					if queue.Len() == 0 {
						// Try to steal from an active worker
						if d.StealWork(queue) {
							didWork = true
						}
					}

					// If stealing failed (chunks too small), try hedged request:
					// Duplicate a task so an idle worker races on a fresh connection
					if !didWork && queue.Len() == 0 {
						if d.HedgeWork(queue) {
							didWork = true
						}
					}

					// If we couldn't split, steal, or hedge anything, stop trying for this tick
					if !didWork {
						break
					}
				}
			}
		}
	}()

	// Monitor for completion
	wgHelpers.Add(1)
	go func() {
		defer wgHelpers.Done()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				queue.Close()
				return
			case <-balancerCtx.Done():
				queue.Close()
				return
			case <-ticker.C:
				// Ensure queue is empty (no pending retries) before considering byte count.
				// This protects against cutting off active retries even if byte count seems high (due to overlaps etc).
				if queue.Len() == 0 && (int(queue.IdleWorkers()) == numConns || d.State.Downloaded.Load() >= fileSize) {
					queue.Close()
					return
				}
			}
		}
	}()

	// Health monitor: detect slow workers
	wgHelpers.Add(1)
	go func() {
		defer wgHelpers.Done()
		ticker := time.NewTicker(types.HealthCheckInterval) // Fixed: using types constant
		defer ticker.Stop()

		for {
			select {
			case <-balancerCtx.Done():
				return
			case <-ticker.C:
				d.checkWorkerHealth()
			}
		}
	}()

	// Start workers
	var wg sync.WaitGroup
	workerErrors := make(chan error, numConns)

	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			err := d.worker(downloadCtx, workerID, workerMirrors, outFile, queue, fileSize, client)
			if err != nil && err != context.Canceled {
				workerErrors <- err
			}
		}(i)
	}

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(workerErrors)
		queue.Close()
	}()

	// Check for errors or pause
	var downloadErr error
	for err := range workerErrors {
		if err != nil {
			downloadErr = err
		}
	}

	// Handle pause: state saved
	if d.State != nil && d.State.IsPaused() {
		// 1. Collect active tasks as remaining work FIRST
		var activeRemaining []types.Task
		d.activeMu.Lock()
		for _, active := range d.activeTasks {
			if remaining := active.RemainingTask(); remaining != nil {
				activeRemaining = append(activeRemaining, *remaining)
			}
		}
		d.activeMu.Unlock()

		// 2. Collect remaining tasks from queue
		remainingTasks := queue.DrainRemaining()
		remainingTasks = append(remainingTasks, activeRemaining...)

		// Calculate Downloaded from remaining tasks (ensures consistency)
		var remainingBytes int64
		for _, task := range remainingTasks {
			remainingBytes += task.Length
		}
		if remainingBytes == 0 {
			utils.Debug("Download pause requested at completion boundary; finalizing as completed")
			d.State.Resume()
			_, _ = d.State.FinalizeSession(fileSize)
			return finalizeCompletedDownload()
		}
		computedDownloaded := fileSize - remainingBytes

		// Calculate total elapsed time
		totalElapsed := d.State.FinalizePauseSession(computedDownloaded)
		var chunkBitmap []byte
		var actualChunkSize int64

		// Get persisted bitmap data
		bitmap, _, _, chunkSize, _ := d.State.GetBitmapSnapshot(false)
		chunkBitmap = bitmap
		actualChunkSize = chunkSize

		// Save state for resume (use computed value for consistency)
		s := &types.DownloadState{
			URL:             d.URL,
			ID:              d.ID,
			DestPath:        destPath,
			TotalSize:       fileSize,
			Downloaded:      computedDownloaded,
			Tasks:           remainingTasks,
			Filename:        filepath.Base(destPath),
			Elapsed:         totalElapsed.Nanoseconds(),
			Mirrors:         candidateMirrors,
			ChunkBitmap:     chunkBitmap,
			ActualChunkSize: actualChunkSize,
		}
		if d.ProgressChan != nil {
			d.ProgressChan <- events.DownloadPausedMsg{
				DownloadID: d.ID,
				Filename:   filepath.Base(destPath),
				Downloaded: computedDownloaded,
				State:      s,
			}
		}

		utils.Debug("Download paused, state saved (Downloaded=%d, RemainingTasks=%d, RemainingBytes=%d)",
			computedDownloaded, len(remainingTasks), remainingBytes)
		return types.ErrPaused // Signal valid pause to caller
	}

	// Handle cancel: context was cancelled but not via Pause()
	// Propagate cancellation so callers don't treat this as a successful completion.
	if downloadCtx.Err() == context.Canceled {
		return context.Canceled
	}

	if downloadErr != nil {
		return downloadErr
	}

	// Note: Download completion notifications are handled by the TUI via DownloadCompleteMsg
	return finalizeCompletedDownload()
}

func (d *ConcurrentDownloader) bootstrapMetadata(ctx context.Context, client *http.Client, rawurl string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create concurrent bootstrap request: %w", err)
	}

	for key, val := range d.Headers {
		if key != "Range" {
			req.Header.Set(key, val)
		}
	}
	req.Header.Set("User-Agent", d.Runtime.GetUserAgent())
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to bootstrap concurrent download: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*types.KB))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("concurrent bootstrap requires 206 response, got %d", resp.StatusCode)
	}

	contentRange := resp.Header.Get("Content-Range")
	if contentRange == "" {
		return 0, fmt.Errorf("concurrent bootstrap missing Content-Range header")
	}
	idx := strings.LastIndex(contentRange, "/")
	if idx == -1 || idx+1 >= len(contentRange) {
		return 0, fmt.Errorf("concurrent bootstrap invalid Content-Range header: %q", contentRange)
	}

	sizeStr := contentRange[idx+1:]
	if sizeStr == "*" {
		return 0, fmt.Errorf("concurrent bootstrap returned unknown size")
	}

	fileSize, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil || fileSize <= 0 {
		return 0, fmt.Errorf("concurrent bootstrap invalid file size %q", sizeStr)
	}

	if d.State != nil {
		d.State.SetTotalSize(fileSize)
	}

	return fileSize, nil
}

// prewarmConnections fires off concurrent pings to the mirrors to populate the connection pool
func (d *ConcurrentDownloader) prewarmConnections(ctx context.Context, client *http.Client, numRequired, hedgeCount int, mirrors []string) {
	totalToStart := numRequired + hedgeCount
	if totalToStart > 128 { // Safety cap
		totalToStart = 128
	}

	// Channel to signal when a connection is ready (handshake complete)
	ready := make(chan struct{}, totalToStart)

	// Create a sub-context for the pings so we can stop them once we have enough
	pingCtx, cancelPings := context.WithCancel(ctx)
	defer cancelPings()

	for i := 0; i < totalToStart; i++ {
		go func(idx int) {

			// Round-robin mirrors
			mirror := mirrors[idx%len(mirrors)]

			// Use a fast Range request to ensure the handshake completes
			req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, mirror, nil)
			if err != nil {
				return
			}

			// Forward custom headers (essential for authenticated mirrors)
			for key, val := range d.Headers {
				if key != "Range" {
					req.Header.Set(key, val)
				}
			}

			// Ensure User-Agent and Range are set
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", d.Runtime.GetUserAgent())
			}
			req.Header.Set("Range", "bytes=0-0")

			// Perform dial + request
			resp, err := client.Do(req)
			if err != nil {
				return
			}

			// Drain body and close to return connection to idle pool, then signal readiness.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			ready <- struct{}{}
		}(i)
	}

	// Wait until we have enough ready connections OR we hit a timeout
	completed := 0
	timeout := time.After(types.DialTimeout) // Use standard dial timeout for the whole batch

	for completed < numRequired {
		select {
		case <-ready:
			completed++
		case <-timeout:
			utils.Debug("Pre-warming timed out after %d/%d connections", completed, numRequired)
			return
		case <-ctx.Done():
			return
		}
	}

	utils.Debug("Pre-warming complete: %d connections hot", completed)
	// Remaining pings will be cancelled by defer cancelPings()
}
