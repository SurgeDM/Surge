package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
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

func TestNewRemoteDownloadServiceRestrictsRedirectsToSameOrigin(t *testing.T) {
	svc, err := NewRemoteDownloadService("https://example.com", "token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService failed: %v", err)
	}
	t.Cleanup(func() { _ = svc.Shutdown() })

	if svc.Client.CheckRedirect == nil {
		t.Fatal("remote API client has no redirect policy")
	}
	if svc.SSEClient.CheckRedirect == nil {
		t.Fatal("remote SSE client has no redirect policy")
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

func TestRemoteDownloadService_ListContextHonorsDeadline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer ts.Close()

	svc, err := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService failed: %v", err)
	}
	t.Cleanup(func() { _ = svc.Shutdown() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := svc.ListContext(ctx); err == nil {
		t.Fatal("expected ListContext to fail when its deadline expires")
	}
}

func TestRemoteDownloadService_StreamEventsRejectsFailedHandshake(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	defer ts.Close()

	svc, err := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService failed: %v", err)
	}
	t.Cleanup(func() { _ = svc.Shutdown() })

	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err == nil {
		t.Fatal("expected failed SSE handshake to be returned")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("unexpected handshake error: %v", err)
	}
	if stream != nil || cleanup != nil {
		t.Fatal("failed handshake returned live stream resources")
	}
}

func TestRemoteDownloadService_StreamEventsRejectsNonEventStreamResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	svc, err := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService failed: %v", err)
	}
	t.Cleanup(func() { _ = svc.Shutdown() })

	_, _, err = svc.StreamEvents(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unexpected content type") {
		t.Fatalf("expected content-type handshake error, got %v", err)
	}
}

func TestRemoteDownloadService_StreamEventsBackpressuresWithoutDroppingEvents(t *testing.T) {
	const eventCount = 150
	written := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		for i := 0; i < eventCount; i++ {
			_, _ = fmt.Fprintf(w, "event: started\ndata: {\"download_id\":\"event-%d\"}\n\n", i)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(written)
		<-r.Context().Done()
	}))
	defer ts.Close()

	svc, err := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService failed: %v", err)
	}
	t.Cleanup(func() { _ = svc.Shutdown() })

	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("StreamEvents failed: %v", err)
	}
	defer cleanup()

	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("SSE server did not send the test events")
	}

	received := make(map[string]struct{}, eventCount)
	deadline := time.After(2 * time.Second)
	for len(received) < eventCount {
		select {
		case msg, ok := <-stream:
			if !ok {
				t.Fatalf("event stream closed after receiving %d of %d events", len(received), eventCount)
			}
			received[msg.DownloadID] = struct{}{}
		case <-deadline:
			t.Fatalf("timed out after receiving %d of %d events", len(received), eventCount)
		}
	}
}

func TestRemoteDownloadService_ConsumeSSECancellationUnblocksBackpressure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan types.DownloadEvent, 1)
	ch <- types.DownloadEvent{Type: types.EventStarted}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("event: started\ndata: {\"download_id\":\"blocked\"}\n\n"))}
	done := make(chan error, 1)
	go func() {
		done <- (&RemoteDownloadService{}).consumeSSE(ctx, ch, resp)
	}()

	select {
	case err := <-done:
		t.Fatalf("consumeSSE returned before cancellation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("consumeSSE returned %v after cancellation, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("consumeSSE did not stop after cancellation")
	}
}

func TestRemoteDownloadService_StreamEventsReportsReconnect(t *testing.T) {
	var requests atomic.Int32
	secondConnected := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := requests.Add(1)
		if request == 2 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("event: started\ndata: {\"download_id\":\"stream-event\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if request == 1 {
			return
		}
		close(secondConnected)
		<-r.Context().Done()
	}))
	defer ts.Close()

	svc, err := NewRemoteDownloadService(ts.URL, "token", HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewRemoteDownloadService failed: %v", err)
	}
	t.Cleanup(func() { _ = svc.Shutdown() })

	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("StreamEvents failed: %v", err)
	}
	defer cleanup()

	var sawDisconnect, sawReconnect, sawReconnectFailure bool
	deadline := time.After(6 * time.Second)
	for !sawDisconnect || !sawReconnect || !sawReconnectFailure {
		select {
		case msg := <-stream:
			if msg.Type != types.EventSystem {
				continue
			}
			sawDisconnect = sawDisconnect || strings.Contains(msg.Message, "disconnected")
			sawReconnect = sawReconnect || strings.Contains(msg.Message, "reconnected")
			sawReconnectFailure = sawReconnectFailure || strings.Contains(msg.Message, "reconnect failed")
		case <-deadline:
			t.Fatalf("timed out waiting for reconnect status; disconnect=%v reconnect failure=%v reconnect=%v", sawDisconnect, sawReconnectFailure, sawReconnect)
		}
	}
	select {
	case <-secondConnected:
	case <-time.After(time.Second):
		t.Fatal("second SSE connection was not established")
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
