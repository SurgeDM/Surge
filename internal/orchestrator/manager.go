package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"net/url"

	"github.com/SurgeDM/Surge/internal/config"
	probing "github.com/SurgeDM/Surge/internal/probe"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/types"

	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/utils"
)

// IsNameActiveFunc lets routing treat in-flight downloads as filename conflicts within a directory.
type IsNameActiveFunc func(dir, name string) bool

// cfgProgress returns the *progress.DownloadProgress associated with cfg, or
// nil if cfg.ProgressState is nil. This is the single point in the orchestrator package
// where the untyped State field is narrowed to a concrete type.
func cfgProgress(cfg *types.DownloadRecord) *progress.DownloadProgress {
	if cfg == nil || cfg.ProgressState == nil {
		return nil
	}
	return cfg.ProgressState.(*progress.DownloadProgress)
}

type LifecycleManager struct {
	settings            *config.Settings
	settingsMu          sync.RWMutex
	settingsRefreshedAt time.Time
	pool                *scheduler.Scheduler
	eventBus            *EventBus
	aggregator          *ProgressAggregator
	isNameActive        IsNameActiveFunc

	// probeSem caps the number of simultaneous server probes so adding a
	// large batch of downloads does not flood the network with HEAD requests.
	probeSem chan struct{}
}

const (
	maxWorkingFileReservationAttempts = 100
	// defaultMaxConcurrentProbes is the fallback probe concurrency cap used when
	// no settings value is available. The live value comes from
	// NetworkSettings.MaxConcurrentProbes.
	defaultMaxConcurrentProbes = 3
	// maxConcurrentProbes is the package-level cap used by tests that construct
	// a manager without a settings snapshot (newLifecycleManagerForTest).
	maxConcurrentProbes = defaultMaxConcurrentProbes
)

var settingsRefreshTTL = time.Second

var reserveWorkingFile = precreateWorkingFile

