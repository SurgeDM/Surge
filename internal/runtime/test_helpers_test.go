package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/surge-downloader/surge/internal/engine/types"
)

func setupRuntimeTestEnv(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(base, "runtime"))
	t.Setenv("APPDATA", filepath.Join(base, "appdata"))
	return base
}

type stubDownloadService struct {
	streamCh   <-chan interface{}
	cleanup    func()
	streamErr  error
	publishErr error
	published  []interface{}
}

func (s *stubDownloadService) List() ([]types.DownloadStatus, error) { return nil, nil }

func (s *stubDownloadService) History() ([]types.DownloadEntry, error) { return nil, nil }

func (s *stubDownloadService) Add(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
	return "", nil
}

func (s *stubDownloadService) AddWithID(string, string, string, []string, map[string]string, string, int64, bool) (string, error) {
	return "", nil
}

func (s *stubDownloadService) Pause(string) error { return nil }

func (s *stubDownloadService) Resume(string) error { return nil }

func (s *stubDownloadService) ResumeBatch([]string) []error { return nil }

func (s *stubDownloadService) UpdateURL(string, string) error { return nil }

func (s *stubDownloadService) Delete(string) error { return nil }

func (s *stubDownloadService) StreamEvents(context.Context) (<-chan interface{}, func(), error) {
	streamCh := s.streamCh
	if streamCh == nil {
		ch := make(chan interface{})
		close(ch)
		streamCh = ch
	}

	cleanup := s.cleanup
	if cleanup == nil {
		cleanup = func() {}
	}
	return streamCh, cleanup, s.streamErr
}

func (s *stubDownloadService) Publish(msg interface{}) error {
	s.published = append(s.published, msg)
	return s.publishErr
}

func (s *stubDownloadService) GetStatus(string) (*types.DownloadStatus, error) { return nil, nil }

func (s *stubDownloadService) Shutdown() error { return nil }
