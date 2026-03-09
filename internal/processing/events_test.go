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
)

func setupProcessingTestDB(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	state.CloseDB()
	state.Configure(filepath.Join(tempDir, "surge.db"))
	if _, err := state.GetDB(); err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	t.Cleanup(state.CloseDB)
	return tempDir
}

func TestStartEventWorker_FinalizesCompletedFileUsingDestPath(t *testing.T) {
	tempDir := setupProcessingTestDB(t)

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
}
