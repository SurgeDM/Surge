package processing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// AddDownloadFunc is the lifecycle's handoff into the engine-facing queue layer.
type AddDownloadFunc func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error)

// AddDownloadWithIDFunc preserves caller-chosen ids when a remote/UI layer already owns them.
type AddDownloadWithIDFunc func(string, string, string, []string, map[string]string, string, int64, bool) (string, error)

// IsNameActiveFunc lets routing treat in-flight downloads as filename conflicts within a directory.
type IsNameActiveFunc func(dir, name string) bool

type LifecycleManager struct {
	settings            *config.Settings
	settingsMu          sync.RWMutex
	settingsRefreshedAt time.Time
	addFunc             AddDownloadFunc
	addWithIDFunc       AddDownloadWithIDFunc
	isNameActive        IsNameActiveFunc
	engineHooks         EngineHooks
	hooksMu             sync.RWMutex
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

func NewLifecycleManager(addFunc AddDownloadFunc, addWithIDFunc AddDownloadWithIDFunc, isNameActive ...IsNameActiveFunc) *LifecycleManager {
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
	if settings != nil && settings.Network.MaxConcurrentProbes > 0 {
		probeCap = settings.Network.MaxConcurrentProbes
	}
	sem := make(chan struct{}, probeCap)
	for i := 0; i < probeCap; i++ {
		sem <- struct{}{}
	}

	return &LifecycleManager{
		settings:            settings,
		settingsRefreshedAt: time.Now(),
		addFunc:             addFunc,
		addWithIDFunc:       addWithIDFunc,
		isNameActive:        activeCheck,
		probeSem:            sem,
	}
}

// SetEngineHooks injects dependencies the manager needs to interact with the broader system
// (like the download worker pool or the event system) without causing cyclic dependency graphs.
func (mgr *LifecycleManager) SetEngineHooks(hooks EngineHooks) {
	mgr.hooksMu.Lock()
	defer mgr.hooksMu.Unlock()
	mgr.engineHooks = hooks
}

// getEngineHooks safely returns the current engine hooks.
func (mgr *LifecycleManager) getEngineHooks() EngineHooks {
	mgr.hooksMu.RLock()
	defer mgr.hooksMu.RUnlock()
	return mgr.engineHooks
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
}

// Enqueue probes and reserves a stable destination before dispatching to the queue layer.
func (mgr *LifecycleManager) Enqueue(ctx context.Context, req *DownloadRequest) (string, string, error) {
	if mgr.addFunc == nil {
		return "", "", fmt.Errorf("add function unavailable")
	}

	utils.Debug("Lifecycle: Enqueue %s (Filename: %s)", req.URL, req.Filename)
	return mgr.enqueueResolved(ctx, req, func(finalPath, finalFilename string, probe *ProbeResult) (string, error) {
		return mgr.addFunc(
			req.URL,
			finalPath,
			finalFilename,
			req.Mirrors,
			req.Headers,
			req.IsExplicitCategory,
			probe.FileSize,
			probe.SupportsRange,
		)
	})
}

// EnqueueWithID does the same lifecycle work as Enqueue while preserving a caller-owned id.
func (mgr *LifecycleManager) EnqueueWithID(ctx context.Context, req *DownloadRequest, requestID string) (string, string, error) {
	if mgr.addWithIDFunc == nil {
		return "", "", fmt.Errorf("addWithID function unavailable")
	}

	utils.Debug("Lifecycle: EnqueueWithID %s (%s)", req.URL, requestID)
	return mgr.enqueueResolved(ctx, req, func(finalPath, finalFilename string, probe *ProbeResult) (string, error) {
		return mgr.addWithIDFunc(
			req.URL,
			finalPath,
			finalFilename,
			req.Mirrors,
			req.Headers,
			requestID,
			probe.FileSize,
			probe.SupportsRange,
		)
	})
}

// enqueueResolved prepares the final path and working file before handing the
// download to the engine, so workers and lifecycle events agree on one stable destination.
func (mgr *LifecycleManager) enqueueResolved(ctx context.Context, req *DownloadRequest, dispatch func(string, string, *ProbeResult) (string, error)) (string, string, error) {
	if req.URL == "" {
		return "", "", fmt.Errorf("URL is required")
	}
	if req.Path == "" {
		return "", "", fmt.Errorf("destination path is required")
	}

	settings := mgr.GetSettings()

	// Throttle concurrent probes — acquire a semaphore slot before probing.
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

	probe, err := ProbeServerWithProxy(ctx, req.URL, req.Filename, req.Headers, settings.ToRuntimeConfig())
	if err != nil {
		utils.Debug("Lifecycle: Probe failed: %v\n", err)
		return "", "", fmt.Errorf("probe failed: %w", err)
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
			probe,
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
		newID, err := dispatch(finalPath, finalFilename, probe)
		if err != nil {
			_ = os.Remove(surgePath)
			return "", "", err
		}

		// Emit queued event now that the pool has accepted the download.
		// The event worker persists this to DB so it survives a crash before the
		// worker emits a started event.
		hooks := mgr.getEngineHooks()
		if hooks.PublishEvent != nil {
			_ = hooks.PublishEvent(events.DownloadQueuedMsg{
				DownloadID: newID,
				Filename:   finalFilename,
				URL:        req.URL,
				DestPath:   filepath.Join(finalPath, finalFilename),
				Mirrors:    append([]string(nil), req.Mirrors...),
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
