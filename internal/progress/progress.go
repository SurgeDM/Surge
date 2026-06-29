package progress

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SurgeDM/Surge/internal/types"

	"github.com/SurgeDM/Surge/internal/utils"
)

type DownloadProgress struct {
	ID            string
	Downloaded    atomic.Int64
	TotalSize     int64
	DestPath      string // Initial destination path
	Filename      string // Initial filename
	URL           string // Source URL
	StartTime     time.Time
	ActiveWorkers atomic.Int32
	Done          atomic.Bool
	Error         atomic.Pointer[error]
	Paused        atomic.Bool
	Pausing       atomic.Bool // Intermediate state: Pause requested but workers not yet exited
	RateLimited   atomic.Bool // Set when the downloader is backing off due to HTTP 429/rate-limit
	cancelFunc    context.CancelFunc

	VerifiedProgress  atomic.Int64  // Verified bytes written to disk (for UI progress)
	SessionStartBytes int64         // SessionStartBytes tracks how many bytes were already downloaded when the current session started
	SavedElapsed      time.Duration // Time spent in previous sessions
	RateLimitBps      int64         // Effective per-download rate limit in bytes/sec
	RateLimitSet      bool          // Whether RateLimitBps is an explicit per-download override

	Mirrors []types.MirrorStatus

	ChunkBitmap     []byte
	ChunkProgress   []int64
	ActualChunkSize int64
	BitmapWidth     int

	mu sync.Mutex // Protects TotalSize, StartTime, SessionStartBytes, SavedElapsed, Mirrors
}

func (ps *DownloadProgress) SetDestPath(path string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.DestPath = path
}

func (ps *DownloadProgress) GetDestPath() string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.DestPath
}

func (ps *DownloadProgress) SetFilename(filename string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Filename = filename
}

func (ps *DownloadProgress) GetFilename() string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.Filename
}

func (ps *DownloadProgress) SetURL(url string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.URL = url
}

func (ps *DownloadProgress) GetURL() string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.URL
}

func (ps *DownloadProgress) SetRateLimit(rate int64, explicit bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.RateLimitBps = rate
	ps.RateLimitSet = explicit
}

func (ps *DownloadProgress) GetRateLimit() (int64, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.RateLimitBps, ps.RateLimitSet
}

func New(id string, totalSize int64) *DownloadProgress {
	return &DownloadProgress{
		ID:        id,
		TotalSize: totalSize,
		StartTime: time.Now(),
	}
}

func (ps *DownloadProgress) SetTotalSize(size int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// If size is already set and timer is running, don't reset the clock.
	// This prevents post-download updates from erasing the session duration.
	if ps.TotalSize == size && !ps.StartTime.IsZero() {
		return
	}

	ps.TotalSize = size
	ps.SessionStartBytes = ps.VerifiedProgress.Load()
	ps.StartTime = time.Now()
}

func (ps *DownloadProgress) SyncSessionStart() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.SessionStartBytes = ps.VerifiedProgress.Load()
	ps.StartTime = time.Now()
}

func (ps *DownloadProgress) SetError(err error) {
	ps.Error.Store(&err)
}

func (ps *DownloadProgress) GetError() error {
	if e := ps.Error.Load(); e != nil {
		return *e
	}
	return nil
}

func (ps *DownloadProgress) GetProgress() (downloaded int64, total int64, totalElapsed time.Duration, sessionElapsed time.Duration, connections int32, sessionStartBytes int64) {
	downloaded = ps.VerifiedProgress.Load()
	connections = ps.ActiveWorkers.Load()
	paused := ps.Paused.Load()

	ps.mu.Lock()
	total = ps.TotalSize
	savedElapsed := ps.SavedElapsed
	startTime := ps.StartTime
	sessionStartBytes = ps.SessionStartBytes
	ps.mu.Unlock()

	// Elapsed time excludes paused duration.
	if paused {
		sessionElapsed = 0
		totalElapsed = savedElapsed
	} else {
		sessionElapsed = time.Since(startTime)
		if sessionElapsed < 0 {
			sessionElapsed = 0
		}
		totalElapsed = savedElapsed + sessionElapsed
	}
	if totalElapsed < 0 {
		totalElapsed = 0
	}

	return
}

func (ps *DownloadProgress) Pause() {
	ps.Paused.Store(true)
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancelFunc != nil {
		ps.cancelFunc()
	}
}

func (ps *DownloadProgress) SetCancelFunc(cancel context.CancelFunc) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.cancelFunc = cancel
}

