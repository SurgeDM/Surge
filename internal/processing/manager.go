package processing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
)

// AddDownloadFunc represents a function capable of adding a download to the engine or queue
// url, path, filename, mirrors, headers, isExplicitCategory, totalSize, supportsRange
type AddDownloadFunc func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error)

// AddDownloadWithIDFunc adds a download with a predefined UUID (used by daemon/TUI for syncing)
type AddDownloadWithIDFunc func(string, string, string, []string, map[string]string, string, int64, bool) (string, error)

// LifecycleManager orchestrates the life of a download outside of the core HTTP engine.
// It handles probing, category routing, file conflict resolution, and settings management.
// IsNameActiveFunc checks whether a given filename is already being downloaded in a directory.
type IsNameActiveFunc func(dir, name string) bool

type LifecycleManager struct {
	settings      *config.Settings
	settingsMu    sync.RWMutex
	addFunc       AddDownloadFunc
	addWithIDFunc AddDownloadWithIDFunc
	IsNameActive  IsNameActiveFunc // Optional; set by wiring layer
}

const maxWorkingFileReservationAttempts = 100

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

// buildIsNameActive returns the configured callback or a safe no-op.
func (mgr *LifecycleManager) buildIsNameActive() func(string, string) bool {
	if mgr.IsNameActive != nil {
		return mgr.IsNameActive
	}
	return func(string, string) bool { return false }
}

func NewLifecycleManager(addFunc AddDownloadFunc, addWithIDFunc AddDownloadWithIDFunc) *LifecycleManager {
	// Snapshot settings once so enqueue can still make routing decisions even if
	// a later disk read fails or the caller never opens the settings UI.
	settings, err := config.LoadSettings()
	if err != nil {
		settings = config.DefaultSettings()
	}

	return &LifecycleManager{
		settings:      settings,
		addFunc:       addFunc,
		addWithIDFunc: addWithIDFunc,
	}
}

// GetSettings returns the current routing/settings snapshot used by the lifecycle layer.
func (m *LifecycleManager) GetSettings() *config.Settings {
	m.settingsMu.RLock()
	defer m.settingsMu.RUnlock()
	return m.settings
}

// SaveSettings persists settings and updates the active instance.
func (m *LifecycleManager) SaveSettings(s *config.Settings) error {
	if err := config.SaveSettings(s); err != nil {
		return err
	}
	m.settingsMu.Lock()
	m.settings = s
	m.settingsMu.Unlock()
	return nil
}

// DownloadRequest represents a verified request coming from the UI or API.
type DownloadRequest struct {
	URL                string
	Filename           string
	Path               string
	IsDefaultPath      bool
	Mirrors            []string
	Headers            map[string]string
	IsExplicitCategory bool
	SkipApproval       bool
}

// Enqueue processes a download request and adds it to the engine.
func (mgr *LifecycleManager) Enqueue(ctx context.Context, req *DownloadRequest) (string, error) {
	if mgr.addFunc == nil {
		return "", fmt.Errorf("add function unavailable")
	}

	utils.Debug("Lifecycle: Enqueue %s", req.URL)
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

// EnqueueWithID works identical to Enqueue but passes an explicit requestID to the engine
func (mgr *LifecycleManager) EnqueueWithID(ctx context.Context, req *DownloadRequest, requestID string) (string, error) {
	if mgr.addWithIDFunc == nil {
		return "", fmt.Errorf("addWithID function unavailable")
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
func (mgr *LifecycleManager) enqueueResolved(ctx context.Context, req *DownloadRequest, dispatch func(string, string, *ProbeResult) (string, error)) (string, error) {
	probe, err := ProbeServer(ctx, req.URL, req.Filename, req.Headers)
	if err != nil {
		utils.Debug("Lifecycle: Probe failed: %v\n", err)
		return "", fmt.Errorf("probe failed: %w", err)
	}

	settings := mgr.GetSettings()
	isNameActive := mgr.buildIsNameActive()

	for attempt := 0; attempt < maxWorkingFileReservationAttempts; attempt++ {
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
			return "", fmt.Errorf("failed to resolve destination: %w", err)
		}

		// Reserve the working path before dispatch so a concurrent enqueue has to
		// pick a different name instead of truncating this in-flight download.
		if err := reserveWorkingFile(finalPath, finalFilename); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", err
		}

		surgePath := filepath.Join(finalPath, finalFilename) + types.IncompleteSuffix
		newID, err := dispatch(finalPath, finalFilename, probe)
		if err != nil {
			_ = os.Remove(surgePath)
			return "", err
		}

		return newID, nil
	}

	return "", fmt.Errorf("failed to reserve unique working file for %q after %d attempts", req.URL, maxWorkingFileReservationAttempts)
}
