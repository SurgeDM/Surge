package service

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSameOriginRedirectPolicy(t *testing.T) {
	tests := []struct {
		name    string
		first   string
		next    string
		wantErr bool
	}{
		{name: "same origin", first: "https://example.com/start", next: "https://example.com/next"},
		{name: "explicit default port", first: "https://example.com/start", next: "https://example.com:443/next"},
		{name: "hostname case", first: "https://EXAMPLE.com/start", next: "https://example.COM/next"},
		{name: "different scheme", first: "http://example.com/start", next: "https://example.com/next", wantErr: true},
		{name: "different hostname", first: "https://example.com/start", next: "https://api.example.com/next", wantErr: true},
		{name: "different port", first: "https://example.com/start", next: "https://example.com:8443/next", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, err := http.NewRequest(http.MethodGet, tt.first, nil)
			if err != nil {
				t.Fatalf("create first request: %v", err)
			}
			next, err := http.NewRequest(http.MethodGet, tt.next, nil)
			if err != nil {
				t.Fatalf("create redirect request: %v", err)
			}

			err = SameOriginRedirectPolicy(next, []*http.Request{first})
			if (err != nil) != tt.wantErr {
				t.Fatalf("SameOriginRedirectPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSameOriginRedirectPolicyComparesWithFirstRequest(t *testing.T) {
	first, _ := http.NewRequest(http.MethodGet, "https://example.com/start", nil)
	intermediate, _ := http.NewRequest(http.MethodGet, "https://other.example/step", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://other.example/end", nil)

	if err := SameOriginRedirectPolicy(next, []*http.Request{first, intermediate}); err == nil {
		t.Fatal("expected redirect matching only the previous request to be rejected")
	}
}

func TestNewHTTPClientKeepsDefaultRedirectBehavior(t *testing.T) {
	client, err := NewHTTPClient(HTTPClientOptions{})
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	if client.CheckRedirect != nil {
		t.Fatal("generic HTTP client unexpectedly has a custom redirect policy")
	}
}

func TestNewHTTPClient_InvalidCAFile(t *testing.T) {
	_, err := NewHTTPClient(HTTPClientOptions{
		CAFile: "does-not-exist.pem",
	})
	if err == nil {
		t.Fatal("expected invalid CA file to return an error")
	}
	if !strings.Contains(err.Error(), "does-not-exist.pem") {
		t.Fatalf("error %q does not mention CA file path", err)
	}
}

func TestNewHTTPTransport_ResponseHeaderTimeout(t *testing.T) {
	want := 3 * time.Second
	transport, err := NewHTTPTransport(HTTPClientOptions{ResponseHeaderTimeout: want})
	if err != nil {
		t.Fatalf("NewHTTPTransport failed: %v", err)
	}
	if transport.ResponseHeaderTimeout != want {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, want)
	}
}
