package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestCmd_AutoResume_Execution(t *testing.T) {
	// 1. Setup Environment
	tmpDir, err := os.MkdirTemp("", "surge-cmd-resume-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("APPDATA", tmpDir)

	surgeDir := config.GetSurgeDir()
	if err := os.MkdirAll(surgeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	settings := config.DefaultSettings()
	settings.General.AutoResume.Value = true
	settings.General.DefaultDownloadDir.Value = tmpDir

	if err := config.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	// 3. Configure State DB
	store.CloseDB() // Ensure clean state
	dbPath := filepath.Join(surgeDir, "state", "surge.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	store.Configure(dbPath)

	// 4. Seed DB with a paused download
	testID := "cmd-resume-id-1"
	testURL := "http://example.com/cmd-resume.zip"
	testDest := filepath.Join(tmpDir, "cmd-resume.zip")

	manualState := &types.DownloadRecord{
		ID:         testID,
		URL:        testURL,
		Filename:   "cmd-resume.zip",
		DestPath:   testDest,
		TotalSize:  1000,
		Downloaded: 500,
		PausedAt:   time.Now().Unix(),
		CreatedAt:  time.Now().Unix(),
	}
	if err := store.AddToMasterList(types.DownloadRecord{
		ID:         testID,
		URL:        testURL,
		DestPath:   testDest,
		Filename:   "cmd-resume.zip",
		Status:     "paused",
		TotalSize:  1000,
		Downloaded: 500,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState(testURL, testDest, manualState); err != nil {
		t.Fatal(err)
	}

	// 5. Initialize GlobalPool + GlobalService
	GlobalProgressCh = make(chan types.DownloadEvent, 10)
	if GlobalPool != nil {
		GlobalPool.GracefulShutdown()
	}
	tmpPool := scheduler.New(GlobalProgressCh, 4)
	t.Cleanup(func() {
		if tmpPool != nil {
			tmpPool.GracefulShutdown()
		}
	})
	GlobalPool = tmpPool

	eventBus := orchestrator.NewEventBus()
	t.Cleanup(func() { eventBus.Shutdown() })
	getAll := func() []types.DownloadRecord { return GlobalPool.GetAll() }
	tmpLifecycle := orchestrator.NewLifecycleManager(GlobalPool, eventBus, nil, buildActiveDownloadChecker(getAll))
	t.Cleanup(func() { tmpLifecycle.Shutdown() })
	GlobalLifecycle = tmpLifecycle
	GlobalService = service.NewLocalDownloadService(GlobalLifecycle)

	defer func() {
		_ = GlobalService.Shutdown()
		GlobalService = nil
		GlobalPool = nil
		GlobalLifecycle = nil
	}()

	// 6. Call the function
	resumePausedDownloads()

	// 7. Verify
	// Check if GlobalPool has the resumed download by ID.
	if GlobalPool.GetStatus(testID) == nil {
		t.Error("Download was not added to GlobalPool by resumePausedDownloads")
	}
}
