package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestPostDownloadCategorySelection(t *testing.T) {
	s := config.DefaultSettings()
	s.Categories.CategoryEnabled.Value = true
	s.Categories.Categories = []config.Category{{Name: "Shows", Pattern: `\.zip$`, PostDownloadCommand: "unpack"}, {Name: "Other", PostDownloadCommand: "other"}}
	r := &types.DownloadRecord{Filename: "show.zip"}
	if got := postDownloadCommand(s, r); got != "unpack" {
		t.Fatalf("automatic category: %q", got)
	}
	other := "Other"
	r.CategoryOverride = &other
	if got := postDownloadCommand(s, r); got != "other" {
		t.Fatalf("explicit category: %q", got)
	}
	empty := ""
	r.CategoryOverride = &empty
	if got := postDownloadCommand(s, r); got != "" {
		t.Fatalf("uncategorized: %q", got)
	}
}

func TestPostDownloadCommandWorkingDirectoryAndVariables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell assertion")
	}
	target := t.TempDir()
	dir := filepath.Join(t.TempDir(), "linked-downloads")
	if err := os.Symlink(target, dir); err != nil {
		t.Fatal(err)
	}
	physicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "show with spaces.zip")
	if err := runPostDownload(`printf '%s\n%s\n%s' "$(pwd -P)" "$d" "$f" > result`, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "result"))
	if err != nil {
		t.Fatal(err)
	}
	want := physicalDir + "\n" + dir + "\nshow with spaces.zip"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if err := runPostDownload("exit 7", path); err == nil {
		t.Fatal("expected command error")
	}
}

func TestPostDownloadHookSkipsFailedCategoryMove(t *testing.T) {
	root := setupEventStateDB(t)
	const id = "failed-move-hook"
	source := filepath.Join(root, "file.bin")
	target := t.TempDir()
	for _, path := range []string{source + types.IncompleteSuffix, filepath.Join(target, "file.bin")} {
		if err := os.WriteFile(path, []byte("file contents"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	entry := types.DownloadRecord{ID: id, Status: "queued", DestPath: source, Filename: "file.bin"}
	if err := store.AddToMasterList(entry); err != nil {
		t.Fatal(err)
	}
	if err := store.ScheduleCategoryMove(id, "Files", target); err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings()
	settings.General.DownloadCompleteNotification.Value = false
	settings.Categories.Categories = []config.Category{{Name: "Files", PostDownloadCommand: "exit 0"}}
	if runtime.GOOS != "windows" {
		settings.Categories.Categories[0].PostDownloadCommand = `printf '%s' "$f" >> hook-result`
	}
	bus := NewEventBus()
	defer bus.Shutdown()
	observer, stop := bus.Subscribe()
	defer stop()
	manager := NewLifecycleManager(nil, bus, settings)
	events := make(chan types.DownloadEvent, 1)
	events <- types.DownloadEvent{Type: types.EventComplete, DownloadID: id, Filename: entry.Filename, Total: 13}
	close(events)
	manager.StartEventWorker(events)

	saved, err := store.GetDownload(id)
	if err != nil || saved == nil || saved.Status != "completed" || saved.DestPath != source || saved.CategoryMoveError == "" {
		t.Fatalf("expected completed download with failed move, got %+v, %v", saved, err)
	}
	if state := store.GetHookState(id); state.Running || state.Processed {
		t.Fatalf("hook started after failed move: %+v", state)
	}
	if runtime.GOOS == "windows" {
		return // The retry fixture below uses a POSIX shell.
	}
	if err := os.Remove(filepath.Join(target, "file.bin")); err != nil {
		t.Fatal(err)
	}
	if err := manager.MoveCompletedCategory(id, "Files", target); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
waitForHook:
	for {
		select {
		case event := <-observer:
			if event.Processing != nil && !*event.Processing {
				if event.HookError != "" {
					t.Fatal(event.HookError)
				}
				break waitForHook
			}
		case <-deadline:
			t.Fatal("hook did not finish after the move retry")
		}
	}
	other := t.TempDir()
	if err := manager.MoveCompletedCategory(id, "Files", other); err != nil {
		t.Fatal(err)
	}
	if err := manager.MoveCompletedCategory(id, "Files", target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(target, "hook-result"))
	if err != nil || string(got) != "file.bin" || manager.HasPendingDownloads() {
		t.Fatalf("hook result = %q, error = %v, pending = %t", got, err, manager.HasPendingDownloads())
	}
	if _, err := os.Stat(filepath.Join(other, "hook-result")); !os.IsNotExist(err) {
		t.Fatalf("ordinary category move ran a hook: %v", err)
	}

}

func TestMoveRetryRunsHookWhenOldCopyCannotBeRemoved(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires a POSIX shell and enforced directory permissions")
	}
	setupEventStateDB(t)
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	path := filepath.Join(sourceDir, "file.bin")
	target := filepath.Join(targetDir, filepath.Base(path))
	for _, file := range []string{path, target} {
		if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	entry := types.DownloadRecord{ID: t.Name(), Filename: "file.bin", DestPath: path, Status: "queued"}
	if err := store.AddToMasterList(entry); err != nil {
		t.Fatal(err)
	}
	if err := store.ScheduleCategoryMove(entry.ID, "Files", targetDir); err != nil {
		t.Fatal(err)
	}
	entry.Status = "completed"
	if err := store.AddToMasterList(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sourceDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sourceDir, 0700) })

	settings := config.DefaultSettings()
	settings.Categories.Categories = []config.Category{{Name: "Files", PostDownloadCommand: `printf '%s' "$f" > hook-result`}}
	bus := NewEventBus()
	defer bus.Shutdown()
	observer, stop := bus.Subscribe()
	defer stop()
	manager := NewLifecycleManager(nil, bus, settings)

	err := manager.MoveCompletedCategory(entry.ID, "Files", targetDir)
	var cleanupErr *store.CategoryMoveCleanupError
	if !errors.As(err, &cleanupErr) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected an old-copy cleanup error, got %v", err)
	}
	saved, err := store.GetDownload(entry.ID)
	if err != nil || saved == nil || saved.DestPath != target || saved.PendingCategoryMove != "" {
		t.Fatalf("expected committed category move, got %+v, %v", saved, err)
	}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event := <-observer:
			if event.Processing != nil && !*event.Processing {
				data, err := os.ReadFile(filepath.Join(targetDir, "hook-result"))
				if err != nil || string(data) != "file.bin" || event.HookError != "" {
					t.Fatalf("hook result = %q, read error = %v, hook error = %q", data, err, event.HookError)
				}
				return
			}
		case <-timeout:
			t.Fatal("hook did not finish after the committed move")
		}
	}
}
