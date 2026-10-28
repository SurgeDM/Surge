package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/types"
)

func requestIDTestModel() RootModel {
	settings := config.DefaultSettings()
	settings.Extension.ExtensionPrompt.Value = true
	return RootModel{Settings: settings, Service: &updateMockService{}, keys: config.DefaultKeyMap(), inputs: newInputModels(), list: NewDownloadList(80, 20)}
}

func TestExtensionConfirmationPreservesRequestedID(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		name := "extension"
		if duplicate {
			name = "duplicate warning"
		}
		t.Run(name, func(t *testing.T) {
			m := requestIDTestModel()
			url := "https://example.com/file.zip"
			if duplicate {
				m.downloads = append(m.downloads, &DownloadModel{ID: "existing", URL: url, Filename: "file.zip"})
			}
			msg := types.DownloadEvent{Type: types.EventRequest, DownloadID: " requested-id ", URL: url, Filename: "file.zip", Path: t.TempDir()}
			updated, _ := m.Update(msg)
			m = updated.(RootModel)
			if m.pendingDownloadID != "requested-id" {
				t.Fatalf("pending ID = %q", m.pendingDownloadID)
			}
			if duplicate {
				if m.state != DuplicateWarningState {
					t.Fatalf("wrong confirmation state: %v", m.state)
				}
				updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			} else {
				if m.state != ExtensionConfirmationState {
					t.Fatalf("wrong confirmation state: %v", m.state)
				}
				updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
			m = updated.(RootModel)
			if d := m.FindDownloadByID("requested-id"); d == nil || d.URL != url {
				t.Fatalf("confirmed request lost its ID: %+v", m.downloads)
			}
			if m.pendingDownloadID != "" {
				t.Fatal("confirmed ID leaked into later requests")
			}
		})
	}
}

func TestRequestedIDDuplicatesRejectedBeforeConfirmation(t *testing.T) {
	m := requestIDTestModel()
	m.downloads = []*DownloadModel{{ID: "existing", URL: "https://example.com/old"}}
	msg := types.DownloadEvent{Type: types.EventRequest, DownloadID: "existing", URL: "https://example.com/new", Path: t.TempDir()}
	updated, _ := m.Update(msg)
	m = updated.(RootModel)
	if m.state != DashboardState || !strings.Contains(strings.Join(m.logEntries, "\n"), "Download ID already exists") {
		t.Fatal("existing ID opened a confirmation")
	}
	msg.DownloadID = "pending"
	updated, _ = m.Update(msg)
	m = updated.(RootModel)
	updated, _ = m.Update(msg)
	m = updated.(RootModel)
	if len(m.pendingRequestQueue) != 0 || m.pendingDownloadID != "pending" {
		t.Fatal("duplicate pending ID was queued")
	}
	msg.DownloadID = "queued"
	updated, _ = m.Update(msg)
	m = updated.(RootModel)
	updated, _ = m.Update(msg)
	m = updated.(RootModel)
	if len(m.pendingRequestQueue) != 1 {
		t.Fatal("duplicate queued ID was accepted")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(RootModel)
	if m.pendingDownloadID != "queued" || m.state != ExtensionConfirmationState {
		t.Fatal("queued request lost its ID")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(RootModel)
	if m.pendingDownloadID != "" {
		t.Fatal("cancelled ID leaked into later requests")
	}
	msg.DownloadID = ""
	updated, _ = m.Update(msg)
	m = updated.(RootModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(RootModel)
	if m.FindDownloadByID("mock-id") == nil {
		t.Fatal("ID-less request reused a cancelled request ID")
	}
}
