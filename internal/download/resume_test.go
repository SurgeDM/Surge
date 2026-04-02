package download_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/google/uuid"
)

func TestIntegration_PauseResume(t *testing.T) {
	// 1. Setup temporary directory for DB and downloads
	tmpDir, err := os.MkdirTemp("", "surge-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Set XDG_CONFIG_HOME to tmpDir so state.GetDB() creates DB there
	// The config package uses "surge" subdirectory
	configDir := tmpDir // XDG_CONFIG_HOME usually contains the app dir
	t.Setenv("XDG_CONFIG_HOME", configDir)

	// Ensure clean state
	state.CloseDB()

	// Force DB init
	dbPath := filepath.Join(tmpDir, "surge.db")
	state.Configure(dbPath)
	if _, err := state.GetDB(); err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer state.CloseDB()

	// 2. Setup Mock Server (500MB file)
	fileSize := int64(500 * 1024 * 1024) // 500MB
	server := testutil.NewStreamingMockServerT(t,
		fileSize,
		testutil.WithRangeSupport(true),
		testutil.WithLatency(10*time.Millisecond), // Small latency to allow interruption
	)
	defer server.Close()

	url := server.URL()
	// Use a fixed filename to make checking easier
	filename := "largefile.bin"
	outputPath := tmpDir
	destPath := filepath.Join(outputPath, filename)

	// 3. Start Download and Interrupt
	ctx := context.Background()
	progressCh := make(chan any, 100)
	runtime := &types.RuntimeConfig{}
	// DB/state persistence now lives in processing event worker.
	mgr := processing.NewLifecycleManager(nil, nil)
	var eventWG sync.WaitGroup
	eventWG.Add(1)
	go func() {
		defer eventWG.Done()
		mgr.StartEventWorker(progressCh)
	}()
	defer func() {
		close(progressCh)
		eventWG.Wait()
	}()

	progState := types.NewProgressState(uuid.New().String(), fileSize)

	cfg := types.DownloadConfig{
		URL:           url,
		OutputPath:    outputPath,
		Filename:      filename,
		ID:            progState.ID,
		ProgressCh:    progressCh,
		State:         progState,
		Runtime:       runtime,
		TotalSize:     fileSize,
		SupportsRange: true,
		IsResume:      false,
	}

	// Pre-create incomplete file (simulating processing layer)
	incompletePath := destPath + types.IncompleteSuffix
	f, err := os.Create(incompletePath)
	if err != nil {
		t.Fatalf("Failed to pre-create partial file: %v", err)
	}
	_ = f.Close()

	// Start download
	errCh := make(chan error)
	go func() {
		errCh <- download.TUIDownload(ctx, &cfg)
	}()

	// Wait for some progress
	deadline := time.Now().Add(15 * time.Second)
	progressed := false
	for time.Now().Before(deadline) {
		if progState.Downloaded.Load() > 0 {
			progressed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !progressed {
		t.Fatal("download did not make initial progress before pause")
	}

	// Interrupt!
	progState.Pause()

	// Wait for download to return
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled && !errors.Is(err, types.ErrPaused) {
			t.Logf("Download returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Download did not return after cancellation")
	}

	// 4. Verify State is Saved (event worker persists asynchronously)
	var savedState *types.DownloadState
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		savedState, err = state.LoadState(url, destPath)
		if err == nil && savedState != nil && savedState.Downloaded > 0 && len(savedState.Tasks) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Failed to load saved state: %v", err)
	}

	if savedState.Downloaded == 0 {
		t.Error("Saved state shows 0 downloaded bytes")
	}
	if savedState.Downloaded >= fileSize {
		t.Errorf("Download finished too fast! Downloaded %d of %d", savedState.Downloaded, fileSize)
	}
	if len(savedState.Tasks) == 0 {
		t.Error("Saved state has no tasks")
	}

	// Verify .surge file exists
	incompletePath = destPath + types.IncompleteSuffix
	info, err := os.Stat(incompletePath)
	if err != nil {
		t.Fatalf("Incomplete file not found: %v", err)
	}
	if info.Size() != fileSize {
		// Note: we preallocate file size, so it should match total size
		t.Errorf("Incomplete file size = %d, want %d", info.Size(), fileSize)
	}

	t.Logf("Paused successfully. Downloaded: %d bytes", savedState.Downloaded)

	// 5. Resume Download
	// Create new context
	resumeCtx, resumeCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer resumeCancel()

	// Update config for resume
	cfg.IsResume = true
	cfg.DestPath = destPath // Important for resume lookup
	cfg.SavedState = savedState

	// Reset pause flag before resume
	progState.Resume()

	err = download.TUIDownload(resumeCtx, &cfg)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	// 6. Verify Completion (event worker finalizes rename/status asynchronously)
	deadline = time.Now().Add(5 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		_, surgeErr := os.Stat(incompletePath)
		finalInfo, finalErr := os.Stat(destPath)
		entry, _ := state.GetDownload(cfg.ID)
		if os.IsNotExist(surgeErr) && finalErr == nil && finalInfo.Size() == fileSize && entry != nil && entry.Status == "completed" {
			completed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !completed {
		t.Fatal("resume did not reach finalized completed state before timeout")
	}

	if _, err := os.Stat(incompletePath); !os.IsNotExist(err) {
		t.Error("Incomplete file still exists after resume completion")
	}
	finalInfo, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Final file not found: %v", err)
	}
	if finalInfo.Size() != fileSize {
		t.Errorf("Final file size = %d, want %d", finalInfo.Size(), fileSize)
	}
	entry, _ := state.GetDownload(cfg.ID)
	if entry == nil || entry.Status != "completed" {
		t.Fatalf("download entry not marked completed, got %+v", entry)
	}
}
