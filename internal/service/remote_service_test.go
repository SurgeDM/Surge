package service

import (
	_ "github.com/SurgeDM/Surge/internal/types"

	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemoteDownloadService_SetRateLimit_ProxiesRequest(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate-limit" && r.URL.Query().Get("id") == "test-id" && r.URL.Query().Get("rate") == "100" {
			called = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc, _ := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	t.Cleanup(func() { _ = svc.Shutdown() })

	err := svc.SetRateLimit("test-id", 100)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected rate limit endpoint to be called")
	}
}

func TestRemoteDownloadService_ClearRateLimit_ProxiesRequest(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate-limit" && r.URL.Query().Get("id") == "test-id" && r.URL.Query().Get("inherit") == "true" {
			called = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc, _ := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	t.Cleanup(func() { _ = svc.Shutdown() })

	err := svc.ClearRateLimit("test-id")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected rate limit clear endpoint to be called")
	}
}

func TestRemoteDownloadService_SetGlobalRateLimit_ProxiesRequest(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate-limit/global" && r.URL.Query().Get("rate") == "200" {
			called = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc, _ := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	t.Cleanup(func() { _ = svc.Shutdown() })

	err := svc.SetGlobalRateLimit(200)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected global rate limit endpoint to be called")
	}
}

func TestRemoteDownloadService_SetDefaultRateLimit_ProxiesRequest(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate-limit/default" && r.URL.Query().Get("rate") == "300" {
			called = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	svc, _ := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	t.Cleanup(func() { _ = svc.Shutdown() })

	err := svc.SetDefaultRateLimit(300)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected default rate limit endpoint to be called")
	}
}

func TestRemoteDownloadService_NegativeRates_Rejected(t *testing.T) {
	svc, _ := NewRemoteDownloadService("http://localhost:0", "token", HTTPClientOptions{})
	t.Cleanup(func() { _ = svc.Shutdown() })

	if err := svc.SetRateLimit("id", -1); err == nil {
		t.Errorf("expected error setting negative rate limit")
	}
	if err := svc.SetGlobalRateLimit(-1); err == nil {
		t.Errorf("expected error setting negative global rate limit")
	}
	if err := svc.SetDefaultRateLimit(-1); err == nil {
		t.Errorf("expected error setting negative default rate limit")
	}
}

func TestRemoteDownloadService_StreamEvents_ShutdownClosesChannel(t *testing.T) {
	blockCh := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-blockCh:
		case <-r.Context().Done():
		}
	}))
	defer ts.Close()
	defer close(blockCh)

	svc, _ := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{
		Timeout: 5 * time.Second,
	})

	ch, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("StreamEvents failed: %v", err)
	}

	// Shutdown the service, which should cancel the context and close the channel
	if err := svc.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// The channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("expected channel to be closed, but got message")
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timed out waiting for channel to close after shutdown")
	}

	cleanup()
}

func TestRemoteDownloadService_StreamEvents_CleanupClosesChannel(t *testing.T) {
	blockCh := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-blockCh:
		case <-r.Context().Done():
		}
	}))
	defer ts.Close()
	defer close(blockCh)

	svc, _ := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	t.Cleanup(func() { _ = svc.Shutdown() })

	ch, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("StreamEvents failed: %v", err)
	}

	// Cleanup should close the channel
	cleanup()

	// The channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("expected channel to be closed, but got message")
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timed out waiting for channel to close after cleanup")
	}
}

func TestRemoteDownloadService_StreamEvents_ReceivesMessages(t *testing.T) {
	blockCh := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		msg := "event: started\ndata: {\"download_id\":\"test-1\",\"filename\":\"test.txt\"}\n\n"
		_, _ = w.Write([]byte(msg))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		select {
		case <-blockCh:
		case <-r.Context().Done():
		}
	}))
	defer ts.Close()
	defer close(blockCh)

	svc, _ := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	t.Cleanup(func() { _ = svc.Shutdown() })

	ch, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("StreamEvents failed: %v", err)
	}
	defer cleanup()

	select {
	case msg := <-ch:
		startedMsg := msg
		ok := true
		if !ok {
			t.Errorf("expected DownloadStartedMsg, got %T", msg)
		}
		if startedMsg.DownloadID != "test-1" {
			t.Errorf("expected DownloadID test-1, got %s", startedMsg.DownloadID)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timed out waiting for message")
	}
}