func precreateWorkingFile(destPath, filename string) error {
	if err := os.MkdirAll(destPath, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	surgePath := filepath.Join(destPath, filename) + types.IncompleteSuffix
	// Exclusive create turns the .surge file into the reservation itself, so two
	// concurrent enqueues cannot silently target the same working path.
	file, err := os.OpenFile(surgePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to pre-create working file: %w", err)
	}
	_ = file.Close()
	return nil
}

// Falls back to a no-op so enqueue callers can always consult the active-name
// hook safely, even in tests or remote contexts that do not have pool access.
func (mgr *LifecycleManager) buildIsNameActive() func(string, string) bool {
	if mgr.isNameActive != nil {
		return mgr.isNameActive
	}
	return func(string, string) bool { return false }
}

func NewLifecycleManager(pool *scheduler.Scheduler, eventBus *EventBus, isNameActive ...IsNameActiveFunc) *LifecycleManager {
	// Snapshot settings once so enqueue can still make routing decisions even if
	// a later disk read fails or the caller never opens the settings UI.
	settings, err := config.LoadSettings()
	if err != nil {
		settings = config.DefaultSettings()
	}

	var activeCheck IsNameActiveFunc
	if len(isNameActive) > 0 {
		activeCheck = isNameActive[0]
	}

	probeCap := defaultMaxConcurrentProbes
	if settings != nil && config.Resolve[int](settings.Network.MaxConcurrentProbes) > 0 {
		probeCap = config.Resolve[int](settings.Network.MaxConcurrentProbes)
	}
	sem := make(chan struct{}, probeCap)
	for i := 0; i < probeCap; i++ {
		sem <- struct{}{}
	}

	var aggregator *ProgressAggregator
	if pool != nil && eventBus != nil {
		aggregator = NewProgressAggregator(pool, eventBus, settings)
	}

	return &LifecycleManager{
		settings:            settings,
		settingsRefreshedAt: time.Now(),
		pool:                pool,
		eventBus:            eventBus,
		aggregator:          aggregator,
		isNameActive:        activeCheck,
		probeSem:            sem,
	}
}

// GetScheduler returns the underlying scheduler
func (mgr *LifecycleManager) GetScheduler() *scheduler.Scheduler {
	return mgr.pool
}

// GetEventBus returns the event bus
func (mgr *LifecycleManager) GetEventBus() *EventBus {
	return mgr.eventBus
}

func (mgr *LifecycleManager) Shutdown() {
	if mgr.aggregator != nil {
		mgr.aggregator.Shutdown()
	}
	if mgr.eventBus != nil {
		mgr.eventBus.Shutdown()
	}
	if mgr.pool != nil {
		mgr.pool.GracefulShutdown()
	}
}

// GetSettings reloads disk-backed routing rules opportunistically so a long-lived
// lifecycle manager picks up saved settings changes without a restart.
func (m *LifecycleManager) GetSettings() *config.Settings {
	m.settingsMu.RLock()
	settings := m.settings
	refreshedAt := m.settingsRefreshedAt
	m.settingsMu.RUnlock()

	if settings != nil && time.Since(refreshedAt) < settingsRefreshTTL {
		return settings
	}

	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()

	// Double-check condition to prevent redundant disk reads under concurrent load
	if m.settings != nil && time.Since(m.settingsRefreshedAt) < settingsRefreshTTL {
		return m.settings
	}

	if loaded, err := config.LoadSettings(); err == nil && loaded != nil {
		m.settings = loaded
		m.settingsRefreshedAt = time.Now()
		return loaded
	}

	if m.settings == nil {
		return config.DefaultSettings()
	}
	return m.settings
}

// ApplySettings swaps in a new routing snapshot for future enqueue calls.
func (m *LifecycleManager) ApplySettings(s *config.Settings) {
	if s == nil {
		s = config.DefaultSettings()
	}
	m.settingsMu.Lock()
	m.settings = s
	m.settingsRefreshedAt = time.Now()
	m.settingsMu.Unlock()
}

// SaveSettings persists and applies a new routing snapshot for future enqueue calls.
func (m *LifecycleManager) SaveSettings(s *config.Settings) error {
	if err := config.SaveSettings(s); err != nil {
		return err
	}
	m.ApplySettings(s)
	return nil
}

// DownloadRequest carries the already-approved inputs needed to probe and reserve a file path.
type DownloadRequest struct {
	URL                string
	Filename           string
	Path               string
	Mirrors            []string
	Headers            map[string]string
	IsExplicitCategory bool
	SkipApproval       bool
	Workers            int
	MinChunkSize       int64
}

// Enqueue probes and reserves a stable destination before dispatching to the queue layer.
func (mgr *LifecycleManager) Enqueue(ctx context.Context, req *DownloadRequest) (string, string, error) {
	if mgr.pool == nil {
		return "", "", types.ErrServiceUnavailable
	}

	utils.Debug("Lifecycle: Enqueue %s (Filename: %s)", req.URL, req.Filename)
	return mgr.enqueueResolved(ctx, req, "")
}

// EnqueueWithID does the same lifecycle work as Enqueue while preserving a caller-owned id.
func (mgr *LifecycleManager) EnqueueWithID(ctx context.Context, req *DownloadRequest, requestID string) (string, string, error) {
	if mgr.pool == nil {
		return "", "", types.ErrServiceUnavailable
	}

	utils.Debug("Lifecycle: EnqueueWithID %s (%s)", req.URL, requestID)
	return mgr.enqueueResolved(ctx, req, requestID)
}

func (mgr *LifecycleManager) enqueueResolved(ctx context.Context, req *DownloadRequest, requestID string) (string, string, error) {
	if req.URL == "" {
		return "", "", types.ErrURLRequired
	}
	if req.Path == "" {
		return "", "", types.ErrDestRequired
	}

	settings := mgr.GetSettings()

	// Throttle concurrent probes - acquire a semaphore slot before probing.
	// If the context is cancelled (e.g., shutdown) we abort immediately.
	if mgr.probeSem != nil {
		select {
		case <-mgr.probeSem:
			// acquired
		case <-ctx.Done():
			return "", "", fmt.Errorf("enqueue aborted before probe: %w", ctx.Err())
		}
		defer func() { mgr.probeSem <- struct{}{} }()
	}

	probeResult, probeErr := probing.ProbeServerWithProxy(ctx, req.URL, req.Filename, req.Headers, settings.ToRuntimeConfig())
	if probeErr != nil {
		// Distinguish between terminal client errors (invalid scheme, etc) and
		// server-side rejections or timeouts that we can optimistically ignore.
		var urlErr *url.Error
		var isTerminal bool
		if errors.As(probeErr, &urlErr) {
			var opErr *net.OpError
			isTerminal = !errors.As(probeErr, &opErr) && // not a network-layer error
				strings.Contains(urlErr.Error(), "unsupported protocol scheme")
		}
		isTerminal = isTerminal || errors.Is(probeErr, probing.ErrProbeRequestCreation)

		if isTerminal {
			return "", "", probeErr
		}

		utils.Debug("Lifecycle: Probe failed: %v - enqueueing with optimistic fallback metadata\n", probeErr)
		// Probe failures are non-fatal for known server-side issues (403/405/500) or
		// network timeouts: some servers reject or intermittently fail
		// lightweight probe requests but still accept the actual download flow.
		// Mark range support as "unknown, try it" by keeping size at zero and
		// setting SupportsRange so the download path can attempt a concurrent
		// bootstrap before falling back to single-stream mode.
		probeResult = &probing.ProbeResult{}
		probeResult.SupportsRange = true
		if req.Filename != "" {
			probeResult.Filename = req.Filename
			probeResult.DetectedFilename = req.Filename
		}
	}

	isNameActive := mgr.buildIsNameActive()

	for attempt := 0; attempt < maxWorkingFileReservationAttempts; attempt++ {
		if ctx.Err() != nil {
			return "", "", fmt.Errorf("enqueue aborted: %w", ctx.Err())
		}

		finalPath, finalFilename, err := ResolveDestination(
			req.URL,
			req.Filename,
			req.Path,
			!req.IsExplicitCategory,
			settings,
			probeResult,
			isNameActive,
		)
		if err != nil {
			return "", "", fmt.Errorf("failed to resolve destination: %w", err)
		}

		// Reserve the working path before dispatch so a concurrent enqueue has to
		// pick a different name instead of truncating this in-flight download.
		if err := reserveWorkingFile(finalPath, finalFilename); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", "", err
		}

		surgePath := filepath.Join(finalPath, finalFilename) + types.IncompleteSuffix

		newID, err := mgr.dispatchToScheduler(req, requestID, finalPath, finalFilename, probeResult)
		if err != nil {
			_ = os.Remove(surgePath)
			return "", "", err
		}

		if mgr.eventBus != nil {
			var rateLimit int64
			var rateLimitSet bool
			if st := mgr.pool.GetStatus(newID); st != nil {
				rateLimit = st.RateLimit
				rateLimitSet = st.RateLimitSet
			} else if settings != nil && settings.Network.DefaultDownloadRateLimit != nil {
				if parsed, err := utils.ParseRateLimitValue(settings.Network.DefaultDownloadRateLimit.Value); err == nil {
					rateLimit = parsed
				}
			}
			_ = mgr.eventBus.Publish(types.DownloadEvent{
				Type:         types.EventQueued,
				DownloadID:   newID,
				Filename:     finalFilename,
				URL:          req.URL,
				DestPath:     filepath.Join(finalPath, finalFilename),
				Mirrors:      append([]string(nil), req.Mirrors...),
				RateLimit:    rateLimit,
				RateLimitSet: rateLimitSet,
				Workers:      req.Workers,
				MinChunkSize: req.MinChunkSize,
			})
		}

		return newID, finalFilename, nil
	}

	return "", "", fmt.Errorf("failed to reserve unique working file for %q after %d attempts", req.URL, maxWorkingFileReservationAttempts)
}

