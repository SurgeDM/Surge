package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
)

type httpAPITestService struct {
	history []types.DownloadEntry
}

func (s *httpAPITestService) List() ([]types.DownloadStatus, error) {
	return nil, nil
}

func (s *httpAPITestService) History() ([]types.DownloadEntry, error) {
	return s.history, nil
}

func (s *httpAPITestService) Add(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
	return "", errors.New("not implemented")
}

func (s *httpAPITestService) AddWithID(string, string, string, []string, map[string]string, string, int64, bool) (string, error) {
	return "", errors.New("not implemented")
}

func (s *httpAPITestService) Pause(string) error {
	return nil
}

func (s *httpAPITestService) Resume(string) error {
	return nil
}

func (s *httpAPITestService) ResumeBatch([]string) []error {
	return nil
}

func (s *httpAPITestService) UpdateURL(string, string) error {
	return nil
}

func (s *httpAPITestService) Delete(string) error {
	return nil
}

func (s *httpAPITestService) StreamEvents(context.Context) (<-chan interface{}, func(), error) {
	channel := make(chan interface{})
	cleanup := func() { close(channel) }
	return channel, cleanup, nil
}

func (s *httpAPITestService) Publish(interface{}) error {
	return nil
}

func (s *httpAPITestService) GetStatus(string) (*types.DownloadStatus, error) {
	return nil, errors.New("not found")
}

func (s *httpAPITestService) Shutdown() error {
	return nil
}

func TestEnsureOpenActionRequestAllowed_RemoteToggle(t *testing.T) {
	original := globalSettings
	t.Cleanup(func() {
		globalSettings = original
	})

	request := httptest.NewRequest(http.MethodPost, "/open-file?id=example", nil)
	request.RemoteAddr = "203.0.113.8:12345"

	globalSettings = config.DefaultSettings()
	if err := ensureOpenActionRequestAllowed(request); err == nil {
		t.Fatal("expected remote open action to be denied by default")
	}

	globalSettings = config.DefaultSettings()
	globalSettings.General.AllowRemoteOpenActions = true
	if err := ensureOpenActionRequestAllowed(request); err != nil {
		t.Fatalf("expected remote open action to be allowed when enabled, got: %v", err)
	}
}

func TestHistoryEndpoint_SortsMostRecentFirst(t *testing.T) {
	service := &httpAPITestService{
		history: []types.DownloadEntry{
			{ID: "old", CompletedAt: 10},
			{ID: "new", CompletedAt: 30},
			{ID: "middle", CompletedAt: 20},
		},
	}

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", service)

	request := httptest.NewRequest(http.MethodGet, "/history", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var got []types.DownloadEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(got))
	}

	if got[0].ID != "new" || got[1].ID != "middle" || got[2].ID != "old" {
		t.Fatalf("unexpected order: got [%s, %s, %s]", got[0].ID, got[1].ID, got[2].ID)
	}
}
