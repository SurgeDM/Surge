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
	m := newRemoteRootModel("https://example.com:1700", nil)

	if m.Orchestrator != nil {
		t.Fatal("expected remote root model to use nil orchestrator")
	}
	if !m.IsRemote {
		t.Fatal("expected remote root model to be marked remote")
	}
	if m.ServerHost != "example.com" {
		t.Fatalf("server host = %q, want example.com", m.ServerHost)
	}
	if m.ServerPort != 1700 {
		t.Fatalf("server port = %d, want 1700", m.ServerPort)
	}
}

func TestNewRemoteRootModel_DownloadRequestUsesServiceAdd(t *testing.T) {
	service := &fakeRemoteDownloadService{}
	m := newRemoteRootModel("https://example.com:1700", service)
	m.Settings.Extension.ExtensionPrompt = false
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

func TestParseConnectTarget_ParsesIPv6AddressWithPort(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   connectTarget
	}{
		{
			name:   "bracketed IPv6 address with port",
			target: "[2001:db8::1]:1700",
			want: connectTarget{
				BaseURL: "https://[2001:db8::1]:1700",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConnectTarget(tt.target, false)
			if err != nil {
				t.Fatalf("parseConnectTarget(%q) returned error: %v", tt.target, err)
			}
			if got != tt.want {
				t.Fatalf("parseConnectTarget(%q) = %#v, want %#v", tt.target, got, tt.want)
			}
		})
	}
}

func TestParseRemoteServerAddress_DefaultPorts(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		wantHost string
		wantPort int
	}{
		{name: "https default port", baseURL: "https://example.com", wantHost: "example.com", wantPort: 443},
		{name: "http default port", baseURL: "http://127.0.0.1", wantHost: "127.0.0.1", wantPort: 80},
		{name: "explicit port", baseURL: "https://example.com:1700", wantHost: "example.com", wantPort: 1700},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort := parseRemoteServerAddress(tt.baseURL)
			if gotHost != tt.wantHost || gotPort != tt.wantPort {
				t.Fatalf("parseRemoteServerAddress(%q) = (%q, %d), want (%q, %d)", tt.baseURL, gotHost, gotPort, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestParseConnectTarget_InvalidTarget(t *testing.T) {
	tests := []string{
		"example.com",
		"2001:db8::1:1700",
		"[2001:db8::1]",
		"example.com:not-a-port",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			if got, err := parseConnectTarget(target, false); err == nil {
				t.Fatalf("parseConnectTarget(%q) = %#v, want error", target, got)
			}
		})
	}
}
