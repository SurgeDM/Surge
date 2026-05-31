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

func (s *httpAPITestService) List() ([]types.DownloadStatus, error) {
	return nil, nil
}

func (s *httpAPITestService) History() ([]types.DownloadEntry, error) {
	if s.historyErr != nil {
		return nil, s.historyErr
	}
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

func (s *httpAPITestService) GetStatus(id string) (*types.DownloadStatus, error) {
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

	request := httptest.NewRequest(http.MethodPost, "/open-file?id=example", nil)
	request.RemoteAddr = "203.0.113.8:12345"

	globalSettings = config.DefaultSettings()
	if err := ensureOpenActionRequestAllowed(request); err == nil {
		t.Fatal("expected remote open action to be denied by default")
	}

	globalSettings = config.DefaultSettings()
	globalSettings.General.AllowRemoteOpenActions.Value = true
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

	noAuthResp, err := server.Client().Get(server.URL + "/events")
	if err != nil {
		t.Fatalf("request without auth failed: %v", err)
	}
	defer func() { _ = noAuthResp.Body.Close() }()
	if noAuthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", noAuthResp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/events", nil)
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

			request := httptest.NewRequest(http.MethodPost, test.path, nil)
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

	request := httptest.NewRequest(http.MethodPost, "/open-file?id=example", nil)
	request.RemoteAddr = "127.0.0.1:23456"
	request.Header.Set("X-Forwarded-For", "198.51.100.10")

	globalSettings = config.DefaultSettings()
	if err := ensureOpenActionRequestAllowed(request); err == nil {
		t.Fatal("expected forwarded loopback request to be denied by default")
	}

	globalSettings = config.DefaultSettings()
	globalSettings.General.AllowRemoteOpenActions.Value = true
	if err := ensureOpenActionRequestAllowed(request); err != nil {
		t.Fatalf("expected forwarded loopback request to be allowed when enabled, got: %v", err)
	}
}

// recordingActionService records the id passed to each lifecycle action so
// tests can assert how the CLI delivered it to the HTTP API.
type recordingActionService struct {
	*httpAPITestService
	ids map[string]string // action -> received id
}

func (s *recordingActionService) Pause(id string) error  { s.ids["pause"] = id; return nil }
func (s *recordingActionService) Resume(id string) error { s.ids["resume"] = id; return nil }
func (s *recordingActionService) Delete(id string) error { s.ids["delete"] = id; return nil }

// Regression for #456: ExecuteAPIAction sent the download id as a path segment
// (e.g. POST /pause/<id>), but the HTTP API registers exact routes and reads the
// id from the "id" query parameter (withRequiredID), so pause/resume/delete/open
// 404'd against a remote daemon. The id must be sent as ?id=. Exercise every
// ExecuteAPIAction caller (pause/resume/delete), not just one, so a future
// action-specific regression is caught.
func TestExecuteAPIAction_SendsIDAsQueryParam(t *testing.T) {
	rec := &recordingActionService{httpAPITestService: &httpAPITestService{}, ids: map[string]string{}}
	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", rec)
	server := httptest.NewServer(mux)
	defer server.Close()

	prevHost, prevToken := globalHost, globalToken
	globalHost, globalToken = "", ""
	defer func() { globalHost, globalToken = prevHost, prevToken }()
	t.Setenv("SURGE_HOST", server.URL)
	t.Setenv("SURGE_TOKEN", "test-token")

	// 32 chars so resolveDownloadID treats it as a full id (no server lookup).
	const fullID = "abcdef0123456789abcdef0123456789"
	for _, action := range []struct{ name, endpoint string }{
		{"pause", "/pause"},
		{"resume", "/resume"},
		{"delete", "/delete"},
	} {
		if err := ExecuteAPIAction(fullID, action.endpoint, http.MethodPost, action.name); err != nil {
			t.Fatalf("ExecuteAPIAction(%s): id should reach %s via ?id=, got error: %v", action.name, action.endpoint, err)
		}
		if rec.ids[action.name] != fullID {
			t.Fatalf("%s: server received id %q via query param, want %q", action.name, rec.ids[action.name], fullID)
		}
	}
}
