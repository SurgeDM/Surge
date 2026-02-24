package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/surge-downloader/surge/internal/engine"
)

func TestProbeRedirectRange(t *testing.T) {
	// Destination server supports range
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=0-0" {
			w.WriteHeader(http.StatusPartialContent)
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
