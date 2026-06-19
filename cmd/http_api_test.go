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

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

type httpAPITestService struct {
	history               []types.DownloadEntry
	historyErr            error
	statusByID            map[string]*types.DownloadStatus
	getStatusErr          error
	streamMsgs            []interface{}
	rateLimitCalls        []string
	rateLimitValues       map[string]int64
	clearRateLimitID      []string
	setRateLimitErr       error
	clearRateLimitErr     error
	clearCompletedReturns int64
	clearFailedReturns    int64
	clearCompletedErr     error
	clearFailedErr        error
}

func newRateLimitTestService() *httpAPITestService {
	return &httpAPITestService{
		rateLimitCalls:  make([]string, 0),
		rateLimitValues: make(map[string]int64),
	}
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
func (s *httpAPITestService) Purge(string) error {
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

type publishRecordingHTTPService struct {
	*httpAPITestService
	published []interface{}
}

func (s *publishRecordingHTTPService) Publish(msg interface{}) error {
	s.published = append(s.published, msg)
	return nil
}

type batchAddRecordingService struct {
	*httpAPITestService
	added  []string
	failOn string
}

func (s *batchAddRecordingService) Add(url string, _ string, _ string, _ []string, _ map[string]string, _ bool, _ int64, _ bool) (string, error) {
	if url == s.failOn {
		return "", errors.New("enqueue failed")
	}
	s.added = append(s.added, url)
	return "id-" + url, nil
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

func (s *httpAPITestService) ClearCompleted() (int64, error) {
	if s.clearCompletedErr != nil {
		return 0, s.clearCompletedErr
	}
	return s.clearCompletedReturns, nil
}

func (s *httpAPITestService) ClearFailed() (int64, error) {
	if s.clearFailedErr != nil {
		return 0, s.clearFailedErr
	}
	return s.clearFailedReturns, nil
}

func (s *httpAPITestService) SetRateLimit(id string, rate int64) error {
	if s.setRateLimitErr != nil {
		return s.setRateLimitErr
	}
	if s.rateLimitCalls != nil {
		s.rateLimitCalls = append(s.rateLimitCalls, "per-download:"+id)
	}
	if s.rateLimitValues != nil {
		s.rateLimitValues[id] = rate
	}
	return nil
}

func (s *httpAPITestService) ClearRateLimit(id string) error {
	if s.clearRateLimitErr != nil {
		return s.clearRateLimitErr
	}
	s.clearRateLimitID = append(s.clearRateLimitID, id)
	return nil
}

func (s *httpAPITestService) SetGlobalRateLimit(rate int64) error {
	s.rateLimitCalls = append(s.rateLimitCalls, "global")
	s.rateLimitValues["__global__"] = rate
	return nil
}

func (s *httpAPITestService) SetDefaultRateLimit(rate int64) error {
	s.rateLimitCalls = append(s.rateLimitCalls, "default")
	s.rateLimitValues["__default__"] = rate
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

func TestHandleBatchDownload_ConfirmPublishesSingleBatchRequest(t *testing.T) {
	previousProgram := serverProgram
	serverProgram = &tea.Program{}
	t.Cleanup(func() {
		serverProgram = previousProgram
	})

	service := &publishRecordingHTTPService{
		httpAPITestService: &httpAPITestService{},
	}
	body := `{
		"path": "/tmp/downloads",
		"skip_approval": false,
		"downloads": [
			{"url": "https://example.com/one.zip"},
			{"url": "https://example.com/two.zip"}
		]
	}`
	request := httptest.NewRequest(http.MethodPost, "/download/batch", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	handleBatchDownload(recorder, request, "", service)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(service.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(service.published))
	}
	msg, ok := service.published[0].(events.BatchDownloadRequestMsg)
	if !ok {
		t.Fatalf("expected BatchDownloadRequestMsg, got %T", service.published[0])
	}
	if len(msg.Requests) != 2 {
		t.Fatalf("expected 2 batch requests, got %d", len(msg.Requests))
	}
	if msg.Requests[0].URL != "https://example.com/one.zip" || msg.Requests[1].URL != "https://example.com/two.zip" {
		t.Fatalf("unexpected batch URLs: %#v", msg.Requests)
	}
}

func TestHandleBatchDownload_SkipApprovalReportsPartialFailure(t *testing.T) {
	previousLifecycle := GlobalLifecycle
	previousCleanup := GlobalLifecycleCleanup
	t.Cleanup(func() {
		GlobalLifecycle = previousLifecycle
		GlobalLifecycleCleanup = previousCleanup
	})
	GlobalLifecycle = nil
	GlobalLifecycleCleanup = nil

	service := &batchAddRecordingService{
		httpAPITestService: &httpAPITestService{},
		failOn:             "https://example.com/two.zip",
	}
	body := `{
		"path": "/tmp/downloads",
		"skip_approval": true,
		"downloads": [
			{"url": "https://example.com/one.zip"},
			{"url": "https://example.com/two.zip"},
			{"url": "https://example.com/three.zip"}
		]
	}`
	request := httptest.NewRequest(http.MethodPost, "/download/batch", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	handleBatchDownload(recorder, request, "", service)

	if recorder.Code != http.StatusMultiStatus {
		t.Fatalf("expected status 207, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(service.added) != 2 {
		t.Fatalf("expected 2 queued downloads, got %d: %#v", len(service.added), service.added)
	}

	var response struct {
		Status   string              `json:"status"`
		Count    int                 `json:"count"`
		Failures []map[string]string `json:"failures"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Status != "partial" || response.Count != 2 || len(response.Failures) != 1 {
		t.Fatalf("unexpected partial response: %#v", response)
	}
	if response.Failures[0]["url"] != "https://example.com/two.zip" {
		t.Fatalf("unexpected failed URL: %#v", response.Failures)
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

// TestRateLimitEndpoint_NegativeRateReturns400 verifies that negative rate
// values are rejected with 400 on all three rate-limit endpoints.
func TestRateLimitEndpoint_NegativeRateReturns400(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "per-download", path: "/rate-limit?id=dl-id&rate=-2"},
		{name: "global", path: "/rate-limit/global?rate=-2"},
		{name: "default", path: "/rate-limit/default?rate=-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			svc := newRateLimitTestService()
			registerHTTPRoutes(mux, 0, "", svc)

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for negative rate, got %d: %s", rec.Code, rec.Body.String())
			}
		})
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
func (s *recordingActionService) Purge(id string) error  { s.ids["purge"] = id; return nil }

// Regression for #456: ExecuteAPIAction sent the download id as a path segment
// (e.g. POST /pause/<id>), but the HTTP API registers exact routes and reads the
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
		{"purge", "/purge"},
	} {
		if err := ExecuteAPIAction(fullID, action.endpoint, http.MethodPost, action.name); err != nil {
			t.Fatalf("ExecuteAPIAction(%s): id should reach %s via ?id=, got error: %v", action.name, action.endpoint, err)
		}
		if rec.ids[action.name] != fullID {
			t.Fatalf("%s: server received id %q via query param, want %q", action.name, rec.ids[action.name], fullID)
		}
	}
}

// TestRateLimitPerDownloadEndpoint tests the /rate-limit?id=...&rate=... endpoint.
func TestRateLimitPerDownloadEndpoint(t *testing.T) {
	for _, tt := range []struct {
		name      string
		path      string
		wantCode  int
		wantID    string
		wantRate  int64
		wantClear bool
	}{
		{
			name:     "missing id returns 400",
			path:     "/rate-limit?rate=1000",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing rate returns 400",
			path:     "/rate-limit?id=dl-1",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "valid request succeeds",
			path:     "/rate-limit?id=dl-1&rate=5000000",
			wantCode: http.StatusOK,
			wantID:   "dl-1",
			wantRate: 5000000,
		},
		{
			name:     "zero rate is valid (unlimited)",
			path:     "/rate-limit?id=dl-2&rate=0",
			wantCode: http.StatusOK,
			wantID:   "dl-2",
			wantRate: 0,
		},
		{
			name:      "inherit clears explicit override",
			path:      "/rate-limit?id=dl-3&inherit=true",
			wantCode:  http.StatusOK,
			wantID:    "dl-3",
			wantClear: true,
		},
		{
			name:      "rate inherit clears explicit override",
			path:      "/rate-limit?id=dl-5&rate=inherit",
			wantCode:  http.StatusOK,
			wantID:    "dl-5",
			wantClear: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newRateLimitTestService()
			mux := http.NewServeMux()
			registerHTTPRoutes(mux, 0, "", svc)
			handler := authMiddleware("test-token", mux)

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}

			if tt.wantID != "" {
				if tt.wantClear {
					if len(svc.clearRateLimitID) != 1 || svc.clearRateLimitID[0] != tt.wantID {
						t.Fatalf("clear calls = %v, want [%s]", svc.clearRateLimitID, tt.wantID)
					}
				} else if got := svc.rateLimitValues[tt.wantID]; got != tt.wantRate {
					t.Fatalf("rate for %s = %d, want %d", tt.wantID, got, tt.wantRate)
				}
			}
		})
	}
}

func TestRateLimitPerDownloadEndpoint_NotFoundReturns404(t *testing.T) {
	tests := []struct {
		name string
		path string
		svc  *httpAPITestService
	}{
		{
			name: "set missing download",
			path: "/rate-limit?id=missing&rate=1024",
			svc:  &httpAPITestService{setRateLimitErr: types.ErrNotFound},
		},
		{
			name: "clear missing download",
			path: "/rate-limit?id=missing&inherit=true",
			svc:  &httpAPITestService{clearRateLimitErr: types.ErrNotFound},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			registerHTTPRoutes(mux, 0, "", tt.svc)

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestRateLimitGlobalEndpoint tests the /rate-limit/global endpoint.
func TestRateLimitGlobalEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantCode   int
		wantGlobal int64
		wantCall   string
	}{
		{
			name:       "valid global rate",
			path:       "/rate-limit/global?rate=1048576",
			wantCode:   http.StatusOK,
			wantGlobal: 1048576,
			wantCall:   "global",
		},
		{
			name:       "zero global rate (unlimited)",
			path:       "/rate-limit/global?rate=0",
			wantCode:   http.StatusOK,
			wantGlobal: 0,
			wantCall:   "global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newRateLimitTestService()
			mux := http.NewServeMux()
			registerHTTPRoutes(mux, 0, "", svc)
			handler := authMiddleware("test-token", mux)

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}

			found := false
			for _, c := range svc.rateLimitCalls {
				if c == tt.wantCall {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected call %q in %v", tt.wantCall, svc.rateLimitCalls)
			}
			if got := svc.rateLimitValues["__global__"]; got != tt.wantGlobal {
				t.Fatalf("global rate = %d, want %d", got, tt.wantGlobal)
			}
		})
	}
}

// TestRateLimitGlobalEndpoint_UnsupportedService returns 501 when the service
// does not implement rateLimitSettingsService.
func TestRateLimitGlobalEndpoint_UnsupportedService(t *testing.T) {
	// httpAPITestService without SetGlobalRateLimit/SetDefaultRateLimit methods
	// returns 501. But our current test service implements them.
	// Test via a minimal service that only satisfies DownloadService.
	mux := http.NewServeMux()
	svc := newRateLimitTestService()
	// Remove the rate limit methods by wrapping
	wrapper := &rateLimitWrapper{svc: svc}
	registerHTTPRoutes(mux, 0, "", wrapper)

	req := httptest.NewRequest(http.MethodPost, "/rate-limit/global?rate=1000", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for unsupported service, got %d: %s", rec.Code, rec.Body.String())
	}
}

type rateLimitWrapper struct {
	svc *httpAPITestService
}

func (r *rateLimitWrapper) List() ([]types.DownloadStatus, error)   { return nil, nil }
func (r *rateLimitWrapper) History() ([]types.DownloadEntry, error) { return nil, nil }
func (r *rateLimitWrapper) Add(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
	return "", nil
}
func (r *rateLimitWrapper) AddWithID(string, string, string, []string, map[string]string, string, int64, bool) (string, error) {
	return "", nil
}
func (r *rateLimitWrapper) Pause(string) error                              { return nil }
func (r *rateLimitWrapper) Resume(string) error                             { return nil }
func (r *rateLimitWrapper) ResumeBatch([]string) []error                    { return nil }
func (r *rateLimitWrapper) UpdateURL(string, string) error                  { return nil }
func (r *rateLimitWrapper) Delete(string) error                             { return nil }
func (r *rateLimitWrapper) Purge(string) error                              { return nil }
func (r *rateLimitWrapper) Publish(interface{}) error                       { return nil }
func (r *rateLimitWrapper) GetStatus(string) (*types.DownloadStatus, error) { return nil, nil }
func (r *rateLimitWrapper) Shutdown() error                                 { return nil }
func (r *rateLimitWrapper) ClearCompleted() (int64, error)                  { return 0, nil }
func (r *rateLimitWrapper) ClearFailed() (int64, error)                     { return 0, nil }
func (r *rateLimitWrapper) SetRateLimit(string, int64) error                { return nil }
func (r *rateLimitWrapper) ClearRateLimit(string) error                     { return nil }
func (r *rateLimitWrapper) StreamEvents(context.Context) (<-chan interface{}, func(), error) {
	return make(chan interface{}), func() {}, nil
}

// TestRateLimitDefaultEndpoint tests the /rate-limit/default endpoint.
func TestRateLimitDefaultEndpoint(t *testing.T) {
	svc := newRateLimitTestService()
	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", svc)
	handler := authMiddleware("test-token", mux)

	req := httptest.NewRequest(http.MethodPost, "/rate-limit/default?rate=2097152", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	found := false
	for _, c := range svc.rateLimitCalls {
		if c == "default" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'default' call in %v", svc.rateLimitCalls)
	}
	if got := svc.rateLimitValues["__default__"]; got != 2097152 {
		t.Fatalf("default rate = %d, want %d", got, 2097152)
	}
}

func TestClearCompletedEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		svcErr       error
		svcReturns   int64
		wantCode     int
		wantResponse string
	}{
		{
			name:     "requires POST method",
			method:   http.MethodGet,
			wantCode: http.StatusMethodNotAllowed,
		},
		{
			name:         "success",
			method:       http.MethodPost,
			svcReturns:   5,
			wantCode:     http.StatusOK,
			wantResponse: `{"deleted":5}`,
		},
		{
			name:         "service error",
			method:       http.MethodPost,
			svcErr:       errors.New("db error"),
			wantCode:     http.StatusInternalServerError,
			wantResponse: "db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &httpAPITestService{
				clearCompletedReturns: tt.svcReturns,
				clearCompletedErr:     tt.svcErr,
			}
			mux := http.NewServeMux()
			registerHTTPRoutes(mux, 0, "", svc)

			req := httptest.NewRequest(tt.method, "/clear-completed", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}

			if tt.wantResponse != "" {
				if !strings.Contains(rec.Body.String(), tt.wantResponse) {
					t.Fatalf("expected response to contain %q, got %q", tt.wantResponse, rec.Body.String())
				}
			}
		})
	}
}

func TestClearFailedEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		svcErr       error
		svcReturns   int64
		wantCode     int
		wantResponse string
	}{
		{
			name:     "requires POST method",
			method:   http.MethodGet,
			wantCode: http.StatusMethodNotAllowed,
		},
		{
			name:         "success",
			method:       http.MethodPost,
			svcReturns:   2,
			wantCode:     http.StatusOK,
			wantResponse: `{"deleted":2}`,
		},
		{
			name:         "service error",
			method:       http.MethodPost,
			svcErr:       errors.New("db fail error"),
			wantCode:     http.StatusInternalServerError,
			wantResponse: "db fail error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &httpAPITestService{
				clearFailedReturns: tt.svcReturns,
				clearFailedErr:     tt.svcErr,
			}
			mux := http.NewServeMux()
			registerHTTPRoutes(mux, 0, "", svc)

			req := httptest.NewRequest(tt.method, "/clear-failed", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}

			if tt.wantResponse != "" {
				if !strings.Contains(rec.Body.String(), tt.wantResponse) {
					t.Fatalf("expected response to contain %q, got %q", tt.wantResponse, rec.Body.String())
				}
			}
		})
	}
}
