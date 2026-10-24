package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SurgeDM/Surge/internal/utils"
)

func TestServerStopRequestsAuthenticatedShutdown(t *testing.T) {
	var called atomic.Bool
	client := newHandlerTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("health request sent token %q", got)
			}
			_, _ = w.Write([]byte(`{"status":"ok","mode":"service"}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/shutdown" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q", got)
		}
		called.Store(true)
		w.WriteHeader(http.StatusAccepted)
	}))

	previousHost, previousToken := globalHost, globalToken
	globalHost, globalToken = "http://127.0.0.1:1700", "test-token"
	t.Cleanup(func() { globalHost, globalToken = previousHost, previousToken })

	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		t.Fatalf("resolve API connection: %v", err)
	}
	if err := requestServerShutdown(serverStopCmd, client, baseURL, token); err != nil {
		t.Fatalf("stop server: %v", err)
	}
	if !called.Load() {
		t.Fatal("shutdown endpoint was not called")
	}
}

func TestServerStopRejectsUnrelatedProcessAtStalePort(t *testing.T) {
	var shutdownCalled atomic.Bool
	client := newHandlerTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/shutdown" {
			shutdownCalled.Store(true)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	previousHost, previousToken := globalHost, globalToken
	globalHost, globalToken = "http://127.0.0.1:1700", "test-token"
	t.Cleanup(func() { globalHost, globalToken = previousHost, previousToken })

	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		t.Fatalf("resolve API connection: %v", err)
	}
	if err := requestServerShutdown(serverStopCmd, client, baseURL, token); err == nil {
		t.Fatal("expected unrelated service to be rejected")
	}
	if shutdownCalled.Load() {
		t.Fatal("sent shutdown to unrelated service")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func newHandlerTestClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
}

type blockingAddService struct {
	*httpAPITestService
	entered chan struct{}
	release chan struct{}
}

func (s *blockingAddService) Add(_ string, _ string, _ string, _ []string, _ map[string]string, _ bool, _ int, _ int64) (string, error) {
	close(s.entered)
	<-s.release
	return "queued-id", nil
}

func TestBatchTracksEnqueueUntilRequestCompletes(t *testing.T) {
	globalLifecycleMu.Lock()
	previousLifecycle := GlobalLifecycle
	GlobalLifecycle = nil
	globalLifecycleMu.Unlock()
	t.Cleanup(func() {
		globalLifecycleMu.Lock()
		GlobalLifecycle = previousLifecycle
		globalLifecycleMu.Unlock()
	})

	service := &blockingAddService{
		httpAPITestService: &httpAPITestService{},
		entered:            make(chan struct{}),
		release:            make(chan struct{}),
	}
	startCount := atomic.LoadInt32(&pendingEnqueue)
	request := httptest.NewRequest(http.MethodPost, "/download/batch", strings.NewReader(`{"skip_approval":true,"downloads":[{"url":"https://example.com/file"}]}`))
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handleBatchDownload(recorder, request, "", service)
		close(done)
	}()
	<-service.entered
	if got := atomic.LoadInt32(&pendingEnqueue); got != startCount+1 {
		close(service.release)
		<-done
		t.Fatalf("pending enqueues = %d, want %d while Add runs", got, startCount+1)
	}
	close(service.release)
	<-done
	if got := atomic.LoadInt32(&pendingEnqueue); got != startCount {
		t.Fatalf("pending enqueues = %d, want %d after request", got, startCount)
	}
}

func TestAPIRejectsOversizedDownloadRequest(t *testing.T) {
	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", &httpAPITestService{})
	body := fmt.Sprintf(`{"url":"https://example.com/%s"}`, strings.Repeat("a", maxAPIRequestBodyBytes))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader(body))
	limitAPIRequestBody(mux).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAPIRejectsOversizedTrailingRequestBody(t *testing.T) {
	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", &httpAPITestService{})
	body := `{"url":"https://example.com/file"}` + strings.Repeat(" ", maxAPIRequestBodyBytes)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader(body))
	limitAPIRequestBody(mux).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAPIRejectsSecondJSONValue(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader(`{"url":"https://example.com/file"} {}`))
	var decoded DownloadRequest
	if err := decodeJSONBody(request, &decoded); err == nil {
		t.Fatal("accepted a second JSON value")
	}
}

func TestDownloadRequestDoesNotLogCredentials(t *testing.T) {
	setupIsolatedCmdState(t)
	logDir := t.TempDir()
	utils.CloseDebug()
	utils.ConfigureDebug(logDir)

	body, err := json.Marshal(DownloadRequest{
		URL: "https://example.com/file?signature=query-secret",
		Headers: map[string]string{
			"Cookie":        "session=cookie-secret",
			"Authorization": "Bearer header-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader(string(body)))
	if _, err := resolveDownloadRequest(request, ""); err != nil {
		t.Fatalf("resolve download request: %v", err)
	}
	utils.CloseDebug()

	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read debug log: entries=%d, error=%v", len(entries), err)
	}
	logData, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"query-secret", "cookie-secret", "header-secret"} {
		if strings.Contains(string(logData), secret) {
			t.Fatalf("debug log exposed %q", secret)
		}
	}
}

func TestBatchRejectsExcessiveDownloadCount(t *testing.T) {
	service := &batchAddRecordingService{httpAPITestService: &httpAPITestService{}}
	var body strings.Builder
	body.WriteString(`{"skip_approval":true,"downloads":[`)
	for i := 0; i <= maxBatchDownloads; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`{"url":"https://example.com/file"}`)
	}
	body.WriteString(`]}`)
	recorder := httptest.NewRecorder()
	handleBatchDownload(recorder, httptest.NewRequest(http.MethodPost, "/download/batch", strings.NewReader(body.String())), "", service)
	if recorder.Code != http.StatusRequestEntityTooLarge || len(service.added) != 0 {
		t.Fatalf("status = %d, queued = %d; want 413 and no enqueues", recorder.Code, len(service.added))
	}
}

func TestBatchDuplicateDownloadsFieldReplacesEarlierItems(t *testing.T) {
	var request BatchDownloadRequest
	data := []byte(`{"downloads":[{"url":"https://example.com/first"}],"downloads":[{"url":"https://example.com/second"}]}`)
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	if len(request.Downloads) != 1 || request.Downloads[0].URL != "https://example.com/second" {
		t.Fatalf("downloads = %#v, want only second item", request.Downloads)
	}
}

func TestInvalidDownloadRequestReturnsNoUsableRequest(t *testing.T) {
	for _, body := range []string{
		`{"url":"https://example.com/file","filename":"../outside"}`,
		`{"url":"https://example.com/file","filename":"C:outside"}`,
		`{"url":"https://example.com/file","filename":"file:stream"}`,
		`{"url":"https://example.com/file","filename":"."}`,
		`{"url":"https://example.com/file","path":"../outside"}`,
		`{"url":"https://example.com/file","path":"bad\u0000path"}`,
		`{"filename":"unsafe","path":"outside"}`,
		`{"url":"https://example.com/file","path":"outside"} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader(body))
			got, err := decodeAndValidateDownloadRequest(request)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if got.URL != "" || got.Filename != "" || got.Path != "" {
				t.Fatalf("failed validation returned request data: %#v", got)
			}
		})
	}
}
