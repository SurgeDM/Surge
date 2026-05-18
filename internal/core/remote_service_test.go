package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
