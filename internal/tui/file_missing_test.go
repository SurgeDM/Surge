package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/types"
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

func TestMovingStatusBlocksOnlySelectedDownloadChanges(t *testing.T) {
	InitializeTUI()
	m := InitialRootModel(1701, "test", nil, nil, nil, false)
	d := NewDownloadModel("moving", "url", "file.bin", 100)
	d.done = true
	other := NewDownloadModel("other", "url", "other.bin", 100)
	m.downloads = []*DownloadModel{d, other}
	m.activeTab = TabAll
	m.SelectedDownloadID = d.ID
	m.UpdateListItems()
	moving := true
	updated, _ := m.Update(types.DownloadEvent{Type: types.EventSystem, DownloadID: d.ID, Moving: &moving})
	m = updated.(RootModel)
	if !strings.Contains(stripANSI.ReplaceAllString(getDownloadStatus(d, ""), ""), "Moving") {
		t.Fatal("moving status not rendered")
	}
	for _, binding := range []key.Binding{m.keys.Dashboard.Delete, m.keys.Dashboard.PurgeFile, m.keys.Dashboard.AssignCategory} {
		keys := binding.Keys()
		if len(keys[0]) != 1 {
			t.Fatal("test expects a single-rune binding")
		}
		ch := []rune(keys[0])[0]
		updated, cmd := m.updateDashboard(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = updated.(RootModel)
		if cmd != nil || m.state != DashboardState {
			t.Fatal("moving download accepted mutation")
		}
	}
	m.SelectedDownloadID = other.ID
	m.list.Select(1)
	updated, _ = m.updateDashboard(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(RootModel)
	if m.state != RemoveConfirmState {
		t.Fatal("other download was locked")
	}
	moving = false
	updated, _ = m.Update(types.DownloadEvent{Type: types.EventSystem, DownloadID: d.ID, Moving: &moving})
	m = updated.(RootModel)
	if d.moving || !strings.Contains(stripANSI.ReplaceAllString(getDownloadStatus(d, ""), ""), "Completed") {
		t.Fatal("moving status did not clear")
	}
}

func TestCategoryAssignmentPreservesDestinationOnEmptyStatusPath(t *testing.T) {
	m := confirmationSettingsModel(t, "General", "clipboard_monitor")
	d := NewDownloadModel("id", "url", "file.bin", 100)
	d.Destination = "/downloads/file.bin"
	d.moving = true
	m.downloads = []*DownloadModel{d}
	updated, _ := m.Update(categoryAssignedMsg{id: d.ID, category: "Videos", status: &types.DownloadStatus{FileMissing: true}})
	m = updated.(RootModel)
	if d.Destination != "/downloads/file.bin" || !d.fileMissing || d.moving || d.categoryOverride == nil || *d.categoryOverride != "Videos" {
		t.Fatalf("incorrect assignment result: %+v", d)
	}
	_, _ = m.Update(categoryAssignedMsg{id: d.ID, category: "Videos", status: &types.DownloadStatus{DestPath: "/videos/file.bin"}})
	if d.Destination != "/videos/file.bin" || d.fileMissing {
		t.Fatal("nonempty destination was not applied")
	}
}
