package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/google/uuid"
)

func completedSpeedMBps(entry types.DownloadEntry) float64 {
	if entry.Status != "completed" {
		return 0
	}
	if entry.AvgSpeed > 0 {
		return entry.AvgSpeed / float64(types.MB)
	}
	if entry.TimeTaken > 0 {
		return float64(entry.TotalSize) * 1000 / float64(entry.TimeTaken) / float64(types.MB)
	}
	return 0
}

// ReloadSettings reloads settings from disk
func (s *LocalDownloadService) ReloadSettings() error {
	settings, err := config.LoadSettings()
	if err != nil {
		return err
	}
	s.settingsMu.Lock()
	s.settings = settings
	s.settingsMu.Unlock()
	return nil
}

// LocalDownloadService implements DownloadService for the local embedded engine.
type LocalDownloadService struct {
	Pool    *download.WorkerPool
	InputCh chan interface{}

	// Broadcast fields
	listeners  []chan interface{}
	listenerMu sync.Mutex

	broadcastWG  sync.WaitGroup
	reportTicker *time.Ticker
	reportWG     sync.WaitGroup

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	// shutdownOnce guarantees Shutdown is safe to call multiple times.
	shutdownOnce sync.Once
	shutdownErr  error

	// Settings Cache
	settings   *config.Settings
	settingsMu sync.RWMutex

	lifecycleHooks LifecycleHooks
}

// LifecycleHooks routes service-level management calls through the LifecycleManager.
type LifecycleHooks struct {
	Pause       func(id string) error
	Resume      func(id string) error
	ResumeBatch func(ids []string) []error
	Cancel      func(id string) error
	UpdateURL   func(id, newURL string) error
}

const (
	SpeedSmoothingAlpha = 0.3
	ReportInterval      = 150 * time.Millisecond
)

// NewLocalDownloadService creates a new specific service instance.
func NewLocalDownloadService(pool *download.WorkerPool) *LocalDownloadService {
	return NewLocalDownloadServiceWithInput(pool, nil)
}

// NewLocalDownloadServiceWithInput creates a service using a provided input channel.
// If inputCh is nil, a new buffered channel is created.
func NewLocalDownloadServiceWithInput(pool *download.WorkerPool, inputCh chan interface{}) *LocalDownloadService {
	if inputCh == nil {
		inputCh = make(chan interface{}, 100)
	}
	s := &LocalDownloadService{
		Pool:      pool,
		InputCh:   inputCh,
		listeners: make([]chan interface{}, 0),
	}

	// Load initial settings
	if s.settings, _ = config.LoadSettings(); s.settings == nil {
		s.settings = config.DefaultSettings()
	}

	// Lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	// Start broadcaster
	s.broadcastWG.Add(1)
	go func() {
		defer s.broadcastWG.Done()
		s.broadcastLoop()
	}()

	// Start progress reporter
	if pool != nil {
		s.reportTicker = time.NewTicker(ReportInterval)
		s.reportWG.Add(1)
		go func() {
			defer s.reportWG.Done()
			s.reportProgressLoop()
		}()
	}

	return s
}

func (s *LocalDownloadService) broadcastLoop() {
	for msg := range s.InputCh {
		s.listenerMu.Lock()
		for _, ch := range s.listeners {
			// Check message type
			isProgress := false
			switch msg.(type) {
			case events.ProgressMsg:
				isProgress = true
			case events.BatchProgressMsg:
				isProgress = true
			}

			if isProgress {
				// Non-blocking send for progress updates
				select {
				case ch <- msg:
				default:
					// Drop progress message if channel is full
				}
			} else {
				// Blocking send with timeout for critical state changes
				// We don't want to drop these, but we also don't want to block forever if a client is dead
				select {
				case ch <- msg:
				case <-time.After(1 * time.Second):
					utils.Debug("Dropped critical event due to slow client")
				}
			}
		}
		s.listenerMu.Unlock()
	}
	// Close all listeners when input closes
	s.listenerMu.Lock()
	for _, ch := range s.listeners {
		close(ch)
	}
	s.listeners = nil
	s.listenerMu.Unlock()

	if s.reportTicker != nil {
		s.reportTicker.Stop()
	}
}

