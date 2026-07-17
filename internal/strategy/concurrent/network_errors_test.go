package concurrent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"
)

func TestIsTransientNetworkError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"context canceled", context.Canceled, false},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"EOF", io.EOF, true},
		{"syscall ECONNRESET", syscall.ECONNRESET, true},
		{"syscall ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"net OpError", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, true},
		{"DNS Error", &net.DNSError{Err: "no such host"}, true},
		{"wrapped generic string", fmt.Errorf("some other error: connection reset by peer"), true},
		{"wrapped generic string broken pipe", fmt.Errorf("write: broken pipe"), true},
		{"non-transient error", errors.New("HTTP 404 Not Found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientNetworkError(tt.err); got != tt.want {
				t.Errorf("isTransientNetworkError() = %v, want %v for err: %v", got, tt.want, tt.err)
			}
		})
	}
}

func TestWaitForConnectivity_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := server.Client()
	ctx := context.Background()

	// Should succeed immediately
	if !waitForConnectivity(ctx, client, server.URL, 5*time.Second) {
		t.Error("waitForConnectivity() returned false, want true")
	}
}

func TestWaitForConnectivity_Timeout(t *testing.T) {
	// A non-routable IP to force a timeout
	client := &http.Client{
		Timeout: 10 * time.Millisecond,
	}
	ctx := context.Background()

	// Should timeout
	start := time.Now()
	if waitForConnectivity(ctx, client, "http://192.0.2.1", 200*time.Millisecond) {
		t.Error("waitForConnectivity() returned true, want false")
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Error("waitForConnectivity() returned too quickly")
	}
}

func TestWaitForConnectivity_Cancel(t *testing.T) {
	client := &http.Client{
		Timeout: 1 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Should return quickly due to cancel
	start := time.Now()
	if waitForConnectivity(ctx, client, "http://192.0.2.1", 5*time.Second) {
		t.Error("waitForConnectivity() returned true, want false")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Error("waitForConnectivity() took too long to cancel")
	}
}
