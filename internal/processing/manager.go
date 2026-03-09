package processing

import (
	"context"
	"fmt"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/utils"
)

// AddDownloadFunc represents a function capable of adding a download to the engine or queue
// url, path, filename, mirrors, headers, isExplicitCategory, totalSize, supportsRange
type AddDownloadFunc func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error)

// AddDownloadWithIDFunc adds a download with a predefined UUID (used by daemon/TUI for syncing)
type AddDownloadWithIDFunc func(string, string, string, []string, map[string]string, string) (string, error)

// LifecycleManager orchestrates the life of a download outside of the core HTTP engine.
// It handles probing, category routing, file conflict resolution, and settings management.
type LifecycleManager struct {
	settings      *config.Settings
	addFunc       AddDownloadFunc
	addWithIDFunc AddDownloadWithIDFunc
}

func NewLifecycleManager(addFunc AddDownloadFunc, addWithIDFunc AddDownloadWithIDFunc) *LifecycleManager {
	// 1. Load Settings immediately on startup as part of the processing layer's responsibility
	settings, err := config.LoadSettings()
	if err != nil {
		// If settings fail to load, fallback to defaults
		settings = config.DefaultSettings()
	}

	return &LifecycleManager{
		settings:      settings,
		addFunc:       addFunc,
		addWithIDFunc: addWithIDFunc,
	}
}

// GetSettings allows the UI to read the current settings (e.g. for the Settings view).
func (m *LifecycleManager) GetSettings() *config.Settings {
	return m.settings
}

// SaveSettings persists settings and updates the active instance.
func (m *LifecycleManager) SaveSettings(s *config.Settings) error {
	if err := config.SaveSettings(s); err != nil {
		return err
	}
	m.settings = s
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

	// 1. Probe the Server
	// We extract probing entirely into the processing layer before adding to the engine pool.
	probe, err := ProbeServer(ctx, req.URL, req.Filename, req.Headers)
	if err != nil {
		utils.Debug("Lifecycle: Probe failed: %v\n", err)
		return "", fmt.Errorf("probe failed: %w", err)
	}

	// 2. Routing and Filename Resolution
	// For now we just assume uniqueness is mostly handled by TUI/API duplicate checker first
	isNameActive := func(name string) bool { return false } // TODO: connect to global pool

	finalPath, finalFilename, err := ResolveDestination(
		req.URL,
		req.Filename,
		req.Path,
		!req.IsExplicitCategory, // if it's explicitly set by user, we skip routing
		mgr.settings,
		probe,
		isNameActive,
	)

	if err != nil {
		return "", fmt.Errorf("failed to resolve destination: %w", err)
	}

	// 3. Dispatch to Engine
	// The Engine no longer probes or thinks. It just downloads what it's told.
	newID, err := mgr.addFunc(
		req.URL,
		finalPath,
		finalFilename,
		req.Mirrors,
		req.Headers,
		req.IsExplicitCategory,
		probe.FileSize,
		probe.SupportsRange,
	)

	if err != nil {
		return "", err
	}
	
	return newID, nil
}

// EnqueueWithID works identical to Enqueue but passes an explicit requestID to the engine
func (mgr *LifecycleManager) EnqueueWithID(ctx context.Context, req *DownloadRequest, requestID string) (string, error) {
	if mgr.addWithIDFunc == nil {
		return "", fmt.Errorf("addWithID function unavailable")
	}

	utils.Debug("Lifecycle: EnqueueWithID %s (%s)", req.URL, requestID)

	probe, err := ProbeServer(ctx, req.URL, req.Filename, req.Headers)
	if err != nil {
		return "", fmt.Errorf("probe failed: %w", err)
	}

	isNameActive := func(name string) bool { return false }

	finalPath, finalFilename, err := ResolveDestination(
		req.URL, req.Filename, req.Path,
		!req.IsExplicitCategory,
		mgr.settings, probe, isNameActive,
	)
	if err != nil {
		return "", fmt.Errorf("failed to resolve destination: %w", err)
	}

	newID, err := mgr.addWithIDFunc(
		req.URL,
		finalPath,
		finalFilename,
		req.Mirrors,
		req.Headers,
		requestID,
	)
	
	if err != nil {
		return "", err
	}
	
	return newID, nil
}