func (s *LocalDownloadService) reportProgressLoop() {
	lastSpeeds := make(map[string]float64)
	lastChunkSnapshot := make(map[string]time.Time)

	if s.reportTicker == nil {
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.reportTicker.C:
		}

		if s.Pool == nil {
			continue
		}
		alpha := s.getSpeedEmaAlpha()

		var batch events.BatchProgressMsg

		activeConfigs := s.Pool.GetAll()
		for _, cfg := range activeConfigs {
			if cfg.State == nil || cfg.State.IsPaused() || cfg.State.Done.Load() {
				// Clean up speed history for inactive
				delete(lastSpeeds, cfg.ID)
				delete(lastChunkSnapshot, cfg.ID)
				continue
			}

			// Calculate Progress
			downloaded, total, totalElapsed, sessionElapsed, connections, sessionStart := cfg.State.GetProgress()

			// Calculate Speed with EMA
			sessionDownloaded := downloaded - sessionStart
			var instantSpeed float64
			if sessionElapsed.Seconds() > 0 && sessionDownloaded > 0 {
				instantSpeed = float64(sessionDownloaded) / sessionElapsed.Seconds()
			}

			lastSpeed := lastSpeeds[cfg.ID]
			var currentSpeed float64
			if lastSpeed == 0 {
				currentSpeed = instantSpeed
			} else {
				currentSpeed = alpha*instantSpeed + (1-alpha)*lastSpeed
			}
			lastSpeeds[cfg.ID] = currentSpeed

			// Create Message
			msg := events.ProgressMsg{
				DownloadID:        cfg.ID,
				Downloaded:        downloaded,
				Total:             total,
				Speed:             currentSpeed,
				Elapsed:           totalElapsed,
				ActiveConnections: int(connections),
			}

			// Chunk snapshots are expensive due to bitmap/progress copies.
			// Send them at a lower cadence than scalar progress fields.
			if time.Since(lastChunkSnapshot[cfg.ID]) >= 500*time.Millisecond {
				bitmap, width, _, chunkSize, chunkProgress := cfg.State.GetBitmapSnapshot(true)
				if width > 0 && len(bitmap) > 0 {
					msg.ChunkBitmap = bitmap
					msg.BitmapWidth = width
					msg.ActualChunkSize = chunkSize
					msg.ChunkProgress = chunkProgress
					lastChunkSnapshot[cfg.ID] = time.Now()
				}
			}

			batch = append(batch, msg)
		}

		// Send batch to InputCh (non-blocking) if not empty
		if len(batch) > 0 {
			select {
			case <-s.ctx.Done():
				return
			case s.InputCh <- batch:
			default:
			}
		}
	}
}

func (s *LocalDownloadService) getSpeedEmaAlpha() float64 {
	s.settingsMu.RLock()
	settings := s.settings
	s.settingsMu.RUnlock()

	if settings == nil {
		return SpeedSmoothingAlpha
	}

	alpha := settings.Performance.SpeedEmaAlpha
	if alpha <= 0 || alpha > 1 {
		return SpeedSmoothingAlpha
	}

	return alpha
}

// StreamEvents returns a channel that receives real-time download events.
func (s *LocalDownloadService) StreamEvents(ctx context.Context) (<-chan interface{}, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ch := make(chan interface{}, 100)
	s.listenerMu.Lock()
	s.listeners = append(s.listeners, ch)
	s.listenerMu.Unlock()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			s.listenerMu.Lock()
			for i, listener := range s.listeners {
				if listener == ch {
					s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
					close(ch)
					break
				}
			}
			s.listenerMu.Unlock()
		})
	}

	// Callers own listener lifetime; service shutdown closes listeners after the
	// broadcaster drains InputCh so lifecycle persistence can observe final events.
	go func() {
		<-ctx.Done()
		cleanup()
	}()

	return ch, cleanup, nil
}

// Publish emits an event into the service's event stream.
func (s *LocalDownloadService) Publish(msg interface{}) error {
	if s.InputCh == nil {
		return fmt.Errorf("input channel not initialized")
	}
	select {
	case s.InputCh <- msg:
		return nil
	case <-time.After(1 * time.Second):
		return fmt.Errorf("event publish timeout")
	}
}

// Shutdown stops the service.
func (s *LocalDownloadService) Shutdown() error {
	s.shutdownOnce.Do(func() {
		if s.reportTicker != nil {
			s.reportTicker.Stop()
		}
		if s.Pool != nil {
			s.Pool.GracefulShutdown()
		}

		// Stop listeners and broadcaster
		s.cancel()
		s.reportWG.Wait()

		// Close input channel to stop broadcaster
		if s.InputCh != nil {
			close(s.InputCh)
		}
		s.broadcastWG.Wait()
	})
	return s.shutdownErr
}

