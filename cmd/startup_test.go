package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
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
	GlobalProgressCh = make(chan types.DownloadEvent, 10)
	GlobalPool = scheduler.New(GlobalProgressCh, 3)
	eventBus := orchestrator.NewEventBus()
	getAll := func() []types.DownloadRecord { return GlobalPool.GetAll() }
	GlobalLifecycle = orchestrator.NewLifecycleManager(GlobalPool, eventBus, nil, buildActiveDownloadChecker(getAll))
	GlobalService = service.NewLocalDownloadService(GlobalLifecycle)
	defer func() {
		if GlobalService != nil {
			_ = GlobalService.Shutdown()
		}
		GlobalService = nil
		GlobalPool = nil
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
		return
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

	entry, err := store.GetDownload(testID)
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
	settings.General.AutoResume.Value = false // Ensure we test that "queued" overrides this
	if err := config.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	// Configure DB
	dbPath := filepath.Join(surgeDir, "state", "surge.db")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	store.CloseDB()
	store.Configure(dbPath)
}

func seedDownload(t *testing.T, id, url, dest, status string) {
	manualState := &types.DownloadRecord{
		ID:         id,
		URL:        url,
		Filename:   filepath.Base(dest),
		DestPath:   dest,
		TotalSize:  1000,
		Downloaded: 0,
		PausedAt:   0,
		CreatedAt:  time.Now().Unix(),
	}
	if err := store.AddToMasterList(types.DownloadRecord{
		ID:         id,
		URL:        url,
		DestPath:   dest,
		Filename:   filepath.Base(dest),
		Status:     status,
		TotalSize:  1000,
		Downloaded: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState(url, dest, manualState); err != nil {
		t.Fatal(err)
	}
}

func TestGetSettings_LoadError_PopulatesStartupWarnings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on windows")
	}

	tmpDir, err := os.MkdirTemp("", "surge-getsettings-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer func() {
		if originalXDG == "" {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			_ = os.Setenv("XDG_CONFIG_HOME", originalXDG)
		}
	}()

	// Force an error when reading settings.toml by creating a directory with its name
	surgeDir := config.GetSurgeDir()
	if err := os.MkdirAll(surgeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.GetSettingsPath(), 0o755); err != nil {
		t.Fatal(err)
	}

	globalSettings = nil // Ensure we don't return cached settings
	defer func() { globalSettings = nil }()

	settings := getSettings()
	if len(settings.StartupWarnings) == 0 {
		t.Error("expected StartupWarnings when LoadSettings fails")
	}
}
