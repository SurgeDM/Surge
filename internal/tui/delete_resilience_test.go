package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

type mockService struct {
	deleteErr error
	deletedID string
}

func (m *mockService) Delete(id string) error {
	m.deletedID = id
	return m.deleteErr
}

func (m *mockService) List() ([]types.DownloadStatus, error)   { return nil, nil }
func (m *mockService) History() ([]types.DownloadEntry, error) { return nil, nil }
func (m *mockService) Add(url string, path string, filename string, mirrors []string, headers map[string]string, isExplicitCategory bool, totalSize int64, supportsRange bool) (string, error) {
	return "", nil
}
func (m *mockService) AddWithID(url string, path string, filename string, mirrors []string, headers map[string]string, id string, totalSize int64, supportsRange bool) (string, error) {
	return "", nil
}
func (m *mockService) ResumeBatch(ids []string) []error { return nil }
func (m *mockService) StreamEvents(ctx context.Context) (<-chan interface{}, func(), error) {
	return nil, nil, nil
}
func (m *mockService) Publish(msg interface{}) error                      { return nil }
func (m *mockService) Pause(id string) error                              { return nil }
func (m *mockService) Resume(id string) error                             { return nil }
func (m *mockService) UpdateURL(id string, newURL string) error           { return nil }
func (m *mockService) GetStatus(id string) (*types.DownloadStatus, error) { return nil, nil }
func (m *mockService) Shutdown() error                                    { return nil }

func TestUpdateDashboard_DeleteResilience(t *testing.T) {
	// This test validates the TUI's defensive layer independently of the service
	// implementation. Even though Service.Delete currently returns nil for missing
	// IDs, the TUI should still gracefully handle ErrNotFound if it occurs.
	dm := &DownloadModel{ID: "ghost-id", Filename: "ghost.zip"}
	svc := &mockService{deleteErr: types.ErrNotFound}

	m := RootModel{
		state:     DashboardState,
		downloads: []*DownloadModel{dm},
		Service:   svc,
		keys:      Keys,
		list:      NewDownloadList(80, 20),
	}
	m.UpdateListItems()
	m.list.Select(0) // Select the ghost download

	// Simulate pressing 'x' (Delete)
	msg := tea.KeyPressMsg{Code: 'x', Text: "x"}
	updated, _ := m.updateDashboard(msg)
	m2 := updated.(RootModel)

	if len(m2.downloads) != 0 {
		t.Errorf("Expected download to be removed even on 'not found' error, but %d entries remain", len(m2.downloads))
	}
	if svc.deletedID != "ghost-id" {
		t.Errorf("Expected Service.Delete to be called with 'ghost-id', got %q", svc.deletedID)
	}
}

func TestUpdateDashboard_DeleteSuccess(t *testing.T) {
	dm := &DownloadModel{ID: "real-id", Filename: "real.zip"}
	svc := &mockService{deleteErr: nil}

	m := RootModel{
		state:     DashboardState,
		downloads: []*DownloadModel{dm},
		Service:   svc,
		keys:      Keys,
		list:      NewDownloadList(80, 20),
	}
	m.UpdateListItems()
	m.list.Select(0)

	msg := tea.KeyPressMsg{Code: 'x', Text: "x"}
	updated, _ := m.updateDashboard(msg)
	m2 := updated.(RootModel)

	if len(m2.downloads) != 0 {
		t.Errorf("Expected download to be removed on success, but %d entries remain", len(m2.downloads))
	}
}

func TestUpdateDashboard_DeleteOtherError(t *testing.T) {
	dm := &DownloadModel{ID: "error-id", Filename: "error.zip"}
	svc := &mockService{deleteErr: errors.New("some other error")}

	m := RootModel{
		state:     DashboardState,
		downloads: []*DownloadModel{dm},
		Service:   svc,
		keys:      Keys,
		list:      NewDownloadList(80, 20),
	}
	m.UpdateListItems()
	m.list.Select(0)

	msg := tea.KeyPressMsg{Code: 'x', Text: "x"}
	updated, _ := m.updateDashboard(msg)
	m2 := updated.(RootModel)

	if len(m2.downloads) != 1 {
		t.Errorf("Expected download to REMAIN on non-not-found error, but %d entries remain", len(m2.downloads))
	}
}