// List returns the status of all active and completed downloads.
func (s *LocalDownloadService) List() ([]types.DownloadStatus, error) {
	var statuses []types.DownloadStatus

	// 1. Get active downloads from pool
	if s.Pool != nil {
		activeConfigs := s.Pool.GetAll()
		for _, cfg := range activeConfigs {
			status := types.DownloadStatus{
				ID:       cfg.ID,
				URL:      cfg.URL,
				Filename: cfg.Filename,
				Status:   "downloading",
			}

			if cfg.State != nil {
				// Calculate progress and speed (thread-safe)
				downloaded, totalSize, _, sessionElapsed, connections, sessionStart := cfg.State.GetProgress()

				status.TotalSize = totalSize
				status.Downloaded = downloaded
				if dp := cfg.State.GetDestPath(); dp != "" {
					status.DestPath = dp
				}

				if status.TotalSize > 0 {
					status.Progress = float64(status.Downloaded) * 100 / float64(status.TotalSize)
				}

				// Get active connections count
				status.Connections = int(connections)

				// Update status based on state
				if cfg.State.IsPausing() {
					status.Status = "pausing"
				} else if cfg.State.IsPaused() {
					status.Status = "paused"
				} else if cfg.State.Done.Load() {
					status.Status = "completed"
				}

				// Calculate speed from progress only while actively downloading.
				if status.Status == "downloading" {
					sessionDownloaded := downloaded - sessionStart
					if sessionElapsed.Seconds() > 0 && sessionDownloaded > 0 {
						status.Speed = float64(sessionDownloaded) / sessionElapsed.Seconds() / float64(types.MB)

						// Calculate ETA (seconds remaining)
						remaining := status.TotalSize - status.Downloaded
						if remaining > 0 && status.Speed > 0 {
							speedBytes := status.Speed * float64(types.MB)
							status.ETA = int64(float64(remaining) / speedBytes)
						}
					}
				}
			}

			statuses = append(statuses, status)
		}
	}

	// 2. Fetch from database for history/paused/completed
	dbDownloads, err := state.ListAllDownloads()
	if err == nil {
		// Create a map of existing IDs to avoid duplicates
		existingIDs := make(map[string]bool)
		for _, s := range statuses {
			existingIDs[s.ID] = true
		}

		for _, d := range dbDownloads {
			// Skip if already present (active)
			if existingIDs[d.ID] {
				continue
			}

			var progress float64
			if d.TotalSize > 0 {
				progress = float64(d.Downloaded) * 100 / float64(d.TotalSize)
			} else if d.Status == "completed" {
				progress = 100.0
			}

			statuses = append(statuses, types.DownloadStatus{
				ID:          d.ID,
				URL:         d.URL,
				Filename:    d.Filename,
				DestPath:    d.DestPath,
				Status:      d.Status,
				TotalSize:   d.TotalSize,
				Downloaded:  d.Downloaded,
				Progress:    progress,
				Speed:       completedSpeedMBps(d),
				Connections: 0,
				TimeTaken:   d.TimeTaken,
				AvgSpeed:    d.AvgSpeed,
			})
		}
	}

	return statuses, nil
}

// Add queues a new download on the local pool without TUI confirmation.
func (s *LocalDownloadService) Add(url string, path string, filename string, mirrors []string, headers map[string]string, isExplicitCategory bool, totalSize int64, supportsRange bool) (string, error) {
	return s.add(url, path, filename, mirrors, headers, "", isExplicitCategory, totalSize, supportsRange)
}

// AddWithID queues a new download using a caller-provided id when non-empty.
func (s *LocalDownloadService) AddWithID(url string, path string, filename string, mirrors []string, headers map[string]string, id string, totalSize int64, supportsRange bool) (string, error) {
	// Remote or RPC-driven calls use preset IDs and should bypass interactive category routing.
	return s.add(url, path, filename, mirrors, headers, id, false, totalSize, supportsRange)
}

func (s *LocalDownloadService) add(url string, path string, filename string, mirrors []string, headers map[string]string, requestedID string, isExplicitCategory bool, totalSize int64, supportsRange bool) (string, error) {
	if s.Pool == nil {
		return "", fmt.Errorf("worker pool not initialized")
	}

	s.settingsMu.RLock()
	settings := s.settings
	s.settingsMu.RUnlock()

	outPath := path
	if outPath == "" {
		if settings.General.DefaultDownloadDir != "" {
			outPath = settings.General.DefaultDownloadDir
		} else {
			outPath = "."
		}
	}
	outPath = utils.EnsureAbsPath(outPath)

	id := strings.TrimSpace(requestedID)
	if id == "" {
		id = uuid.New().String()
	}
	if st := s.Pool.GetStatus(id); st != nil {
		return "", fmt.Errorf("download id already exists")
	}
	if entry, err := state.GetDownload(id); err != nil {
		return "", fmt.Errorf("failed to query download state: %w", err)
	} else if entry != nil {
		return "", fmt.Errorf("download id already exists")
	}

	state := types.NewProgressState(id, 0)
	state.DestPath = filepath.Join(outPath, filename) // Best guess until download starts

	cfg := types.DownloadConfig{
		URL:                url,
		Mirrors:            mirrors,
		OutputPath:         outPath,
		ID:                 id,
		Filename:           filename, // If empty, will be auto-detected
		ProgressCh:         s.InputCh,
		State:              state,
		Runtime:            types.ConvertRuntimeConfig(settings.ToRuntimeConfig()),
		Headers:            headers,
		IsExplicitCategory: isExplicitCategory,
		TotalSize:          totalSize,
		SupportsRange:      supportsRange,
	}

	s.Pool.Add(cfg)

	return id, nil
}

