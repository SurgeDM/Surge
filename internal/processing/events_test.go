package processing_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
	"github.com/surge-downloader/surge/internal/testutil"
)

func TestStartEventWorker_FinalizesCompletedFileUsingDestPath(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)

	finalPath := filepath.Join(tempDir, "video.mp4")
	surgePath := finalPath + types.IncompleteSuffix
	if err := os.WriteFile(surgePath, []byte("partial"), 0o644); err != nil {
		t.Fatalf("failed to create incomplete file: %v", err)
	}

	if err := state.AddToMasterList(types.DownloadEntry{
		ID:       "download-1",
		URL:      "https://example.com/video.mp4",
		URLHash:  state.URLHash("https://example.com/video.mp4"),
		DestPath: finalPath,
		Filename: "video.mp4",
		Status:   "downloading",
	}); err != nil {
		t.Fatalf("failed to seed download entry: %v", err)
	}
	if err := state.SaveStateWithOptions("https://example.com/video.mp4", finalPath, &types.DownloadState{
		ID:        "download-1",
		URL:       "https://example.com/video.mp4",
		Filename:  "video.mp4",
		DestPath:  finalPath,
		TotalSize: 7,
		Tasks: []types.Task{
			{Offset: 0, Length: 7},
		},
	}, state.SaveStateOptions{SkipFileHash: true}); err != nil {
		t.Fatalf("failed to seed download state: %v", err)
	}

	mgr := processing.NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 1)
	ch <- events.DownloadCompleteMsg{
		DownloadID: "download-1",
		Filename:   "video.mp4",
		Elapsed:    2 * time.Second,
		Total:      7,
	}
	close(ch)

	mgr.StartEventWorker(ch)

	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("expected finalized file at %s: %v", finalPath, err)
	}
	if _, err := os.Stat(surgePath); !os.IsNotExist(err) {
		t.Fatalf("expected incomplete file to be removed, stat err: %v", err)
	}

	entry, err := state.GetDownload("download-1")
	if err != nil {
		t.Fatalf("failed to reload completed entry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected completed entry to exist")
	}
	if entry.Status != "completed" {
		t.Fatalf("status = %q, want completed", entry.Status)
	}
	if entry.DestPath != finalPath {
		t.Fatalf("dest_path = %q, want %q", entry.DestPath, finalPath)
	}

	db, err := state.GetDB()
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	var taskCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks WHERE download_id = ?", "download-1").Scan(&taskCount); err != nil {
		t.Fatalf("failed to count tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task_count = %d, want 0", taskCount)
	}
}

func TestStartEventWorker_PersistsQueuedMirrorsForResume(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)
	finalPath := filepath.Join(tempDir, "video.mp4")

	mgr := processing.NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 1)
	ch <- events.DownloadQueuedMsg{
		DownloadID: "download-queued",
		URL:        "https://example.com/video.mp4",
		Filename:   "video.mp4",
		DestPath:   finalPath,
		Mirrors:    []string{"https://mirror-1.example/video.mp4", "https://mirror-2.example/video.mp4"},
	}
	close(ch)

	mgr.StartEventWorker(ch)

	queuedState, err := state.LoadState("https://example.com/video.mp4", finalPath)
	if err != nil {
		t.Fatalf("failed to reload queued state: %v", err)
	}
	if queuedState == nil {
		t.Fatal("expected queued state entry to exist")
	}
	if len(queuedState.Mirrors) != 2 {
		t.Fatalf("mirrors = %v, want 2 queued mirrors", queuedState.Mirrors)
	}
	if queuedState.Mirrors[0] != "https://mirror-1.example/video.mp4" || queuedState.Mirrors[1] != "https://mirror-2.example/video.mp4" {
		t.Fatalf("mirrors = %v, want queued mirrors to round-trip", queuedState.Mirrors)
	}
}

func TestStartEventWorker_PreservesQueuedMirrorsAcrossStartedThenError(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)
	finalPath := filepath.Join(tempDir, "video.mp4")

	mgr := processing.NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 3)
	ch <- events.DownloadQueuedMsg{
		DownloadID: "download-queued",
		URL:        "https://example.com/video.mp4",
		Filename:   "video.mp4",
		DestPath:   finalPath,
		Mirrors:    []string{"https://mirror-1.example/video.mp4", "https://mirror-2.example/video.mp4"},
	}
	ch <- events.DownloadStartedMsg{
		DownloadID: "download-queued",
		URL:        "https://example.com/video.mp4",
		Filename:   "video.mp4",
		Total:      1024,
		DestPath:   finalPath,
	}
	ch <- events.DownloadErrorMsg{
		DownloadID: "download-queued",
		Filename:   "video.mp4",
		DestPath:   finalPath,
		Err:        os.ErrDeadlineExceeded,
	}
	close(ch)

	mgr.StartEventWorker(ch)

	entry, err := state.GetDownload("download-queued")
	if err != nil {
		t.Fatalf("failed to reload errored entry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected errored entry to exist")
	}
	if entry.Status != "error" {
		t.Fatalf("status = %q, want error", entry.Status)
	}
	if len(entry.Mirrors) != 2 {
		t.Fatalf("mirrors = %v, want queued mirrors to survive started/error", entry.Mirrors)
	}
	if entry.Mirrors[0] != "https://mirror-1.example/video.mp4" || entry.Mirrors[1] != "https://mirror-2.example/video.mp4" {
		t.Fatalf("mirrors = %v, want queued mirrors to round-trip", entry.Mirrors)
	}
}
