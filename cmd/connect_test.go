package cmd

import (
	"context"
	"testing"

	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/tui"
)

type fakeRemoteDownloadService struct {
	addCalls     int
	lastURL      string
	lastPath     string
	lastFile     string
	lastExplicit bool
}

var _ core.DownloadService = (*fakeRemoteDownloadService)(nil)

func (f *fakeRemoteDownloadService) List() ([]types.DownloadStatus, error) {
	return nil, nil
}

func (f *fakeRemoteDownloadService) History() ([]types.DownloadEntry, error) {
	return nil, nil
}

func (f *fakeRemoteDownloadService) Add(url, path, filename string, mirrors []string, headers map[string]string, isExplicitCategory bool, totalSize int64, supportsRange bool) (string, error) {
	f.addCalls++
	f.lastURL = url
	f.lastPath = path
	f.lastFile = filename
	f.lastExplicit = isExplicitCategory
	return "remote-add-id", nil
}

func (f *fakeRemoteDownloadService) AddWithID(url, path, filename string, mirrors []string, headers map[string]string, id string, totalSize int64, supportsRange bool) (string, error) {
	return id, nil
}

func (f *fakeRemoteDownloadService) Pause(id string) error { return nil }

func (f *fakeRemoteDownloadService) Resume(id string) error { return nil }

func (f *fakeRemoteDownloadService) ResumeBatch(ids []string) []error { return nil }

func (f *fakeRemoteDownloadService) UpdateURL(id string, newURL string) error { return nil }

func (f *fakeRemoteDownloadService) Delete(id string) error { return nil }

func (f *fakeRemoteDownloadService) StreamEvents(ctx context.Context) (<-chan interface{}, func(), error) {
	ch := make(chan interface{})
	return ch, func() { close(ch) }, nil
}

func (f *fakeRemoteDownloadService) Publish(msg interface{}) error { return nil }

func (f *fakeRemoteDownloadService) GetStatus(id string) (*types.DownloadStatus, error) {
	return nil, nil
}

func (f *fakeRemoteDownloadService) Shutdown() error { return nil }

func TestNewRemoteRootModel_UsesNilOrchestrator(t *testing.T) {
	m := newRemoteRootModel(1700, nil, "example.com")

	if m.Orchestrator != nil {
		t.Fatal("expected remote root model to use nil orchestrator")
	}
	if !m.IsRemote {
		t.Fatal("expected remote root model to be marked remote")
	}
	if m.ServerHost != "example.com" {
		t.Fatalf("server host = %q, want example.com", m.ServerHost)
	}
}

func TestNewRemoteRootModel_DownloadRequestUsesServiceAdd(t *testing.T) {
	service := &fakeRemoteDownloadService{}
	m := newRemoteRootModel(1700, service, "example.com")
	m.Settings.General.ExtensionPrompt = false
	m.Settings.General.WarnOnDuplicate = false

	updated, cmd := m.Update(events.DownloadRequestMsg{
		URL:      "https://example.com/file.bin",
		Filename: "file.bin",
		Path:     ".",
	})
	if cmd != nil {
		t.Fatal("expected remote add path to complete synchronously without orchestration cmd")
	}

	root, ok := updated.(tui.RootModel)
	if !ok {
		t.Fatalf("unexpected updated model type %T", updated)
	}
	if service.addCalls != 1 {
		t.Fatalf("expected service.Add to be called once, got %d", service.addCalls)
	}
	if service.lastURL != "https://example.com/file.bin" {
		t.Fatalf("service URL = %q, want request URL", service.lastURL)
	}
	if service.lastFile != "file.bin" {
		t.Fatalf("service filename = %q, want file.bin", service.lastFile)
	}
	selected := root.GetSelectedDownload()
	if selected == nil {
		t.Fatal("expected queued remote download to be selected")
	}
	if selected.ID != "remote-add-id" {
		t.Fatalf("queued download ID = %q, want remote-add-id", selected.ID)
	}
}
