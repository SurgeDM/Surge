package concurrent

import (
	"context"
	"errors"
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
			return types.ErrMaxRedirects
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

	d.initMirrorStatus(rawurl, candidateMirrors, activeMirrors, destPath)

	workingPath := destPath + types.IncompleteSuffix
	downloadCtx, cancel := context.WithCancel(ctx)

	if d.State != nil {
		d.State.SetCancelFunc(cancel)
	}

	client, transport := d.setupNetwork()
	// Release transport back to the pool ONLY after all helpers and workers are joined (LIFO: runs last)
	defer engine.DefaultNetworkPool.ReleaseTransport(transport)

	// Helper synchronization for monitors and balancer
	var wgHelpers sync.WaitGroup
	// Ensure we wait for helpers to finish; run wait AFTER cancel (LIFO: Wait runs second, cancel runs first)
	defer wgHelpers.Wait()
	defer cancel()

	// Ensure we have the total file size
	if fileSize <= 0 {
		var err error
		fileSize, err = d.bootstrapMetadata(downloadCtx, client, rawurl)
		if err != nil {
			return err
		}
	}
	d.TotalSize = fileSize

	numConns := d.getInitialConnections(fileSize)
	chunkSize := d.determineChunkSize(fileSize, numConns)

	workerMirrors := d.getWorkerMirrors(activeMirrors)

	// Pre-warm connections if configured
	hedgeCount := d.Runtime.GetDialHedgeCount()
	if hedgeCount > 0 {
		d.prewarmConnections(downloadCtx, client, numConns, hedgeCount, workerMirrors)
	}

	// Open existing output file with .surge suffix (must be created by processing layer)
	outFile, err := os.OpenFile(workingPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open working file: %w", err)
	}
	defer func() {
		if outFile != nil {
			_ = outFile.Close()
		}
	}()

	// Initialize chunk visualization (must happen BEFORE setupTasks so RestoreBitmap can overwrite it)
	if d.State != nil {
		d.State.InitBitmap(fileSize, chunkSize)
	}

	tasks, err := d.setupTasks(destPath, fileSize, chunkSize, outFile)
	if err != nil {
		return err
	}

	queue := NewTaskQueue()
	queue.PushMultiple(tasks)

	// Start monitoring and balancing helpers
	d.startHelpers(downloadCtx, &wgHelpers, queue, fileSize, numConns)

	// Execute download workers
	downloadErr := d.executeWorkers(downloadCtx, client, outFile, queue, fileSize, workerMirrors, numConns)

	// Handle pause request: must return types.ErrPaused to prevent finalization
	if d.State != nil && d.State.IsPaused() {
		pauseErr := d.handlePause(destPath, fileSize, queue, candidateMirrors)
		if pauseErr == nil {
			// Pause was requested at completion boundary, so handlePause finalized it.
			return d.syncFile(outFile)
		}
		return pauseErr
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
	return d.syncFile(outFile)
}

func (d *ConcurrentDownloader) initMirrorStatus(rawurl string, candidateMirrors []string, activeMirrors []string, destPath string) {
	d.URL = rawurl
	d.DestPath = destPath

	if d.State == nil {
		return
	}

	d.State.SetURL(rawurl)
	d.State.SetDestPath(destPath)

	var statuses []types.MirrorStatus
	statuses = append(statuses, types.MirrorStatus{URL: rawurl, Active: true})

	activeMap := make(map[string]bool)
	for _, m := range activeMirrors {
		activeMap[m] = true
		if m != rawurl {
			statuses = append(statuses, types.MirrorStatus{URL: m, Active: true})
		}
	}

	for _, m := range candidateMirrors {
		if !activeMap[m] && m != rawurl {
			statuses = append(statuses, types.MirrorStatus{URL: m, Active: false, Error: true})
		}
	}

	d.State.SetMirrors(statuses)
}

func (d *ConcurrentDownloader) setupNetwork() (*http.Client, *http.Transport) {
	var proxyURL, customDNS string
	if d.Runtime != nil {
		proxyURL = d.Runtime.ProxyURL
		customDNS = d.Runtime.CustomDNS
	}

	transport := engine.DefaultNetworkPool.AcquireTransport(proxyURL, customDNS, types.PoolMaxConnsPerHost)
	client := &http.Client{Transport: transport}
	d.applyClientSettings(client)
	return client, transport
}

func (d *ConcurrentDownloader) getWorkerMirrors(activeMirrors []string) []string {
	mirrors := make([]string, 0, len(activeMirrors)+1)
	mirrors = append(mirrors, d.URL)
	for _, v := range activeMirrors {
		if v != d.URL {
			mirrors = append(mirrors, v)
		}
	}
	return mirrors
}

func (d *ConcurrentDownloader) setupTasks(destPath string, fileSize, chunkSize int64, outFile *os.File) ([]types.Task, error) {
	savedState, err := state.LoadState(d.URL, destPath)
	isResume := err == nil && savedState != nil && len(savedState.Tasks) > 0

	if isResume {
		if d.State != nil {
			d.State.Downloaded.Store(savedState.Downloaded)
			d.State.VerifiedProgress.Store(savedState.Downloaded)
			d.State.SetSavedElapsed(time.Duration(savedState.Elapsed))
			d.State.SyncSessionStart()

			if len(savedState.ChunkBitmap) > 0 && savedState.ActualChunkSize > 0 {
				d.State.RestoreBitmap(savedState.ChunkBitmap, savedState.ActualChunkSize)
				d.State.RecalculateProgress(savedState.Tasks)
				d.State.Downloaded.Store(d.State.VerifiedProgress.Load())
				d.State.SyncSessionStart()
				utils.Debug("Restored chunk map: size %d", savedState.ActualChunkSize)
			}
		}
		utils.Debug("Resuming from saved state: %d tasks, %d bytes downloaded", len(savedState.Tasks), savedState.Downloaded)
		return savedState.Tasks, nil
	}

	if err := outFile.Truncate(fileSize); err != nil {
		return nil, fmt.Errorf("failed to preallocate file: %w", err)
	}
	if d.State != nil {
		d.State.Downloaded.Store(0)
		d.State.SyncSessionStart()
	}
	return createTasks(fileSize, chunkSize), nil
}

func (d *ConcurrentDownloader) startHelpers(ctx context.Context, wg *sync.WaitGroup, queue *TaskQueue, fileSize int64, numConns int) {
	// Balancer for dynamic chunk splitting and work stealing
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.runBalancer(ctx, queue)
	}()

	// Monitor for download completion
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.runCompletionMonitor(ctx, queue, fileSize, numConns)
	}()

	// Health monitor for detecting slow workers
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.runHealthMonitor(ctx)
	}()
}

