package concurrent

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestHandlePause_RemainingZeroButVPLessThanFileSize_SavesStateNotFinalize(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "vp_guard.bin")
	progState := progress.New("vp-guard", fileSize)
	progState.Bytes.VerifiedProgress.Store(500)

	d := &ConcurrentDownloader{
		ID:      "vp-guard",
		State:   progState,
		Runtime: &types.RuntimeConfig{},
	}

	queue := NewTaskQueue()
	err := d.handlePause(destPath, fileSize, queue, nil)
	if !errors.Is(err, types.ErrPaused) {
		t.Fatalf("expected ErrPaused (save state for resume), got %v", err)
	}
	if got := progState.Bytes.VerifiedProgress.Load(); got != 500 {
		t.Fatalf("VP = %d, want 500 (must not raise to fileSize)", got)
	}
}

func TestHandlePause_RemainingZeroAndVPEqualsFileSize_Finalizes(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "vp_equal.bin")
	progState := progress.New("vp-equal", fileSize)
	progState.Bytes.VerifiedProgress.Store(fileSize)

	d := &ConcurrentDownloader{
		ID:      "vp-equal",
		State:   progState,
		Runtime: &types.RuntimeConfig{},
	}

	err := d.handlePause(destPath, fileSize, NewTaskQueue(), nil)
	if err != nil {
		t.Fatalf("expected nil (finalize), got %v", err)
	}
	if progState.IsPaused() {
		t.Error("state should not be paused — should be finalized as completed")
	}
}

func TestHandlePause_RebuildsTasksFromBitmap(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	const fileSize, chunkSize int64 = 1000, 250
	state := progress.New("bitmap-resume", fileSize)
	state.InitBitmap(fileSize, chunkSize)
	state.UpdateChunkStatus(0, chunkSize, types.ChunkCompleted)
	state.UpdateChunkStatus(2*chunkSize, chunkSize, types.ChunkCompleted)

	d := &ConcurrentDownloader{ID: "bitmap-resume", State: state, Runtime: &types.RuntimeConfig{}}
	if err := d.handlePause(filepath.Join(tmpDir, "bitmap-resume.bin"), fileSize, NewTaskQueue(), nil); !errors.Is(err, types.ErrPaused) {
		t.Fatalf("handlePause = %v, want ErrPaused", err)
	}

	snapshot := state.TakePendingResumeState()
	if snapshot == nil {
		t.Fatal("missing resume snapshot")
	}
	want := []types.Task{{Offset: chunkSize, Length: chunkSize}, {Offset: 3 * chunkSize, Length: chunkSize}}
	if !reflect.DeepEqual(snapshot.Tasks, want) {
		t.Fatalf("resume tasks = %+v, want %+v", snapshot.Tasks, want)
	}
}

func TestHandlePause_CompleteBitmapDoesNotSaveIncompleteState(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	const fileSize, chunkSize int64 = 1000, 250
	state := progress.New("bitmap-complete", fileSize)
	state.InitBitmap(fileSize, chunkSize)
	for offset := int64(0); offset < fileSize; offset += chunkSize {
		state.SetChunkState(int(offset/chunkSize), types.ChunkCompleted)
	}

	d := &ConcurrentDownloader{ID: "bitmap-complete", State: state, Runtime: &types.RuntimeConfig{}}
	if err := d.handlePause(filepath.Join(tmpDir, "bitmap-complete.bin"), fileSize, NewTaskQueue(), nil); err != nil {
		t.Fatalf("handlePause = %v, want nil", err)
	}
	if snapshot := state.TakePendingResumeState(); snapshot != nil {
		t.Fatalf("unexpected incomplete resume snapshot: %+v", snapshot)
	}
	if got := state.Bytes.VerifiedProgress.Load(); got != fileSize {
		t.Fatalf("VerifiedProgress = %d, want %d", got, fileSize)
	}
}

func TestHandlePause_NilState_RemainingZero_NoPanic(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	d := &ConcurrentDownloader{ID: "nil-state"}
	err := d.handlePause(filepath.Join(tmpDir, "nil_state.bin"), 1000, NewTaskQueue(), nil)
	if err != nil {
		t.Fatalf("expected nil for nil-state completion boundary, got %v", err)
	}
}

func TestSaveStateSnapshot_NilState_RemainingTasksNoPanic(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "nil_state_remain.bin")
	d := &ConcurrentDownloader{ID: "nil-state-remain"}

	queueFalse := NewTaskQueue()
	queueFalse.Push(types.Task{Offset: 0, Length: fileSize})
	if err := d.saveStateSnapshot(destPath, fileSize, queueFalse, nil, false); err != nil {
		t.Fatalf("emit=false nil State: %v", err)
	}

	queueTrue := NewTaskQueue()
	queueTrue.Push(types.Task{Offset: 0, Length: fileSize})
	err := d.saveStateSnapshot(destPath, fileSize, queueTrue, nil, true)
	if !errors.Is(err, types.ErrPaused) {
		t.Fatalf("emit=true nil State: got %v, want ErrPaused", err)
	}
}
