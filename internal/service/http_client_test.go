package service

import (
	"strings"
	"testing"
	"time"
)

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
