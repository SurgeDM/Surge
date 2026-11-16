package orchestrator

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
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
