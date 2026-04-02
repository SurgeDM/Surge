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
	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/google/uuid"
)

func TestIntegration_MirrorResume(t *testing.T) {
	// 1. Setup temporary directory for DB and downloads
	tmpDir, err := os.MkdirTemp("", "surge-mirror-resume-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Set XDG_CONFIG_HOME to tmpDir so state.GetDB() creates DB there
	// The config package uses "surge" subdirectory
	configDir := tmpDir // XDG_CONFIG_HOME usually contains the app dir
	t.Setenv("XDG_CONFIG_HOME", configDir)

	// Configure debug
	utils.ConfigureDebug(tmpDir)

	// Ensure clean state
	state.CloseDB()
	dbPath := filepath.Join(tmpDir, "surge.db")
	state.Configure(dbPath)
	if _, err := state.GetDB(); err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer state.CloseDB()

	// 2. Setup Mock Servers (Primary + Mirror)
	fileSize := int64(200 * 1024 * 1024) // 200MB
	primary := testutil.NewStreamingMockServerT(t,
		fileSize,
		testutil.WithRangeSupport(true),
		testutil.WithByteLatency(20*time.Microsecond), // Slow down to ensure we can pause
	)
	defer primary.Close()

	mirror := testutil.NewStreamingMockServerT(t,
		fileSize,
		testutil.WithRangeSupport(true),
		testutil.WithByteLatency(20*time.Microsecond),
	)
	defer mirror.Close()

	// 3. Start Download with Mirror
	ctx1 := context.Background()
	progressCh := make(chan any, 100)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 4,
	}
	// Wire event persistence worker because pause state is persisted in processing layer.
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

	filename := "mirrorfile.bin"
	outputPath := tmpDir
	destPath := filepath.Join(outputPath, filename)

	cfg := types.DownloadConfig{
		URL:           primary.URL(),
		OutputPath:    outputPath,
		Filename:      filename,
		ID:            progState.ID,
		ProgressCh:    progressCh,
		State:         progState,
		Runtime:       runtime,
		TotalSize:     fileSize,
		SupportsRange: true,
		IsResume:      false,
		Mirrors:       []string{mirror.URL()}, // Pass mirror
	}

	// Pre-create incomplete file (simulating processing layer)
	incompletePath := destPath + types.IncompleteSuffix
	f, err := os.Create(incompletePath)
	if err != nil {
		t.Fatalf("Failed to pre-create partial file: %v", err)
	}
	_ = f.Close()

	// Start download and interrupt
	errCh := make(chan error)
	go func() {
		errCh <- download.TUIDownload(ctx1, &cfg)
	}()

	// Wait until download really started so Pause() has an attached cancel func.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if progState.Downloaded.Load() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if progState.Downloaded.Load() == 0 {
		t.Fatal("download did not make initial progress before pause")
	}

	// Interrupt!
	progState.Pause()

	// Wait for return
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, types.ErrPaused) {
			t.Fatalf("unexpected pause result: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Download did not return")
	}

	// 4. Verify Mirrors Saved (event worker persists state asynchronously)
	var savedState *types.DownloadState
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		savedState, err = state.LoadState(primary.URL(), destPath)
		if err == nil && savedState != nil && len(savedState.Mirrors) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || savedState == nil || len(savedState.Mirrors) == 0 {
		// Print debug metadata
		entries, _ := os.ReadDir(tmpDir)
		for _, e := range entries {
			if !e.IsDir() {
				info, statErr := os.Stat(filepath.Join(tmpDir, e.Name()))
				if statErr == nil {
					t.Logf("File %s exists (size=%d)", e.Name(), info.Size())
				} else {
					t.Logf("File %s exists", e.Name())
				}
			}
		}
	}
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if savedState == nil || len(savedState.Mirrors) == 0 {
		t.Fatal("Mirrors not saved in state!")
	}
	if savedState.Mirrors[0] != mirror.URL() {
		t.Errorf("Saved mirror mismatch. Want %s, got %v", mirror.URL(), savedState.Mirrors)
	}

	// 5. Resume without explicit mirrors
	// Create new config simulating a resumption where we don't know the mirrors initially.
	// Resume now receives preloaded state from the caller.
	resumeState := types.NewProgressState(savedState.ID, fileSize)
	resumeCfg := types.DownloadConfig{
		URL:           primary.URL(),
		OutputPath:    outputPath,
		Filename:      filename,
		ID:            savedState.ID,
		ProgressCh:    progressCh,
		State:         resumeState,
		Runtime:       runtime,
		TotalSize:     fileSize,
		SupportsRange: true,
		IsResume:      true,
		DestPath:      destPath,
		SavedState:    savedState,
		Mirrors:       []string{}, // Empty mirrors!
	}

	// We can't easily hook into TUIDownload to verify it loaded mirrors without running it.
	ctx2 := context.Background()
	go func() {
		errCh <- download.TUIDownload(ctx2, &resumeCfg)
	}()

	// Give it enough time to start and restore mirrors from saved state.
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if len(resumeState.GetMirrors()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	resumeState.Pause()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, types.ErrPaused) {
			t.Fatalf("unexpected resume pause result: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("resumed download did not return after pause")
	}

	// Check if resume state has mirrors
	stateMirrors := resumeState.GetMirrors()
	if len(stateMirrors) == 0 {
		t.Fatal("resume state mirrors were not updated from saved state")
	}

	found := false
	for _, m := range stateMirrors {
		if m.URL == mirror.URL() {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Resume state mirrors missing secondary. Got %v, want to include %s", stateMirrors, mirror.URL())
	}
}
