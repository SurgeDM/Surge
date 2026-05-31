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
	history          []types.DownloadEntry
	historyErr       error
	statusByID       map[string]*types.DownloadStatus
	getStatusErr     error
	streamMsgs       []interface{}
	rateLimitCalls   []string
	rateLimitValues  map[string]int64
	clearRateLimitID []string
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

func (s *httpAPITestService) SetRateLimit(id string, rate int64) error {
	if s.rateLimitCalls != nil {
		s.rateLimitCalls = append(s.rateLimitCalls, "per-download:"+id)
	}
	if s.rateLimitValues != nil {
		s.rateLimitValues[id] = rate
	}
	return nil
}

func (s *httpAPITestService) ClearRateLimit(id string) error {
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
		{name: "per-download", path: "/rate-limit?id=dl-id&rate=-1"},
		{name: "global", path: "/rate-limit/global?rate=-1"},
		{name: "default", path: "/rate-limit/default?rate=-1"},
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

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

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
func (r *rateLimitWrapper) Pause(string) error             { return nil }
func (r *rateLimitWrapper) Resume(string) error            { return nil }
func (r *rateLimitWrapper) ResumeBatch([]string) []error   { return nil }
func (r *rateLimitWrapper) UpdateURL(string, string) error { return nil }
func (r *rateLimitWrapper) Delete(string) error            { return nil }
func (r *rateLimitWrapper) StreamEvents(context.Context) (<-chan interface{}, func(), error) {
	return make(chan interface{}), func() {}, nil
}
func (r *rateLimitWrapper) Publish(interface{}) error { return nil }
func (r *rateLimitWrapper) GetStatus(id string) (*types.DownloadStatus, error) {
	return nil, errors.New("not found")
}
func (r *rateLimitWrapper) Shutdown() error                          { return nil }
func (r *rateLimitWrapper) SetRateLimit(id string, rate int64) error { return nil }

// TestRateLimitDefaultEndpoint tests the /rate-limit/default endpoint.
func TestRateLimitDefaultEndpoint(t *testing.T) {
	svc := newRateLimitTestService()
	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", svc)

	req := httptest.NewRequest(http.MethodPost, "/rate-limit/default?rate=2097152", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

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
