package engine_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/surge-downloader/surge/internal/engine"
)

func TestProbeRedirectRange(t *testing.T) {
	// Destination server supports range
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=0-0" {
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "x")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// Redirect server
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	res, err := engine.ProbeServer(context.Background(), redirect.URL, "", nil)
	if err != nil {
		t.Fatalf("ProbeServer failed: %v", err)
	}

	if !res.SupportsRange {
		t.Errorf("ProbeServer did not forward Range header: SupportsRange is false!")
	}
}

func TestProbeServer_RecoversFilenameAfterRangeProbe(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Initial probe path: 206 response without Content-Disposition.
		if r.Method == http.MethodGet && r.Header.Get("Range") == "bytes=0-0" {
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "x")
			return
		}

		// Fallback metadata probe should recover filename from headers.
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Disposition", `attachment; filename="queued-file.zip"`)
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Content-Disposition", `attachment; filename="queued-file.zip"`)
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "0123456789")
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	res, err := engine.ProbeServer(context.Background(), redirect.URL, "", nil)
	if err != nil {
		t.Fatalf("ProbeServer failed: %v", err)
	}

	if res.Filename != "queued-file.zip" {
		t.Fatalf("filename = %q, want %q", res.Filename, "queued-file.zip")
	}
}
