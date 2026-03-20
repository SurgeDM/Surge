package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/utils"
)

func runStartupIntegrityCheck() string {
	// Normalize downloads stuck in "downloading" status from a prior crash/kill.
	// This must happen before ValidateIntegrity so the newly-paused entries
	// are included in the integrity check and eligible for auto-resume.
	if normalized, err := state.NormalizeStaleDownloads(); err != nil {
		msg := fmt.Sprintf("Startup: failed to normalize stale downloads: %v", err)
		utils.Debug("%s", msg)
	} else if normalized > 0 {
		utils.Debug("Startup: normalized %d stale downloading entries to paused", normalized)
	}

	// Validate integrity of paused/queued downloads before auto-resume.
	// This removes entries whose .surge files are missing/tampered and
	// also cleans orphan .surge files that no longer have DB entries.
	if removed, err := state.ValidateIntegrity(); err != nil {
		msg := fmt.Sprintf("Startup integrity check failed: %v", err)
		return msg
	} else if removed > 0 {
		msg := fmt.Sprintf("Startup integrity check: removed %d corrupted/orphaned downloads", removed)
		return msg
	}
	utils.Debug("%s", "Startup integrity check: no issues found")
	return ""
}

func mustInitializeGlobalState() {
	if err := initializeGlobalState(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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
	state.Configure(stateDBPath)

	// Config logging
	utils.ConfigureDebug(logsDir)

	// Clean up old logs
	retention := getSettings().General.LogRetentionCount
	utils.CleanupLogs(retention)
	return nil
}

func getSettings() *config.Settings {
	if globalSettings != nil {
		return globalSettings
	}
	settings, err := config.LoadSettings()
	if err != nil {
		return config.DefaultSettings()
	}
	return settings
}

func resumePausedDownloads() {
	settings := getSettings()

	pausedEntries, err := state.LoadPausedDownloads()
	if err != nil {
		return
	}

	for _, entry := range pausedEntries {
		// If entry is explicitly queued, we should start it regardless of AutoResume setting
		// If entry is paused, we only start it if AutoResume is enabled
		if entry.Status == "paused" && !settings.General.AutoResume {
			continue
		}
		if GlobalService == nil || entry.ID == "" {
			continue
		}
		if err := GlobalService.Resume(entry.ID); err == nil {
			atomic.AddInt32(&activeDownloads, 1)
		}
	}
}
