package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

type httpAPITestService struct {
	history      []types.DownloadEntry
	historyErr   error
	statusByID   map[string]*types.DownloadStatus
	getStatusErr error
	streamMsgs   []interface{}
}

func (s *httpAPITestService) List(ctx context.Context) ([]types.DownloadStatus, error) {
	return nil, nil
}

func (s *httpAPITestService) History(ctx context.Context) ([]types.DownloadEntry, error) {
	if s.historyErr != nil {
		return nil, s.historyErr
	}
	return s.history, nil
}

func (s *httpAPITestService) Add(context.Context, string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
	return "", errors.New("not implemented")
}

func (s *httpAPITestService) AddWithID(context.Context, string, string, string, []string, map[string]string, string, int64, bool) (string, error) {
	return "", errors.New("not implemented")
}

func (s *httpAPITestService) Pause(context.Context, string) error {
	return nil
}

func (s *httpAPITestService) Resume(context.Context, string) error {
	return nil
}

func (s *httpAPITestService) ResumeBatch(context.Context, []string) []error {
	return nil
}

func (s *httpAPITestService) UpdateURL(context.Context, string, string) error {
	return nil
}

func (s *httpAPITestService) Delete(context.Context, string) error {
	return nil
}

func (s *httpAPITestService) StreamEvents(context.Context) (<-chan interface{}, func(), error) {
	channel := make(chan interface{}, len(s.streamMsgs))
	for _, msg := range s.streamMsgs {
		channel <- msg
	}
	close(channel)
	cleanup := func() {}
	return channel, cleanup, nil
}

func (s *httpAPITestService) Publish(interface{}) error {
	return nil
}

func (s *httpAPITestService) GetStatus(ctx context.Context, id string) (*types.DownloadStatus, error) {
	if s.getStatusErr != nil {
		return nil, s.getStatusErr
	}
	if s.statusByID == nil {
		return nil, errors.New("not found")
	}
	status, ok := s.statusByID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return status, nil
}

func (s *httpAPITestService) Shutdown() error {
	return nil
}

func TestEnsureOpenActionRequestAllowed_RemoteToggle(t *testing.T) {
	original := globalSettings
	t.Cleanup(func() {
		globalSettings = original
	})

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/open-file?id=example", nil)
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

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/history", nil)
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

func TestEventsEndpoint_RequiresAuthAndStreamsSSE(t *testing.T) {
	service := &httpAPITestService{
		streamMsgs: []interface{}{
			events.DownloadQueuedMsg{
				DownloadID: "queue-1",
				Filename:   "archive.zip",
				URL:        "https://example.com/archive.zip",
				DestPath:   "/tmp/archive.zip",
			},
		},
	}

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", service)
	handler := corsMiddleware(authMiddleware("test-token", mux))
	server := httptest.NewServer(handler)
	defer server.Close()

	noAuthReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	noAuthResp, err := server.Client().Do(noAuthReq)
	if err != nil {
		t.Fatalf("request without auth failed: %v", err)
	}
	defer func() { _ = noAuthResp.Body.Close() }()
	if noAuthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", noAuthResp.StatusCode)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create authed request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("authed request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with auth, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read SSE body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "event: queued") {
		t.Fatalf("expected queued SSE event, got %q", text)
	}
	if !strings.Contains(text, `"DownloadID":"queue-1"`) {
		t.Fatalf("expected queued payload in SSE body, got %q", text)
	}
}