// IsNameActive reports whether the configured active-download callback would
// treat the given directory/name pair as an in-flight conflict.
func (mgr *LifecycleManager) IsNameActive(dir, name string) bool {
	return mgr.buildIsNameActive()(dir, name)
}

func (mgr *LifecycleManager) dispatchToScheduler(req *DownloadRequest, requestID string, finalPath string, finalFilename string, probeResult *probing.ProbeResult) (string, error) {
	if mgr.pool == nil {
		return "", types.ErrPoolNotInit
	}

	settings := mgr.GetSettings()
	id := strings.TrimSpace(requestID)
	if id == "" {
		id = uuid.New().String()
	}

	if st := mgr.pool.GetStatus(id); st != nil {
		return "", types.ErrIDExists
	}

	state := progress.New(id, 0)
	state.SetDestPath(filepath.Join(finalPath, finalFilename))

	runtime := settings.ToRuntimeConfig()
	if req.Workers > 0 {
		maxConns := runtime.GetMaxConnectionsPerDownload()
		if req.Workers > maxConns {
			req.Workers = maxConns
		}
		runtime.Workers = req.Workers
	}
	if req.MinChunkSize > 0 {
		runtime.MinChunkSize = req.MinChunkSize
	}

	var rateLimit int64
	var rateLimitSet bool
	if settings.Network.DefaultDownloadRateLimit != nil {
		if parsed, err := utils.ParseRateLimitValue(settings.Network.DefaultDownloadRateLimit.Value); err == nil {
			rateLimit = parsed
			rateLimitSet = true
		}
	}

	cfg := types.DownloadRecord{
		URL:                req.URL,
		Mirrors:            req.Mirrors,
		OutputPath:         finalPath,
		ID:                 id,
		Filename:           finalFilename,
		ProgressState:              state,
		Runtime:            runtime,
		Headers:            req.Headers,
		IsExplicitCategory: req.IsExplicitCategory,
		TotalSize:          probeResult.FileSize,
		SupportsRange:      probeResult.SupportsRange,
		RateLimit:       rateLimit,
		RateLimitSet:       rateLimitSet,
	}

	if mgr.eventBus != nil {
		cfg.ProgressCh = mgr.eventBus.InputCh
	}

	mgr.pool.Add(cfg)
	return id, nil
}
