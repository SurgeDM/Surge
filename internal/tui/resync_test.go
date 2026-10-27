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

func TestReconnectInvalidatesRefreshAlreadyInFlight(t *testing.T) {
	model := benchModel(1)
	model.resyncing = true
	updated, _ := model.updateEvents(types.DownloadEvent{Type: types.EventSystem, ResyncRequired: true, Message: "reconnected"})
	got := updated.(RootModel)
	if got.lifecycleRevision != 1 {
		t.Fatal("new resync requirement did not invalidate the in-flight query")
	}
	// Even a failed response from the earlier connection must trigger a fresh
	// query rather than consuming the reconnect notice and leaving stale rows.
	refreshed, cmd := got.updateEvents(resyncDownloadsMsg{revision: 0})
	if cmd == nil || !refreshed.(RootModel).resyncing {
		t.Fatal("reconnect recovery was lost behind an older refresh")
	}
}
