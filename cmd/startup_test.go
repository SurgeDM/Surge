package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/utils"
)

// TestServer_Startup_HandlesResume verifies that resumePausedDownloads() works for server mode
func TestServer_Startup_HandlesResume(t *testing.T) {
	// 1. Setup Environment
	tmpDir, err := os.MkdirTemp("", "surge-server-startup-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	setupTestEnv(t, tmpDir)

	// 2. Seed DB with 'queued' download
	testID := "server-resume-id"
	testURL := "http://example.com/server-resume.zip"
	testDest := filepath.Join(tmpDir, "server-resume.zip")
	seedDownload(t, testID, testURL, testDest, "queued")

	// 3. Initialize Global Pool (required for resumePausedDownloads)
	GlobalProgressCh = make(chan any, 10)
	GlobalPool = download.NewWorkerPool(GlobalProgressCh, 3)
	GlobalService = core.NewLocalDownloadServiceWithInput(GlobalPool, GlobalProgressCh)

	GlobalLifecycle = processing.NewLifecycleManager(nil, nil, nil)
	GlobalLifecycle.SetEngineHooks(processing.EngineHooks{
		Pause:               GlobalPool.Pause,
		ExtractPausedConfig: GlobalPool.ExtractPausedConfig,
		AddConfig:           GlobalPool.Add,
		GetStatus:           GlobalPool.GetStatus,
		Cancel:              GlobalPool.Cancel,
		UpdateURL:           GlobalPool.UpdateURL,
		PublishEvent:        GlobalService.Publish,
	})
	if svc, ok := GlobalService.(*core.LocalDownloadService); ok {
		svc.SetLifecycleHooks(core.LifecycleHooks{
			Pause:       GlobalLifecycle.Pause,
			Resume:      GlobalLifecycle.Resume,
			ResumeBatch: GlobalLifecycle.ResumeBatch,
			Cancel:      GlobalLifecycle.Cancel,
			UpdateURL:   GlobalLifecycle.UpdateURL,
		})
	}
	defer func() {
		GlobalLifecycle = nil
	}()

	// 4. Run Resume Logic (Simulate Server Start)
	resumePausedDownloads()

	// 5. Verify Download is in GlobalPool
	status := GlobalPool.GetStatus(testID)
	// GetStatus checks active downloads. If it returned non-nil, it's active!
	if status == nil {
		// Check if it's in queued map (GetStatus checks both active and queued internal maps)
		// Wait, GetStatus implementation in pool.go checks p.downloads and p.queued
		t.Fatal("Download not found in GlobalPool after resumePausedDownloads()")
	}

	if status.Status != "queued" && status.Status != "downloading" {
		t.Errorf("Expected status queued/downloading, got %s", status.Status)
	}
}

func TestStartupIntegrityCheck_RemovesMissingPausedEntry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "surge-startup-integrity-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	setupTestEnv(t, tmpDir)

	testID := "startup-integrity-missing-id"
	testURL := "http://example.com/startup-integrity.bin"
	testDest := filepath.Join(tmpDir, "startup-integrity.bin")
	seedDownload(t, testID, testURL, testDest, "paused")

	// Ensure .surge file is missing to simulate an orphaned paused DB entry.
	if err := os.Remove(testDest + types.IncompleteSuffix); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove test .surge file: %v", err)
	}

	msg := runStartupIntegrityCheck()
	utils.Debug("%s", msg)

	entry, err := state.GetDownload(testID)
	if err != nil {
		t.Fatalf("GetDownload failed: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected missing paused entry to be removed, got %+v", entry)
	}
}

// Helper: Setup XDG_CONFIG_HOME and Settings
func setupTestEnv(t *testing.T, tmpDir string) {
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Cleanup(func() {
		if originalXDG == "" {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			_ = os.Setenv("XDG_CONFIG_HOME", originalXDG)
		}
	})

	surgeDir := config.GetSurgeDir()
	if err := os.MkdirAll(surgeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Setup Settings (AutoResume=false default)
	settings := config.DefaultSettings()
	settings.General.AutoResume = false // Ensure we test that "queued" overrides this
	if err := config.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	// Configure DB
	dbPath := filepath.Join(surgeDir, "state", "surge.db")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	state.CloseDB()
	state.Configure(dbPath)
}

func seedDownload(t *testing.T, id, url, dest, status string) {
	manualState := &types.DownloadState{
		ID:         id,
		URL:        url,
		Filename:   filepath.Base(dest),
		DestPath:   dest,
		TotalSize:  1000,
		Downloaded: 0,
		PausedAt:   0,
		CreatedAt:  time.Now().Unix(),
	}
	if err := state.SaveState(url, dest, manualState); err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateStatus(id, status); err != nil {
		t.Fatal(err)
	}
}
