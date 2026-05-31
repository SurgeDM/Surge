package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRemoteDownloadService_StreamEvents_CleanupClosesChannel(t *testing.T) {
	var once sync.Once
	connected := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = fmt.Fprint(w, ": ping\n\n")
			f.Flush()
		}

		once.Do(func() { close(connected) })
		<-r.Context().Done()
	}))
	defer server.Close()

	svc, err := NewRemoteDownloadService(server.URL, "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService returned error: %v", err)
	}
	t.Cleanup(func() { _ = svc.Shutdown() })

	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("StreamEvents returned error: %v", err)
	}
	t.Cleanup(cleanup)

	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE connection")
	}

	cleanup()

	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("expected stream channel to close after cleanup")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream channel to close after cleanup")
	}
}

func TestRemoteDownloadService_StreamEvents_ShutdownClosesChannel(t *testing.T) {
	var once sync.Once
	connected := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = fmt.Fprint(w, ": ping\n\n")
			f.Flush()
		}

		once.Do(func() { close(connected) })
		<-r.Context().Done()
	}))
	defer server.Close()

	svc, err := NewRemoteDownloadService(server.URL, "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService returned error: %v", err)
	}

	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("StreamEvents returned error: %v", err)
	}
	t.Cleanup(cleanup)

	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE connection")
	}

	if err := svc.Shutdown(); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("expected stream channel to close after shutdown")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream channel to close after shutdown")
	}
}

// fakeRateLimitServer is a test HTTP server that validates rate-limit requests.
type fakeRateLimitServer struct {
	*httptest.Server
	mu           sync.Mutex
	perDlCalls   []perDlRateCall
	clearCalls   []string
	globalCalls  []int64
	defaultCalls []int64
}

type perDlRateCall struct {
	id   string
	rate int64
}

func newFakeRateLimitServer() *fakeRateLimitServer {
	s := &fakeRateLimitServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rate-limit":
			id := r.URL.Query().Get("id")
			if r.URL.Query().Get("inherit") == "true" {
				s.mu.Lock()
				s.clearCalls = append(s.clearCalls, id)
				s.mu.Unlock()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"rate_limit_inherited"}`))
				return
			}
			rateStr := r.URL.Query().Get("rate")
			rate, _ := strconv.ParseInt(rateStr, 10, 64)
			s.mu.Lock()
			s.perDlCalls = append(s.perDlCalls, perDlRateCall{id: id, rate: rate})
			s.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"rate_limited"}`))
		case r.URL.Path == "/rate-limit/global":
			rateStr := r.URL.Query().Get("rate")
			rate, _ := strconv.ParseInt(rateStr, 10, 64)
			s.mu.Lock()
			s.globalCalls = append(s.globalCalls, rate)
			s.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"global_rate_limited"}`))
		case r.URL.Path == "/rate-limit/default":
			rateStr := r.URL.Query().Get("rate")
			rate, _ := strconv.ParseInt(rateStr, 10, 64)
			s.mu.Lock()
			s.defaultCalls = append(s.defaultCalls, rate)
			s.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"default_rate_limited"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	return s
}

func TestRemoteDownloadService_SetRateLimit_ProxiesRequest(t *testing.T) {
	srv := newFakeRateLimitServer()
	defer srv.Close()

	svc, err := NewRemoteDownloadService(srv.URL, "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService: %v", err)
	}
	defer func() { _ = svc.Shutdown() }()

	if err := svc.SetRateLimit("dl-abc", 2_000_000); err != nil {
		t.Fatalf("SetRateLimit: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.perDlCalls) != 1 {
		t.Fatalf("expected 1 per-download call, got %d", len(srv.perDlCalls))
	}
	call := srv.perDlCalls[0]
	if call.id != "dl-abc" {
		t.Errorf("id = %q, want dl-abc", call.id)
	}
	if call.rate != 2_000_000 {
		t.Errorf("rate = %d, want %d", call.rate, 2_000_000)
	}
}

func TestRemoteDownloadService_ClearRateLimit_ProxiesRequest(t *testing.T) {
	srv := newFakeRateLimitServer()
	defer srv.Close()

	svc, err := NewRemoteDownloadService(srv.URL, "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService: %v", err)
	}
	defer func() { _ = svc.Shutdown() }()

	if err := svc.ClearRateLimit("dl-abc"); err != nil {
		t.Fatalf("ClearRateLimit: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.clearCalls) != 1 {
		t.Fatalf("expected 1 clear call, got %d", len(srv.clearCalls))
	}
	if srv.clearCalls[0] != "dl-abc" {
		t.Errorf("id = %q, want dl-abc", srv.clearCalls[0])
	}
}

func TestRemoteDownloadService_SetGlobalRateLimit_ProxiesRequest(t *testing.T) {
	srv := newFakeRateLimitServer()
	defer srv.Close()

	svc, err := NewRemoteDownloadService(srv.URL, "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService: %v", err)
	}
	defer func() { _ = svc.Shutdown() }()

	if err := svc.SetGlobalRateLimit(5_000_000); err != nil {
		t.Fatalf("SetGlobalRateLimit: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.globalCalls) != 1 {
		t.Fatalf("expected 1 global call, got %d", len(srv.globalCalls))
	}
	if srv.globalCalls[0] != 5_000_000 {
		t.Errorf("rate = %d, want %d", srv.globalCalls[0], 5_000_000)
	}
}

func TestRemoteDownloadService_SetDefaultRateLimit_ProxiesRequest(t *testing.T) {
	srv := newFakeRateLimitServer()
	defer srv.Close()

	svc, err := NewRemoteDownloadService(srv.URL, "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService: %v", err)
	}
	defer func() { _ = svc.Shutdown() }()

	if err := svc.SetDefaultRateLimit(1_000_000); err != nil {
		t.Fatalf("SetDefaultRateLimit: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.defaultCalls) != 1 {
		t.Fatalf("expected 1 default call, got %d", len(srv.defaultCalls))
	}
	if srv.defaultCalls[0] != 1_000_000 {
		t.Errorf("rate = %d, want %d", srv.defaultCalls[0], 1_000_000)
	}
}

func TestRemoteDownloadService_SetRateLimit_RejectsNegativeRate(t *testing.T) {
	svc, err := NewRemoteDownloadService("http://127.0.0.1:1", "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService: %v", err)
	}
	defer func() { _ = svc.Shutdown() }()

	err = svc.SetRateLimit("dl-1", -1)
	if err == nil {
		t.Fatal("expected error for negative rate")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("expected 'non-negative' error, got: %v", err)
	}
}

func TestRemoteDownloadService_SetGlobalRateLimit_RejectsNegativeRate(t *testing.T) {
	svc, err := NewRemoteDownloadService("http://127.0.0.1:1", "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService: %v", err)
	}
	defer func() { _ = svc.Shutdown() }()

	err = svc.SetGlobalRateLimit(-1)
	if err == nil {
		t.Fatal("expected error for negative rate")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("expected 'non-negative' error, got: %v", err)
	}
}

func TestRemoteDownloadService_SetDefaultRateLimit_RejectsNegativeRate(t *testing.T) {
	svc, err := NewRemoteDownloadService("http://127.0.0.1:1", "test-token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService: %v", err)
	}
	defer func() { _ = svc.Shutdown() }()

	err = svc.SetDefaultRateLimit(-1)
	if err == nil {
		t.Fatal("expected error for negative rate")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("expected 'non-negative' error, got: %v", err)
	}
}