func (ps *DownloadProgress) Resume() {
	ps.Paused.Store(false)
}

func (ps *DownloadProgress) IsPaused() bool {
	return ps.Paused.Load()
}

func (ps *DownloadProgress) SetPausing(pausing bool) {
	ps.Pausing.Store(pausing)
}

func (ps *DownloadProgress) IsPausing() bool {
	return ps.Pausing.Load()
}

func (ps *DownloadProgress) SetSavedElapsed(d time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.SavedElapsed = d
}

func (ps *DownloadProgress) GetSavedElapsed() time.Duration {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.SavedElapsed
}

// FinalizeSession closes the current session and accumulates its elapsed time into total elapsed.
// It returns (sessionElapsed, totalElapsedAfterFinalize).
func (ps *DownloadProgress) FinalizeSession(downloaded int64) (time.Duration, time.Duration) {
	if downloaded < 0 {
		downloaded = ps.VerifiedProgress.Load()
	}

	now := time.Now()
	ps.mu.Lock()
	sessionElapsed := now.Sub(ps.StartTime)
	if sessionElapsed < 0 {
		sessionElapsed = 0
	}
	ps.SavedElapsed += sessionElapsed
	if ps.SavedElapsed < 0 {
		ps.SavedElapsed = 0
	}
	ps.SessionStartBytes = downloaded
	ps.StartTime = now
	totalElapsed := ps.SavedElapsed
	ps.mu.Unlock()

	ps.Downloaded.Store(downloaded)
	ps.VerifiedProgress.Store(downloaded)

	return sessionElapsed, totalElapsed
}

// SessionReset wipes the current progress and session state, allowing for a fresh start (e.g. fallback).
func (ps *DownloadProgress) SessionReset() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.Downloaded.Store(0)
	ps.VerifiedProgress.Store(0)
	ps.SessionStartBytes = 0
	ps.StartTime = time.Now()
	ps.SavedElapsed = 0
	ps.ActiveWorkers.Store(0)
	ps.Done.Store(false)
	ps.Paused.Store(false)
	ps.Pausing.Store(false)
	ps.RateLimited.Store(false)
	ps.Error.Store(nil)

	// Clear mirrors error status
	for i := range ps.Mirrors {
		ps.Mirrors[i].Error = false
	}

	// Reset chunk tracking if initialized
	if ps.BitmapWidth > 0 {
		ps.ChunkBitmap = make([]byte, len(ps.ChunkBitmap))
		ps.ChunkProgress = make([]int64, ps.BitmapWidth)
	}
}

// FinalizePauseSession finalizes the current session for a pause transition.
// It keeps timing/data frozen while paused and returns total elapsed after finalize.
func (ps *DownloadProgress) FinalizePauseSession(downloaded int64) time.Duration {
	_, total := ps.FinalizeSession(downloaded)
	return total
}

func (ps *DownloadProgress) SetMirrors(mirrors []types.MirrorStatus) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	// Deep copy to prevent race conditions if caller modifies the slice
	ps.Mirrors = make([]types.MirrorStatus, len(mirrors))
	copy(ps.Mirrors, mirrors)
}

func (ps *DownloadProgress) GetMirrors() []types.MirrorStatus {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	// Return a copy
	if len(ps.Mirrors) == 0 {
		return nil
	}
	mirrors := make([]types.MirrorStatus, len(ps.Mirrors))
	copy(mirrors, ps.Mirrors)
	return mirrors
}

// bitmapLayout returns the number of tracked chunks and backing bytes for a
// 2-bit-per-chunk bitmap.
func bitmapLayout(totalSize, chunkSize int64) (numChunks int, bytesNeeded int, ok bool) {
	if totalSize <= 0 || chunkSize <= 0 {
		return 0, 0, false
	}

	numChunks = int((totalSize + chunkSize - 1) / chunkSize)
	if numChunks <= 0 {
		return 0, 0, false
	}

	bytesNeeded = (numChunks + 3) / 4
	return numChunks, bytesNeeded, true
}

// InitBitmap initializes the chunk bitmap
func (ps *DownloadProgress) InitBitmap(totalSize int64, chunkSize int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(ps.ChunkBitmap) > 0 && ps.TotalSize == totalSize && ps.ActualChunkSize == chunkSize {
		return
	}

	utils.Debug("InitBitmap: Total=%d, ChunkSize=%d", totalSize, chunkSize)

	if chunkSize <= 0 {
		return
	}

	numChunks, bytesNeeded, ok := bitmapLayout(totalSize, chunkSize)
	if !ok {
		return
	}

	ps.ActualChunkSize = chunkSize
	ps.BitmapWidth = numChunks
	ps.ChunkBitmap = make([]byte, bytesNeeded)
	ps.ChunkProgress = make([]int64, numChunks)
}

