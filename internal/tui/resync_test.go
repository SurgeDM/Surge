package tui

import (
	"testing"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestResyncUpdatesTerminalStatesAndRemovesDeletedRows(t *testing.T) {
	model := benchModel(2)
	model.SelectedDownloadID = model.downloads[0].ID
	id := model.SelectedDownloadID
	updated, cmd := model.updateEvents(resyncDownloadsMsg{statuses: []types.DownloadStatus{{ID: id, Filename: "completed.bin", Status: "completed", TotalSize: 100, Downloaded: 100}}})
	got := updated.(RootModel)
	if cmd != nil {
		t.Fatal("resync should not start transfers")
	}
	if len(got.downloads) != 1 || got.downloads[0].ID != id || !got.downloads[0].done || got.downloads[0].Downloaded != 100 {
		t.Fatalf("resync did not reconcile: %+v", got.downloads)
	}
}

func TestResyncDoesNotApplySnapshotOlderThanLifecycleTransition(t *testing.T) {
	model := benchModel(1)
	model.lifecycleRevision = 2
	model.resyncing = true
	updated, cmd := model.updateEvents(resyncDownloadsMsg{revision: 1})
	got := updated.(RootModel)
	if cmd == nil || len(got.downloads) != 1 || !got.resyncing {
		t.Fatal("stale snapshot applied instead of requesting a fresh one")
	}
}
