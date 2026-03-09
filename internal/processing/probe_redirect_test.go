package processing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/surge-downloader/surge/internal/processing"
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

	res, err := processing.ProbeServer(context.Background(), redirect.URL, "", nil)
	if err != nil {
		t.Fatalf("ProbeServer failed: %v", err)
	}

	if !res.SupportsRange {
		t.Errorf("ProbeServer did not forward Range header: SupportsRange is false!")
	}
}

func TestProbeRedirect_SameOriginPreservesAuthHeaders(t *testing.T) {
	var gotAuth, gotCookie, gotAPIKey, gotRange string

	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		gotAPIKey = r.Header.Get("X-API-Key")
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 0-0/1")
		w.WriteHeader(http.StatusPartialContent)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := processing.ProbeServer(context.Background(), server.URL+"/redirect", "", map[string]string{
		"Authorization": "Bearer same-origin",
		"Cookie":        "session=same-origin",
		"X-API-Key":     "same-origin-key",
	})
	if err != nil {
		t.Fatalf("ProbeServer failed: %v", err)
	}

	if gotAuth != "Bearer same-origin" {
		t.Fatalf("authorization = %q, want preserved", gotAuth)
	}
	if gotCookie != "session=same-origin" {
		t.Fatalf("cookie = %q, want preserved", gotCookie)
	}
	if gotAPIKey != "same-origin-key" {
		t.Fatalf("x-api-key = %q, want preserved", gotAPIKey)
	}
	if gotRange != "bytes=0-0" {
		t.Fatalf("range = %q, want bytes=0-0", gotRange)
	}
}

func TestProbeRedirect_CrossOriginDropsSensitiveHeaders(t *testing.T) {
	var gotAuth, gotCookie, gotAPIKey, gotRange string

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		gotAPIKey = r.Header.Get("X-API-Key")
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 0-0/1")
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	_, err := processing.ProbeServer(context.Background(), redirect.URL, "", map[string]string{
		"Authorization": "Bearer cross-origin",
		"Cookie":        "session=cross-origin",
		"X-API-Key":     "cross-origin-key",
	})
	if err != nil {
		t.Fatalf("ProbeServer failed: %v", err)
	}

	if gotAuth != "" {
		t.Fatalf("authorization leaked on cross-origin redirect: %q", gotAuth)
	}
	if gotCookie != "" {
		t.Fatalf("cookie leaked on cross-origin redirect: %q", gotCookie)
	}
	if gotAPIKey != "" {
		t.Fatalf("x-api-key leaked on cross-origin redirect: %q", gotAPIKey)
	}
	if gotRange != "bytes=0-0" {
		t.Fatalf("range = %q, want bytes=0-0", gotRange)
	}
}
