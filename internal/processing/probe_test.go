package processing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/processing"
)

func TestProbeServer_UsesConfiguredProxy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var directHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directHits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer target.Close()

	var proxyHits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		w.Header().Set("Content-Range", "bytes 0-0/1")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer proxy.Close()

	settings := config.DefaultSettings()
	settings.Network.ProxyURL = proxy.URL
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}

	result, err := processing.ProbeServer(context.Background(), target.URL, "", nil)
	if err != nil {
		t.Fatalf("ProbeServer() error = %v", err)
	}
	if !result.SupportsRange {
		t.Fatal("ProbeServer() did not use proxy-backed partial-content response")
	}
	if proxyHits.Load() == 0 {
		t.Fatal("expected probe request to go through configured proxy")
	}
	if directHits.Load() != 0 {
		t.Fatalf("expected target to be unreachable directly during proxy test, got %d direct hits", directHits.Load())
	}
}

func TestProbeMirrors_PreservesCallerOrderAfterDedupe(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(75 * time.Millisecond)
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer slow.Close()

	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer fast.Close()

	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
	}))
	defer invalid.Close()

	valid, errs := processing.ProbeMirrorsWithProxy(context.Background(), []string{
		slow.URL,
		fast.URL,
		slow.URL,
		invalid.URL,
	}, "")

	want := []string{slow.URL, fast.URL}
	if len(valid) != len(want) {
		t.Fatalf("len(valid) = %d, want %d (%v)", len(valid), len(want), valid)
	}
	for i := range want {
		if valid[i] != want[i] {
			t.Fatalf("valid[%d] = %q, want %q", i, valid[i], want[i])
		}
	}

	if len(errs) != 1 {
		t.Fatalf("len(errs) = %d, want 1 (%v)", len(errs), errs)
	}
	if _, ok := errs[invalid.URL]; !ok {
		t.Fatalf("expected invalid mirror failure for %s, got %v", invalid.URL, errs)
	}
}

func TestProbeServer_ReadsBodyBeforeContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="delayed.txt"`)
		w.Header().Set("Content-Range", "bytes 0-0/1000")
		w.WriteHeader(http.StatusPartialContent)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Delay body to ensure DetermineFilename blocking on io.ReadFull is not interrupted by premature context cancellation
		time.Sleep(100 * time.Millisecond)
		if _, err := w.Write([]byte("x")); err != nil {
			t.Errorf("ProbeServer() failed to write body: %v", err)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := processing.ProbeServer(ctx, server.URL, "", nil)
	if err != nil {
		t.Fatalf("ProbeServer() failed: %v", err)
	}
	if result.Filename != "delayed.txt" {
		t.Errorf("Expected filename 'delayed.txt', got %q. The context might have been prematurely canceled.", result.Filename)
	}
}