func TestResolveDownloadDestPath(t *testing.T) {
	tests := []struct {
		name           string
		useNilService  bool
		service        *httpAPITestService
		id             string
		wantPath       string
		wantErrIs      error
		wantErrContain string
	}{
		{
			name:          "service unavailable",
			useNilService: true,
			id:            "x",
			wantErrIs:     ErrServiceUnavailable,
		},
		{
			name: "status path present",
			service: &httpAPITestService{
				statusByID: map[string]*types.DownloadStatus{
					"hit": {ID: "hit", DestPath: "C:\\tmp\\a.bin"},
				},
			},
			id:       "hit",
			wantPath: `C:\tmp\a.bin`,
		},
		{
			name: "status path empty falls back to history",
			service: &httpAPITestService{
				statusByID: map[string]*types.DownloadStatus{
					"fallback": {ID: "fallback", DestPath: ""},
				},
				history: []types.DownloadEntry{{ID: "fallback", DestPath: "C:\\tmp\\b.bin"}},
			},
			id:       "fallback",
			wantPath: `C:\tmp\b.bin`,
		},
		{
			name: "history entry has no destination path",
			service: &httpAPITestService{
				history: []types.DownloadEntry{{ID: "bad", DestPath: "."}},
			},
			id:        "bad",
			wantErrIs: ErrNoDestinationPath,
		},
		{
			name: "id absent returns not found",
			service: &httpAPITestService{
				history: []types.DownloadEntry{{ID: "other", DestPath: "C:\\tmp\\c.bin"}},
			},
			id:        "missing",
			wantErrIs: ErrDownloadNotFound,
		},
		{
			name: "history read failure bubbles as internal",
			service: &httpAPITestService{
				historyErr: errors.New("db down"),
			},
			id:             "x",
			wantErrContain: "failed to read history",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var service core.DownloadService
			if !test.useNilService {
				service = test.service
			}

			gotPath, err := resolveDownloadDestPath(service, test.id)

			if test.wantErrIs == nil && test.wantErrContain == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				if gotPath != test.wantPath {
					t.Fatalf("expected path %q, got %q", test.wantPath, gotPath)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if test.wantErrIs != nil && !errors.Is(err, test.wantErrIs) {
				t.Fatalf("expected errors.Is(%v), got %v", test.wantErrIs, err)
			}
			if test.wantErrContain != "" && !strings.Contains(err.Error(), test.wantErrContain) {
				t.Fatalf("expected error containing %q, got %q", test.wantErrContain, err.Error())
			}
		})
	}
}

func TestOpenEndpoints_ReturnMappedResolveStatuses(t *testing.T) {
	original := globalSettings
	t.Cleanup(func() {
		globalSettings = original
	})
	globalSettings = config.DefaultSettings()

	tests := []struct {
		name       string
		path       string
		useNil     bool
		service    *httpAPITestService
		statusCode int
	}{
		{
			name:       "service unavailable returns 503",
			path:       "/open-file?id=missing",
			useNil:     true,
			statusCode: http.StatusServiceUnavailable,
		},
		{
			name: "missing download returns 404",
			path: "/open-folder?id=missing",
			service: &httpAPITestService{
				history: []types.DownloadEntry{},
			},
			statusCode: http.StatusNotFound,
		},
		{
			name: "history read failure returns 500",
			path: "/open-file?id=broken",
			service: &httpAPITestService{
				historyErr: errors.New("db down"),
			},
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			var service core.DownloadService
			if !test.useNil {
				service = test.service
			}
			registerHTTPRoutes(mux, 0, "", service)

			request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, test.path, nil)
			request.RemoteAddr = "127.0.0.1:12345"
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, request)

			if recorder.Code != test.statusCode {
				t.Fatalf("expected status %d, got %d, body=%s", test.statusCode, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestEnsureOpenActionRequestAllowed_ForwardedLoopbackDenied(t *testing.T) {
	original := globalSettings
	t.Cleanup(func() {
		globalSettings = original
	})

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/open-file?id=example", nil)
	request.RemoteAddr = "127.0.0.1:23456"
	request.Header.Set("X-Forwarded-For", "198.51.100.10")

	globalSettings = config.DefaultSettings()
	if err := ensureOpenActionRequestAllowed(request); err == nil {
		t.Fatal("expected forwarded loopback request to be denied by default")
	}

	globalSettings = config.DefaultSettings()
	globalSettings.General.AllowRemoteOpenActions = true
	if err := ensureOpenActionRequestAllowed(request); err != nil {
		t.Fatalf("expected forwarded loopback request to be allowed when enabled, got: %v", err)
	}
}
