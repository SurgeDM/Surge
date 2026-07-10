package probe_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/probe"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestProbeServer_UsesConfiguredProxy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())

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

	runCfg := &types.RuntimeConfig{ProxyURL: proxy.URL}

	result, err := probe.ProbeServerWithProxy(context.Background(), target.URL, "", nil, runCfg)
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

	valid, errs := probe.ProbeMirrorsWithProxy(context.Background(), []string{
		slow.URL,
		fast.URL,
		slow.URL,
		invalid.URL,
	}, nil)

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := probe.ProbeServerWithProxy(ctx, server.URL, "", nil, nil)
	if err != nil {
		t.Fatalf("ProbeServerWithProxy() failed: %v", err)
	}
	if result.Filename != "delayed.txt" {
		t.Errorf("Expected filename 'delayed.txt', got %q. The context might have been prematurely canceled.", result.Filename)
	}
}

func TestProbeServer_RangeSupported(t *testing.T) {
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(1024*1024), // 1MB
		testutil.WithRangeSupport(true),
		testutil.WithFilename("testfile.bin"),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := probe.ProbeServer(ctx, server.URL(), "", nil)
	if err != nil {
		t.Fatalf("probeServer failed: %v", err)
	}

	if !result.SupportsRange {
		t.Error("Expected SupportsRange to be true")
	}
	if result.FileSize != 1024*1024 {
		t.Errorf("Expected FileSize 1048576, got %d", result.FileSize)
	}
}

func TestProbeServer_RangeNotSupported(t *testing.T) {
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(2048),
		testutil.WithRangeSupport(false),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := probe.ProbeServer(ctx, server.URL(), "", nil)
	if err != nil {
		t.Fatalf("probeServer failed: %v", err)
	}

	if result.SupportsRange {
		t.Error("Expected SupportsRange to be false")
	}
	if result.FileSize != 2048 {
		t.Errorf("Expected FileSize 2048, got %d", result.FileSize)
	}
}

func TestProbeServer_CustomFilenameHint(t *testing.T) {
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(1024),
		testutil.WithFilename("server-file.zip"),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Provide a custom filename hint
	result, err := probe.ProbeServer(ctx, server.URL(), "my-custom-file.zip", nil)
	if err != nil {
		t.Fatalf("probeServer failed: %v", err)
	}

	if result.Filename != "my-custom-file.zip" {
		t.Errorf("Expected Filename 'my-custom-file.zip', got '%s'", result.Filename)
	}
}

func TestProbeServer_ContentType(t *testing.T) {
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(1024),
		testutil.WithContentType("application/zip"),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := probe.ProbeServer(ctx, server.URL(), "", nil)
	if err != nil {
		t.Fatalf("probeServer failed: %v", err)
	}

	if result.ContentType != "application/zip" {
		t.Errorf("Expected ContentType 'application/zip', got '%s'", result.ContentType)
	}
}

func TestProbeServer_InvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := probe.ProbeServer(ctx, "http://invalid-host-that-does-not-exist.test:9999/file", "", nil)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestProbeServer_ContextCancellation(t *testing.T) {
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(1024),
		testutil.WithLatency(5*time.Second), // Long latency
	)
	defer server.Close()

	// Cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := probe.ProbeServer(ctx, server.URL(), "", nil)
	if err == nil {
		t.Error("Expected error when context is cancelled")
	}
}

func TestProbeServer_UnexpectedStatusCode(t *testing.T) {
	// Create a custom server that returns 404
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := probe.ProbeServer(ctx, server.URL, "", nil)
	if err == nil {
		t.Error("Expected error for 404 status")
	}
}

func TestProbeServer_ServerError(t *testing.T) {
	// Create a custom server that returns 500
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := probe.ProbeServer(ctx, server.URL, "", nil)
	if err == nil {
		t.Error("Expected error for 500 status")
	}
}

func TestProbeServer_ZeroFileSize(t *testing.T) {
	// Server returns 200 OK with no Content-Length header
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := probe.ProbeServer(ctx, server.URL, "", nil)
	if err != nil {
		t.Fatalf("probeServer failed: %v", err)
	}

	// FileSize should be 0 when Content-Length is missing
	if result.FileSize != 0 {
		t.Errorf("Expected FileSize 0, got %d", result.FileSize)
	}
}

func TestProbeServer_ContentRangeFormats(t *testing.T) {
	tests := []struct {
		name          string
		contentRange  string
		expectedSize  int64
		supportsRange bool
	}{
		{
			name:          "Standard format",
			contentRange:  "bytes 0-0/1048576",
			expectedSize:  1048576,
			supportsRange: true,
		},
		{
			name:          "Unknown size",
			contentRange:  "bytes 0-0/*",
			expectedSize:  0,
			supportsRange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Range", tt.contentRange)
				w.Header().Set("Content-Length", "1")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("x"))
			}))
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := probe.ProbeServer(ctx, server.URL, "", nil)
			if err != nil {
				t.Fatalf("probeServer failed: %v", err)
			}

			if result.SupportsRange != tt.supportsRange {
				t.Errorf("SupportsRange = %v, want %v", result.SupportsRange, tt.supportsRange)
			}
			if result.FileSize != tt.expectedSize {
				t.Errorf("FileSize = %d, want %d", result.FileSize, tt.expectedSize)
			}
		})
	}
}

func TestProbeServer_LargeFile(t *testing.T) {
	// Test with a large file size (10GB)
	largeSize := int64(10 * 1024 * 1024 * 1024) // 10GB

	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", largeSize))
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := probe.ProbeServer(ctx, server.URL, "", nil)
	if err != nil {
		t.Fatalf("probeServer failed: %v", err)
	}

	if result.FileSize != largeSize {
		t.Errorf("FileSize = %d, want %d", result.FileSize, largeSize)
	}
}

func TestProbeResult_Fields(t *testing.T) {
	pr := &probe.ProbeResult{
		FileSize:      123456789,
		SupportsRange: true,
		Filename:      "document.pdf",
		ContentType:   "application/pdf",
	}

	if pr.FileSize != 123456789 {
		t.Errorf("FileSize = %d, want 123456789", pr.FileSize)
	}
	if !pr.SupportsRange {
		t.Error("SupportsRange should be true")
	}
	if pr.Filename != "document.pdf" {
		t.Errorf("Filename = '%s', want 'document.pdf'", pr.Filename)
	}
	if pr.ContentType != "application/pdf" {
		t.Errorf("ContentType = '%s', want 'application/pdf'", pr.ContentType)
	}
}

func TestProbeResult_ZeroValues(t *testing.T) {
	pr := &probe.ProbeResult{}

	if pr.FileSize != 0 {
		t.Errorf("FileSize = %d, want 0", pr.FileSize)
	}
	if pr.SupportsRange {
		t.Error("SupportsRange should be false by default")
	}
	if pr.Filename != "" {
		t.Errorf("Filename = '%s', want empty", pr.Filename)
	}
	if pr.ContentType != "" {
		t.Errorf("ContentType = '%s', want empty", pr.ContentType)
	}
}