// RestoreBitmap restores the chunk bitmap from saved state
func (ps *DownloadProgress) RestoreBitmap(bitmap []byte, actualChunkSize int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(bitmap) == 0 || actualChunkSize <= 0 || ps.TotalSize <= 0 {
		return
	}

	numChunks, bytesNeeded, ok := bitmapLayout(ps.TotalSize, actualChunkSize)
	if !ok {
		return
	}

	// Deep copy to prevent mutation hazard of caller's backing array
	ps.ChunkBitmap = make([]byte, bytesNeeded)
	copy(ps.ChunkBitmap, bitmap)
	ps.ActualChunkSize = actualChunkSize
	ps.BitmapWidth = numChunks

	if len(ps.ChunkProgress) != numChunks {
		ps.ChunkProgress = make([]int64, numChunks)
	}
}

// SetChunkProgress updates chunk progress array from external sources (e.g. remote events).
func (ps *DownloadProgress) SetChunkProgress(progress []int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(progress) == 0 {
		return
	}
	if len(ps.ChunkProgress) != len(progress) {
		ps.ChunkProgress = make([]int64, len(progress))
	}
	copy(ps.ChunkProgress, progress)
}

// SetChunkState sets the 2-bit state for a specific chunk index (thread-safe)
func (ps *DownloadProgress) SetChunkState(index int, status types.ChunkStatus) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.setChunkState(index, status)
}

// setChunkState sets the 2-bit state (internal, expects lock)
func (ps *DownloadProgress) setChunkState(index int, status types.ChunkStatus) {
	if index < 0 || index >= ps.BitmapWidth {
		return
	}

	byteIndex := index / 4
	if byteIndex >= len(ps.ChunkBitmap) {
		return
	}
	bitOffset := (index % 4) * 2

	mask := byte(3 << bitOffset)
	ps.ChunkBitmap[byteIndex] &= ^mask

	val := byte(status) << bitOffset
	ps.ChunkBitmap[byteIndex] |= val
}

// GetChunkState gets the 2-bit state for a specific chunk index (thread-safe)
func (ps *DownloadProgress) GetChunkState(index int) types.ChunkStatus {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.getChunkState(index)
}

// getChunkState gets the 2-bit state (internal, expects lock)
func (ps *DownloadProgress) getChunkState(index int) types.ChunkStatus {
	if index < 0 || index >= ps.BitmapWidth {
		return types.ChunkPending
	}

	byteIndex := index / 4
	if byteIndex >= len(ps.ChunkBitmap) {
		return types.ChunkPending
	}
	bitOffset := (index % 4) * 2

	val := (ps.ChunkBitmap[byteIndex] >> bitOffset) & 3
	return types.ChunkStatus(val)
}

// UpdateChunkStatus updates the bitmap based on byte range
func (ps *DownloadProgress) UpdateChunkStatus(offset, length int64, status types.ChunkStatus) {
	ps.mu.Lock()

	if ps.ActualChunkSize == 0 || len(ps.ChunkBitmap) == 0 {
		ps.mu.Unlock()
		return
	}

	if len(ps.ChunkProgress) != ps.BitmapWidth {
		utils.Debug("UpdateChunkStatus: Initializing ChunkProgress array (width=%d)", ps.BitmapWidth)
		ps.ChunkProgress = make([]int64, ps.BitmapWidth)
	}

	startIdx := int(offset / ps.ActualChunkSize)
	endIdx := int((offset + length - 1) / ps.ActualChunkSize)

	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= ps.BitmapWidth {
		endIdx = ps.BitmapWidth - 1
	}

	var totalIncrement int64

	for i := startIdx; i <= endIdx; i++ {
		// Calculate precise overlap with this chunk
		chunkStart := int64(i) * ps.ActualChunkSize
		chunkEnd := chunkStart + ps.ActualChunkSize
		if chunkEnd > ps.TotalSize {
			chunkEnd = ps.TotalSize
		}

		updateStart := offset
		if updateStart < chunkStart {
			updateStart = chunkStart
		}

		updateEnd := offset + length
		if updateEnd > chunkEnd {
			updateEnd = chunkEnd
		}

		overlap := updateEnd - updateStart
		if overlap < 0 {
			overlap = 0
		}

		switch status {
		case types.ChunkCompleted:
			increment := overlap
			remainingSpace := (chunkEnd - chunkStart) - ps.ChunkProgress[i]

			if increment > remainingSpace {
				increment = remainingSpace
			}

			if increment > 0 {
				ps.ChunkProgress[i] += increment
				totalIncrement += increment
			}

			if ps.ChunkProgress[i] >= (chunkEnd - chunkStart) {
				ps.ChunkProgress[i] = chunkEnd - chunkStart
				ps.setChunkState(i, types.ChunkCompleted)
			} else {
				if ps.getChunkState(i) != types.ChunkCompleted {
					ps.setChunkState(i, types.ChunkDownloading)
				}
			}
		case types.ChunkDownloading:
			current := ps.getChunkState(i)
			if current != types.ChunkCompleted {
				ps.setChunkState(i, types.ChunkDownloading)
			}
		}
	}

	ps.mu.Unlock()

	if totalIncrement > 0 {
		ps.VerifiedProgress.Add(totalIncrement)
	}
}

