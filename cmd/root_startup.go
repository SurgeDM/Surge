package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// initializeLocalEngine is called only by local TUI/server owners, after the
// instance lock is acquired. Client and metadata commands never create a pool.
func initializeLocalEngine() error {
	if GlobalService != nil {
		if err := GlobalService.Shutdown(); err != nil {
			return fmt.Errorf("stop previous local service: %w", err)
		}
	}
	if lifecycle := currentLifecycle(); lifecycle != nil {
		lifecycle.Shutdown()
	}
	if GlobalPool != nil {
		GlobalPool.GracefulShutdown()
	}
	if cleanup := takeLifecycleCleanup(); cleanup != nil {
		cleanup()
	}
	globalLifecycleMu.Lock()
	GlobalService = nil
	GlobalLifecycle = nil
	globalLifecycleMu.Unlock()
	GlobalPool = nil
	GlobalProgressCh = nil
	globalHTTPServerMu.Lock()
	server := globalHTTPServer
	globalHTTPServer = nil
	globalHTTPServerMu.Unlock()
	if server != nil {
		_ = server.Close()
	}

	setGlobalSettings(nil)
	settings := getSettings()
	setGlobalSettings(settings)
	if err := initializeGlobalState(); err != nil {
		return err
	}
	GlobalProgressCh = make(chan types.DownloadEvent, types.ProgressChannelBuffer)
	GlobalPool = scheduler.New(GlobalProgressCh, config.Resolve[int](settings.Network.MaxConcurrentDownloads))
	return nil
}

// initializeClientState prepares local history/auth/logging only when needed.
// Explicit remote clients use the configured endpoint without touching local
// settings, state directories, history, or a local download engine.
func initializeClientState() error {
	if resolveHostTarget() != "" {
		return nil
	}
	return initializeGlobalState()
}

func runStartupIntegrityCheck() string {
	// Normalize downloads stuck in "downloading" status from a prior crash/kill.
	// This must happen before ValidateIntegrity so the newly-paused entries
	// are included in the integrity check and eligible for auto-resume.
	if normalized, err := store.NormalizeStaleDownloads(); err != nil {
		msg := fmt.Sprintf("Startup: failed to normalize stale downloads: %v", err)
		utils.Debug("%s", msg)
	} else if normalized > 0 {
		utils.Debug("Startup: normalized %d stale downloading entries to paused", normalized)
	}

	// Validate integrity of paused/queued downloads before auto-resume.
	// This removes entries whose .surge files are missing/tampered and
	// also cleans orphan .surge files that no longer have DB entries.
	if removed, err := store.ValidateIntegrity(); err != nil {
		msg := fmt.Sprintf("Startup integrity check failed: %v", err)
		return msg
	} else if removed > 0 {
		msg := fmt.Sprintf("Startup integrity check: removed %d corrupted/orphaned downloads", removed)
		return msg
	}
	utils.Debug("%s", "Startup integrity check: no issues found")
	return ""
}

// initializeGlobalState sets up the environment and configures the engine state and logging
func initializeGlobalState() error {
	stateDir := config.GetStateDir()
	logsDir := config.GetLogsDir()
	stateDBPath := filepath.Join(stateDir, "surge.db")

	if err := config.EnsureDirs(); err != nil {
		return fmt.Errorf("failed to create surge directories: %w", err)
	}

	// Config engine state
	store.Configure(stateDBPath)

	// Config logging
	utils.ConfigureDebug(logsDir)
	utils.Debug("Surge %s (commit %s)", Version, Commit)

	// Clean up old logs (keeping retention-1 because a new log will be created immediately after)
	retention := config.Resolve[int](getSettings().General.LogRetentionCount)
	if retention > 0 {
		utils.CleanupLogs(retention - 1)
	} else {
		utils.CleanupLogs(retention)
	}
	return nil
}

func getSettings() *config.Settings {
	if owner, ok := GlobalService.(service.SettingsService); ok {
		if settings, err := owner.GetSettings(); err == nil {
			return settings
		}
	}
	if settings := getGlobalSettings(); settings != nil {
		return settings
	}
	settings, err := config.LoadSettings()
	if err != nil {
		utils.Debug("Warning: failed to load settings: %v - using defaults", err)
		defaults := config.DefaultSettings()
		defaults.StartupWarnings = append(defaults.StartupWarnings,
			fmt.Sprintf("Config: failed to read settings file (%v) - all settings reset to defaults", err))
		return defaults
	}
	return settings
}

func getGlobalSettings() *config.Settings {
	globalSettingsMu.RLock()
	defer globalSettingsMu.RUnlock()
	return globalSettings.Clone()
}

func setGlobalSettings(settings *config.Settings) {
	globalSettingsMu.Lock()
	globalSettings = settings.Clone()
	globalSettingsMu.Unlock()
}

func resumePausedDownloads() {
	settings := getSettings()

	pausedEntries, err := store.LoadPausedDownloads()
	if err != nil {
		return
	}

	for _, entry := range pausedEntries {
		// If entry is explicitly queued, we should start it regardless of AutoResume setting
		// If entry is paused, we only start it if AutoResume is enabled
		if entry.Status == "paused" && !config.Resolve[bool](settings.General.AutoResume) {
			continue
		}
		if GlobalService == nil || entry.ID == "" {
			continue
		}
		if err := GlobalService.Resume(entry.ID); err != nil {
			utils.Debug("Failed to resume download %s at startup: %v", entry.ID, err)
		}
	}
}
