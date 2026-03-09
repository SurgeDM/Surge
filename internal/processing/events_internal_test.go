package processing

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/testutil"
)

func TestFinalizeCompletedFile_CopiesAcrossDevicesOnEXDEV(t *testing.T) {
	tempDir := t.TempDir()
	finalPath := filepath.Join(tempDir, "video.mp4")
	surgePath := finalPath + types.IncompleteSuffix
	if err := os.WriteFile(surgePath, []byte("partial"), 0o644); err != nil {
		t.Fatalf("failed to create working file: %v", err)
	}

	origRename := renameCompletedFile
	origCopy := copyCompletedFile
	t.Cleanup(func() {
		renameCompletedFile = origRename
		copyCompletedFile = origCopy
	})

	var copied bool
	renameCompletedFile = func(string, string) error {
		return &os.LinkError{Op: "rename", Old: surgePath, New: finalPath, Err: syscall.EXDEV}
	}
	copyCompletedFile = func(src, dst string) error {
		copied = true
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	}

	if err := finalizeCompletedFile(finalPath); err != nil {
		t.Fatalf("finalizeCompletedFile failed: %v", err)
	}
	if !copied {
		t.Fatal("expected copy fallback to run on EXDEV")
	}

	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("failed to read finalized file: %v", err)
	}
	if string(data) != "partial" {
		t.Fatalf("final data = %q, want partial", string(data))
	}
	if _, err := os.Stat(surgePath); !os.IsNotExist(err) {
		t.Fatalf("expected working file removal after copy fallback, stat err: %v", err)
	}
}

func TestStartEventWorker_MarksCompletionAsErrorWhenFinalizationFails(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)

	finalPath := filepath.Join(tempDir, "video.mp4")
	surgePath := finalPath + types.IncompleteSuffix
	if err := os.WriteFile(surgePath, []byte("partial"), 0o644); err != nil {
		t.Fatalf("failed to create working file: %v", err)
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

	origRename := renameCompletedFile
	t.Cleanup(func() {
		renameCompletedFile = origRename
	})
	renameCompletedFile = func(string, string) error {
		return errors.New("disk full")
	}

	mgr := NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 1)
	ch <- events.DownloadCompleteMsg{
		DownloadID: "download-1",
		Filename:   "video.mp4",
		Elapsed:    2 * time.Second,
		Total:      7,
	}
	close(ch)

	mgr.StartEventWorker(ch)

	entry, err := state.GetDownload("download-1")
	if err != nil {
		t.Fatalf("failed to reload entry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected download entry to remain")
	}
	if entry.Status != "error" {
		t.Fatalf("status = %q, want error", entry.Status)
	}
	if _, err := os.Stat(surgePath); err != nil {
		t.Fatalf("expected working file to remain for retry, stat err: %v", err)
	}
	if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
		t.Fatalf("expected no finalized file after failure, stat err: %v", err)
	}
}

func TestStartEventWorker_RemovesIncompleteFileOnErrorWithoutDBEntry(t *testing.T) {
	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "video.mp4")
	surgePath := destPath + types.IncompleteSuffix
	if err := os.WriteFile(surgePath, []byte("partial"), 0o644); err != nil {
		t.Fatalf("failed to create working file: %v", err)
	}

	mgr := NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 1)
	ch <- events.DownloadErrorMsg{
		DownloadID: "download-no-db",
		Filename:   "video.mp4",
		DestPath:   destPath,
		Err:        errors.New("boom"),
	}
	close(ch)

	mgr.StartEventWorker(ch)

	if _, err := os.Stat(surgePath); !os.IsNotExist(err) {
		t.Fatalf("expected working file to be removed even without DB entry, stat err: %v", err)
	}
}