// RecalculateProgress reconstructs ChunkProgress from remaining tasks (for resume)
func (ps *DownloadProgress) RecalculateProgress(remainingTasks []types.Task) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.ActualChunkSize == 0 || ps.BitmapWidth == 0 {
		return
	}

	ps.ChunkProgress = make([]int64, ps.BitmapWidth)
	var totalVerified int64
	for i := 0; i < ps.BitmapWidth; i++ {
		chunkStart := int64(i) * ps.ActualChunkSize
		chunkEnd := chunkStart + ps.ActualChunkSize
		if chunkEnd > ps.TotalSize {
			chunkEnd = ps.TotalSize
		}
		ps.ChunkProgress[i] = chunkEnd - chunkStart
		totalVerified += ps.ChunkProgress[i]
	}

	for _, task := range remainingTasks {
		offset := task.Offset
		length := task.Length

		startIdx := int(offset / ps.ActualChunkSize)
		endIdx := int((offset + length - 1) / ps.ActualChunkSize)

		if startIdx < 0 {
			startIdx = 0
		}
		if endIdx >= ps.BitmapWidth {
			endIdx = ps.BitmapWidth - 1
		}

		for i := startIdx; i <= endIdx; i++ {
			chunkStart := int64(i) * ps.ActualChunkSize
			chunkEnd := chunkStart + ps.ActualChunkSize
			if chunkEnd > ps.TotalSize {
				chunkEnd = ps.TotalSize
			}

			taskStart := offset
			if taskStart < chunkStart {
				taskStart = chunkStart
			}

			taskEnd := offset + length
			if taskEnd > chunkEnd {
				taskEnd = chunkEnd
			}

			overlap := taskEnd - taskStart
			if overlap > 0 {
				ps.ChunkProgress[i] -= overlap
				totalVerified -= overlap
			}
		}
	}

	ps.VerifiedProgress.Store(totalVerified)

	for i := 0; i < ps.BitmapWidth; i++ {
		chunkStart := int64(i) * ps.ActualChunkSize
		chunkEnd := chunkStart + ps.ActualChunkSize
		if chunkEnd > ps.TotalSize {
			chunkEnd = ps.TotalSize
		}
		chunkSize := chunkEnd - chunkStart

		if ps.ChunkProgress[i] >= chunkSize {
			ps.ChunkProgress[i] = chunkSize
			ps.setChunkState(i, types.ChunkCompleted)
		} else if ps.ChunkProgress[i] > 0 {
			ps.setChunkState(i, types.ChunkDownloading)
		} else {
			ps.ChunkProgress[i] = 0
			ps.setChunkState(i, types.ChunkPending)
		}
	}
}

// GetBitmap returns a copy of the bitmap and metadata
func (ps *DownloadProgress) GetBitmap() ([]byte, int, int64, int64, []int64) {
	return ps.GetBitmapSnapshot(true)
}

// GetBitmapSnapshot returns a copy of bitmap metadata and optionally chunk progress.
func (ps *DownloadProgress) GetBitmapSnapshot(includeProgress bool) ([]byte, int, int64, int64, []int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(ps.ChunkBitmap) == 0 {
		return nil, 0, 0, 0, nil
	}

	result := make([]byte, len(ps.ChunkBitmap))
	copy(result, ps.ChunkBitmap)

	var progressResult []int64
	if includeProgress {
		progressResult = make([]int64, len(ps.ChunkProgress))
		copy(progressResult, ps.ChunkProgress)
	}

	return result, ps.BitmapWidth, ps.TotalSize, ps.ActualChunkSize, progressResult
}