func (d *ConcurrentDownloader) runBalancer(ctx context.Context, queue *TaskQueue) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for queue.IdleWorkers() > 0 {
				didWork := false
				if queue.Len() == 0 {
					if d.StealWork(queue) {
						didWork = true
					}
				}
				if !didWork && queue.Len() == 0 {
					if d.HedgeWork(queue) {
						didWork = true
					}
				}
				if !didWork {
					break
				}
			}
		}
	}
}

func (d *ConcurrentDownloader) runCompletionMonitor(ctx context.Context, queue *TaskQueue, fileSize int64, numConns int) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			queue.Close()
			return
		case <-ticker.C:
			// Completion condition:
			// 1. Queue is empty (no pending retries)
			// AND
			// 2. All workers are idle OR we've accounted for all bytes
			// Ensure queue is empty (no pending retries) before considering byte count.
			// This protects against cutting off active retries even if byte count seems high (due to overlaps etc).
			isDone := queue.Len() == 0 && (int(queue.IdleWorkers()) == numConns || (d.State != nil && d.State.Downloaded.Load() >= fileSize))
			if isDone {
				queue.Close()
				return
			}
		}
	}
}

func (d *ConcurrentDownloader) runHealthMonitor(ctx context.Context) {
	ticker := time.NewTicker(types.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.checkWorkerHealth()
		}
	}
}

func (d *ConcurrentDownloader) executeWorkers(ctx context.Context, client *http.Client, outFile *os.File, queue *TaskQueue, fileSize int64, workerMirrors []string, numConns int) error {
	var wg sync.WaitGroup
	workerErrors := make(chan error, numConns)

	// Start workers
	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			err := d.worker(ctx, workerID, workerMirrors, outFile, queue, fileSize, client)
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
	seenErrors := make(map[string]bool)
	for err := range workerErrors {
		if err != nil {
			errStr := err.Error()
			if !seenErrors[errStr] {
				downloadErr = errors.Join(downloadErr, err)
				seenErrors[errStr] = true
			}
		}
	}
	return downloadErr
}

func (d *ConcurrentDownloader) handlePause(destPath string, fileSize int64, queue *TaskQueue, candidateMirrors []string) error {
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
		return nil
	}
	computedDownloaded := fileSize - remainingBytes

	// Calculate total elapsed time
	totalElapsed := d.State.FinalizePauseSession(computedDownloaded)

	// Get persisted bitmap data
	bitmap, _, _, chunkSize, _ := d.State.GetBitmapSnapshot(false)

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
		ChunkBitmap:     bitmap,
		ActualChunkSize: chunkSize,
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
	return types.ErrPaused
}

func (d *ConcurrentDownloader) syncFile(outFile *os.File) error {
	if outFile == nil {
		return nil
	}
	if err := outFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}
	return nil
}

func (d *ConcurrentDownloader) bootstrapMetadata(ctx context.Context, client *http.Client, rawurl string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create concurrent bootstrap request: %w", err)
	}

	// Preserve auth/session cookies from the browser across the bootstrap request;
	// the server may reject unauthenticated probes with 401/403.
	for key, val := range d.Headers {
		if key != "Range" {
			req.Header.Set(key, val)
		}
	}
	// Range must come after custom headers so a caller-supplied Range can't override the probe byte
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