// Pause pauses an active download.
func (s *LocalDownloadService) Pause(id string) error {
	if s.lifecycleHooks.Pause != nil {
		return s.lifecycleHooks.Pause(id)
	}
	return fmt.Errorf("PauseFunc not initialized")
}

// Resume resumes a paused download.
func (s *LocalDownloadService) Resume(id string) error {
	if s.lifecycleHooks.Resume != nil {
		return s.lifecycleHooks.Resume(id)
	}
	return fmt.Errorf("ResumeFunc not initialized")
}

// ResumeBatch resumes multiple paused downloads efficiently.
func (s *LocalDownloadService) ResumeBatch(ids []string) []error {
	if s.lifecycleHooks.ResumeBatch != nil {
		return s.lifecycleHooks.ResumeBatch(ids)
	}
	errs := make([]error, len(ids))
	for i := range errs {
		errs[i] = fmt.Errorf("ResumeBatchFunc not initialized")
	}
	return errs
}

// SetLifecycleHooks wires the processing layer into the service so
// pause/resume/cancel/updateURL calls are routed through the lifecycle manager.
func (s *LocalDownloadService) SetLifecycleHooks(hooks LifecycleHooks) {
	s.lifecycleHooks = hooks
}

// UpdateURL updates the URL of a paused or errored download
func (s *LocalDownloadService) UpdateURL(id string, newURL string) error {
	if s.lifecycleHooks.UpdateURL != nil {
		return s.lifecycleHooks.UpdateURL(id, newURL)
	}
	// Fallback: update pool in-memory only (no DB persistence)
	if s.Pool == nil {
		return fmt.Errorf("worker pool not initialized")
	}
	return s.Pool.UpdateURL(id, newURL)
}

// Delete cancels and removes a download.
func (s *LocalDownloadService) Delete(id string) error {
	if s.lifecycleHooks.Cancel != nil {
		return s.lifecycleHooks.Cancel(id)
	}
	// Fallback when lifecycle hooks not wired (e.g. tests)
	if s.Pool == nil {
		return fmt.Errorf("worker pool not initialized")
	}
	s.Pool.Cancel(id)
	if entry, err := state.GetDownload(id); err == nil && entry != nil {
		if s.InputCh != nil {
			s.InputCh <- events.DownloadRemovedMsg{
				DownloadID: id,
				Filename:   entry.Filename,
				DestPath:   entry.DestPath,
				Completed:  entry.Status == "completed",
			}
		}
	}
	return nil
}

// GetStatus returns a status for a single download by id.
func (s *LocalDownloadService) GetStatus(id string) (*types.DownloadStatus, error) {
	if id == "" {
		return nil, fmt.Errorf("missing id")
	}

	// 1. Check active pool
	if s.Pool != nil {
		status := s.Pool.GetStatus(id)
		if status != nil {
			return status, nil
		}
	}

	// 2. Fallback to DB
	entry, err := state.GetDownload(id)
	if err == nil && entry != nil {
		var progress float64
		if entry.TotalSize > 0 {
			progress = float64(entry.Downloaded) * 100 / float64(entry.TotalSize)
		} else if entry.Status == "completed" {
			progress = 100.0
		}

		status := types.DownloadStatus{
			ID:         entry.ID,
			URL:        entry.URL,
			Filename:   entry.Filename,
			TotalSize:  entry.TotalSize,
			Downloaded: entry.Downloaded,
			Progress:   progress,
			Speed:      completedSpeedMBps(*entry),
			Status:     entry.Status,
			TimeTaken:  entry.TimeTaken,
			AvgSpeed:   entry.AvgSpeed,
		}
		return &status, nil
	}

	return nil, fmt.Errorf("download not found")
}

// History returns completed downloads
func (s *LocalDownloadService) History() ([]types.DownloadEntry, error) {
	// For local service, we can directly access the state DB
	return state.LoadCompletedDownloads()
}
