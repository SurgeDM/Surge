package concurrent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

func TestHandlePause_CompletionBoundary(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "test.bin")
	state := types.NewProgressState("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:    "test-id",
		State: state,
	}

	queue := NewTaskQueue()
	// No tasks in queue means remainingBytes == 0

	err := downloader.handlePause(destPath, fileSize, queue, nil)
	if err != nil {
		t.Fatalf("handlePause returned error on completion boundary: %v", err)
	}

	if state.IsPaused() {
		t.Errorf("State should not be paused on completion boundary")
	}
}

func TestHandlePause_Normal(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "test.bin")
	state := types.NewProgressState("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:    "test-id",
		State: state,
	}

	queue := NewTaskQueue()
	queue.Push(types.Task{Offset: 500, Length: 500})

	err := downloader.handlePause(destPath, fileSize, queue, nil)
	if err != types.ErrPaused {
		t.Fatalf("Expected ErrPaused, got %v", err)
	}
}

func TestSetupTasks_NewDownload(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	chunkSize := int64(500)
	destPath := filepath.Join(tmpDir, "new.bin")
	workingPath := destPath + types.IncompleteSuffix

	f, err := os.Create(workingPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	state := types.NewProgressState("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:      "test-id",
		State:   state,
		Runtime: &types.RuntimeConfig{},
	}

	tasks, err := downloader.setupTasks(destPath, fileSize, chunkSize, f)
	if err != nil {
		t.Fatalf("setupTasks failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}
}

func TestGetWorkerMirrors(t *testing.T) {
	d := &ConcurrentDownloader{URL: "http://primary.com"}
	active := []string{"http://primary.com", "http://mirror1.com", "http://mirror2.com"}

	mirrors := d.getWorkerMirrors(active)

	if len(mirrors) != 3 {
		t.Errorf("Expected 3 mirrors, got %d", len(mirrors))
	}
	if mirrors[0] != "http://primary.com" {
		t.Errorf("Primary URL should be first, got %s", mirrors[0])
	}
}

func TestInitMirrorStatus(t *testing.T) {
	state := types.NewProgressState("test-id", 1000)
	d := &ConcurrentDownloader{ID: "test-id", State: state}

	primary := "http://primary.com"
	candidates := []string{"http://mirror1.com", "http://mirror2.com"}
	active := []string{"http://primary.com", "http://mirror1.com"}

	d.initMirrorStatus(primary, candidates, active, "/path/to/dest")

	statuses := state.GetMirrors()
	if len(statuses) != 3 {
		t.Errorf("Expected 3 statuses, got %d", len(statuses))
	}

	foundMirror2 := false
	for _, s := range statuses {
		if s.URL == "http://mirror2.com" {
			foundMirror2 = true
			if s.Active {
				t.Error("Mirror2 should be inactive")
			}
			if !s.Error {
				t.Error("Mirror2 should have error (as it is not active)")
			}
		}
	}
	if !foundMirror2 {
		t.Error("Mirror2 status not found")
	}
}

func TestSetupTasks_BitmapRestoration(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	chunkSize := int64(100)
	destPath := filepath.Join(tmpDir, "resume.bin")

	// Create a saved state
	savedBitmap := []byte{0xFF, 0x00, 0x00} // 10 chunks need 3 bytes
	savedState := &types.DownloadState{
		ID:              "test-id",
		URL:             "http://example.com",
		DestPath:        destPath,
		TotalSize:       fileSize,
		Downloaded:      500,
		ActualChunkSize: chunkSize,
		ChunkBitmap:     savedBitmap,
		Tasks:           []types.Task{{Offset: 500, Length: 500}},
	}
	// Note: state.SaveState and state.LoadState are already used in the codebase.
	if err := state.SaveState("http://example.com", destPath, savedState); err != nil {
		t.Fatal(err)
	}

	f, _ := os.Create(destPath + types.IncompleteSuffix)
	defer func() { _ = f.Close() }()

	progState := types.NewProgressState("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:    "test-id",
		URL:   "http://example.com",
		State: progState,
	}

	// This simulates the fixed order in Download():
	// 1. InitBitmap
	progState.InitBitmap(fileSize, chunkSize)
	// 2. setupTasks (which calls RestoreBitmap)
	_, err := downloader.setupTasks(destPath, fileSize, chunkSize, f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify bitmap is NOT empty (it should have the restored data)
	bitmap, _, _, _, _ := progState.GetBitmapSnapshot(false)
	if len(bitmap) == 0 {
		t.Error("Bitmap should have been restored, but it is empty")
	}
	if bitmap[0] != 0xAA {
		t.Errorf("Bitmap[0] should be 0xAA (all chunks completed), got 0x%02X", bitmap[0])
	}
}

func TestHandlePause_CompletionFinalization(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "test.bin")
	progState := types.NewProgressState("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:    "test-id",
		State: progState,
	}

	queue := NewTaskQueue()
	// No tasks left

	err := downloader.handlePause(destPath, fileSize, queue, nil)
	if err != nil {
		t.Errorf("Expected nil error for completion boundary, got %v", err)
	}

	if progState.IsPaused() {
		t.Error("Should have resumed state for completion")
	}
}
