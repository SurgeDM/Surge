package tui

import (
	"testing"

	"charm.land/bubbles/v2/viewport"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

func TestAutoFollow_BrandNewDownload(t *testing.T) {
	m := RootModel{
		activeTab:   TabDone,
		pinnedTab:   -1,
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)),
	}

	msg := events.DownloadStartedMsg{
		DownloadID: "new-1",
		Filename:   "new-file",
		Total:      100,
		State:      types.NewProgressState("new-1", 100),
	}

	updated, _ := m.Update(msg)
	m2 := updated.(RootModel)

	if m2.activeTab != TabActive {
		t.Errorf("Expected activeTab to be TabActive (1), got %d", m2.activeTab)
	}
	if m2.SelectedDownloadID != "" {
		t.Errorf("Expected SelectedDownloadID to be cleared, got %q", m2.SelectedDownloadID)
	}
}

func TestAutoFollow_ExistingDownloadRestart(t *testing.T) {
	dm := NewDownloadModel("existing-1", "http://example.com", "file", 100)
	dm.paused = true

	m := RootModel{
		activeTab:   TabQueued,
		pinnedTab:   -1,
		downloads:   []*DownloadModel{dm},
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)),
	}

	msg := events.DownloadStartedMsg{
		DownloadID: "existing-1",
		Filename:   "file",
		Total:      100,
		State:      types.NewProgressState("existing-1", 100),
	}

	updated, _ := m.Update(msg)
	m2 := updated.(RootModel)

	if m2.activeTab != TabActive {
		t.Errorf("Expected activeTab to be TabActive (1), got %d", m2.activeTab)
	}
}

func TestAutoFollow_SuppressedByPin(t *testing.T) {
	m := RootModel{
		activeTab:   TabDone,
		pinnedTab:   TabDone,
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)),
	}

	msg := events.DownloadStartedMsg{
		DownloadID: "new-1",
		Filename:   "new-file",
		Total:      100,
		State:      types.NewProgressState("new-1", 100),
	}

	updated, _ := m.Update(msg)
	m2 := updated.(RootModel)

	if m2.activeTab != TabDone {
		t.Errorf("Expected activeTab to remain TabDone (2) because it is pinned, got %d", m2.activeTab)
	}
}

func TestAutoFollow_QueuedToActiveTransition(t *testing.T) {
	// Test that transitioning from Queued to Active (via DownloadStartedMsg)
	// also triggers auto-follow if we are currently on Queued tab.
	dm := NewDownloadModel("id-1", "http://example.com", "file", 100)
	// Initially it's queued (done=false, paused=false, speed=0, connections=0)

	m := RootModel{
		activeTab:   TabQueued,
		pinnedTab:   -1,
		downloads:   []*DownloadModel{dm},
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)),
	}

	// Update list to reflect initial state
	m.UpdateListItems()

	msg := events.DownloadStartedMsg{
		DownloadID: "id-1",
		Filename:   "file",
		Total:      100,
		State:      types.NewProgressState("id-1", 100),
	}

	updated, _ := m.Update(msg)
	m2 := updated.(RootModel)

	if m2.activeTab != TabActive {
		t.Errorf("Expected auto-follow to Active tab, got %d", m2.activeTab)
	}
}
