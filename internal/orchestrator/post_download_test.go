package orchestrator

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
	dir := t.TempDir()
	path := filepath.Join(dir, "show with spaces.zip")
	if err := runPostDownload(`printf '%s\n%s\n%s' "$PWD" "$d" "$f" > result`, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "result"))
	if err != nil {
		t.Fatal(err)
	}
	want := dir + "\n" + dir + "\nshow with spaces.zip"
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
	manager := NewLifecycleManager(nil, nil, settings)
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
}
