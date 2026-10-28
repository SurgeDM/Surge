package tui

import (
	"fmt"
	"github.com/SurgeDM/Surge/internal/types"
	"strings"
	"testing"
)

func TestMissingFileStatusAndRecovery(t *testing.T) {
	InitializeTUI()
	d := NewDownloadModel("id", "url", "file.bin", 100)
	d.done = true
	for _, missing := range []bool{true, false} {
		d.fileMissing = missing
		text := stripANSI.ReplaceAllString(getDownloadStatus(d, ""), "")
		want := "Completed"
		if missing {
			want = "File missing"
		}
		if !strings.Contains(text, want) {
			t.Fatalf("status=%q, want %q", text, want)
		}
		if !d.done || d.err != nil {
			t.Fatal("missing flag changed completion state")
		}
	}
}

func TestFailedMoveUpdatesMissingStatusAndActivityLog(t *testing.T) {
	m := confirmationSettingsModel(t, "General", "clipboard_monitor")
	d := NewDownloadModel("id", "url", "file.bin", 100)
	d.done = true
	d.Destination = "/old/file.bin"
	category := "Videos"
	d.categoryOverride = &category
	m.downloads = []*DownloadModel{d}
	updated, _ := m.Update(categoryAssignedMsg{id: d.ID, category: "Images", err: fmt.Errorf("no such file"), status: &types.DownloadStatus{FileMissing: true}})
	m = updated.(RootModel)
	if !d.fileMissing || *d.categoryOverride != "Videos" || d.Destination != "/old/file.bin" {
		t.Fatal("failed move changed category/path or did not mark missing")
	}
	if !strings.Contains(strings.Join(m.logEntries, "\n"), "Move failed: file missing") {
		t.Fatal("activity log did not clearly report missing file")
	}
}
