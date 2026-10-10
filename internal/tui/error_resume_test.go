package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/types"
)

type errorResumeService struct {
	mockService
	resumeErr error
	resumedID string
	pausedID  string
}

func (s *errorResumeService) Resume(id string) error { s.resumedID = id; return s.resumeErr }
func (s *errorResumeService) Pause(id string) error  { s.pausedID = id; return nil }

func TestPauseShortcutResumesErroredDownloads(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		done, paused, failed  bool
		resumeErr             error
		wantResume, wantPause bool
	}{
		{name: "errored", done: true, failed: true, wantResume: true},
		{name: "errored resume fails", done: true, failed: true, resumeErr: types.ErrNotFound, wantResume: true},
		{name: "paused", paused: true, wantResume: true},
		{name: "paused resume fails", paused: true, resumeErr: types.ErrNotFound, wantResume: true},
		{name: "completed", done: true},
		{name: "active", wantPause: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDownloadModel("download", "https://example.com/file", "file.bin", 100)
			d.Downloaded = 40
			d.done, d.paused = tc.done, tc.paused
			if tc.failed {
				d.err = errors.New("network unavailable")
			}
			originalErr := d.err
			svc := &errorResumeService{resumeErr: tc.resumeErr}
			m := RootModel{state: DashboardState, downloads: []*DownloadModel{d}, Service: svc,
				keys: config.DefaultKeyMap(), list: NewDownloadList(80, 20)}
			if d.done {
				m.activeTab = TabDone
			}
			m.UpdateListItems()
			m.list.Select(0)
			_, cmd := m.updateDashboard(tea.KeyPressMsg{Code: 'p', Text: "p"})
			if (svc.resumedID == d.ID) != tc.wantResume || (svc.pausedID == d.ID) != tc.wantPause {
				t.Fatalf("resume=%q pause=%q", svc.resumedID, svc.pausedID)
			}
			if tc.wantResume && tc.resumeErr == nil {
				if d.done || d.err != nil || d.paused || !d.resuming || cmd == nil {
					t.Fatal("successful resume did not clear terminal state and start spinner")
				}
			} else if tc.resumeErr != nil {
				if d.done != tc.done || d.paused != tc.paused || d.err != originalErr || d.resuming {
					t.Fatal("failed resume changed download state")
				}
			}
			if d.Downloaded != 40 {
				t.Fatal("resume discarded displayed progress")
			}
		})
	}
}

func TestResumeEventsClearErrorAndBindCurrentProgress(t *testing.T) {
	for _, event := range []tea.Msg{
		resumeResultMsg{id: "download"},
		types.DownloadEvent{Type: types.EventResumed, DownloadID: "download"},
		types.DownloadEvent{Type: types.EventStarted, DownloadID: "download", Filename: "file.bin", Total: 100,
			State: &types.DownloadRecord{ProgressState: progress.New("download", 100)}},
	} {
		d := NewDownloadModel("download", "", "file.bin", 100)
		d.done = true
		d.err = errors.New("connection failed")
		d.state = progress.New("old-state", 100)
		m := RootModel{downloads: []*DownloadModel{d}, list: NewDownloadList(80, 20)}
		_, _ = m.updateEvents(event)
		if d.done || d.err != nil {
			t.Fatalf("%T did not clear stale error", event)
		}
		if ev, ok := event.(types.DownloadEvent); ok && ev.Type == types.EventStarted && d.state != stateProgress(ev.State) {
			t.Fatal("new transfer retained the previous progress state")
		}
		_, _ = m.updateEvents(types.DownloadEvent{Type: types.EventProgress, DownloadID: d.ID, Downloaded: 60, Total: 100})
		if d.Downloaded != 60 {
			t.Fatal("resumed download ignored subsequent progress")
		}
	}
}
