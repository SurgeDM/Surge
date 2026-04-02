package processing

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/testutil"
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
	origNotify := notify
	t.Cleanup(func() {
		renameCompletedFile = origRename
		notify = origNotify
	})
	renameCompletedFile = func(string, string) error {
		return errors.New("disk full")
	}
	var calls []struct {
		title string
		msg   string
	}
	notify = func(title, msg string) {
		calls = append(calls, struct {
			title string
			msg   string
		}{title: title, msg: msg})
	}

	settingsDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", settingsDir)
	settings := config.DefaultSettings()
	settings.General.DownloadCompleteNotification = true
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
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
	if len(calls) != 1 {
		t.Fatalf("notification calls = %d, want 1", len(calls))
	}
	if calls[0].title != "Download failed: video.mp4" {
		t.Fatalf("notification title = %q, want %q", calls[0].title, "Download failed: video.mp4")
	}
	if calls[0].msg != "disk full" {
		t.Fatalf("notification msg = %q, want %q", calls[0].msg, "disk full")
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

func TestStartEventWorker_SuppressesNotificationWhenSettingDisabled(t *testing.T) {
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

	settingsDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", settingsDir)
	settings := config.DefaultSettings()
	settings.General.DownloadCompleteNotification = false
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	origNotify := notify
	t.Cleanup(func() { notify = origNotify })
	var calls int
	notify = func(string, string) { calls++ }

	mgr := NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 1)
	ch <- events.DownloadCompleteMsg{
		DownloadID: "download-1",
		Filename:   "video.mp4",
		Elapsed:    1 * time.Second,
		Total:      7,
	}
	close(ch)

	mgr.StartEventWorker(ch)

	if calls != 0 {
		t.Fatalf("notification calls = %d, want 0", calls)
	}
}

func TestStartEventWorker_CompletionNotificationUsesGenericMessageWhenElapsedZero(t *testing.T) {
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

	settingsDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", settingsDir)
	settings := config.DefaultSettings()
	settings.General.DownloadCompleteNotification = true
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	origNotify := notify
	t.Cleanup(func() { notify = origNotify })
	var calls []struct {
		title string
		msg   string
	}
	notify = func(title, msg string) {
		calls = append(calls, struct {
			title string
			msg   string
		}{title: title, msg: msg})
	}

	mgr := NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 1)
	ch <- events.DownloadCompleteMsg{
		DownloadID: "download-1",
		Filename:   "video.mp4",
		Elapsed:    0,
		Total:      7,
	}
	close(ch)

	mgr.StartEventWorker(ch)

	if len(calls) != 1 {
		t.Fatalf("notification calls = %d, want 1", len(calls))
	}
	if calls[0].title != "Download Complete: video.mp4" {
		t.Fatalf("notification title = %q, want %q", calls[0].title, "Download Complete: video.mp4")
	}
	if calls[0].msg != "Download complete!" {
		t.Fatalf("notification msg = %q, want %q", calls[0].msg, "Download complete!")
	}
}

func TestStartEventWorker_ErrorNotificationFallsBackToDownloadID(t *testing.T) {
	settingsDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", settingsDir)
	settings := config.DefaultSettings()
	settings.General.DownloadCompleteNotification = true
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	origNotify := notify
	t.Cleanup(func() { notify = origNotify })
	var calls []struct {
		title string
		msg   string
	}
	notify = func(title, msg string) {
		calls = append(calls, struct {
			title string
			msg   string
		}{title: title, msg: msg})
	}

	mgr := NewLifecycleManager(nil, nil)
	ch := make(chan interface{}, 1)
	ch <- events.DownloadErrorMsg{
		DownloadID: "download-42",
		Filename:   "",
		DestPath:   "",
		Err:        errors.New("boom"),
	}
	close(ch)

	mgr.StartEventWorker(ch)

	if len(calls) != 1 {
		t.Fatalf("notification calls = %d, want 1", len(calls))
	}
	if calls[0].title != "Download failed: download-42" {
		t.Fatalf("notification title = %q, want %q", calls[0].title, "Download failed: download-42")
	}
	if calls[0].msg != "boom" {
		t.Fatalf("notification msg = %q, want %q", calls[0].msg, "boom")
	}
}
