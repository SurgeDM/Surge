package concurrent

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/SurgeDM/Surge/internal/utils"
)

// isTransientNetworkError returns true if the error is a transient network issue
// (connection reset, timeout, unreachable) that may resolve after a network change
// or brief outage. Server-side errors (4xx/5xx) are NOT transient.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Context deadline exceeded is a timeout — transient
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Context cancelled is an intentional abort — NOT transient
	if errors.Is(err, context.Canceled) {
		return false
	}

	// EOF mid-stream means connection dropped — transient
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}

	// Syscall-level network errors
	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.ENETDOWN) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	// net.OpError wraps most TCP/DNS failures
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// DNS resolution failures
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// net.Error interface covers timeouts from the net package
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// String-based fallback for wrapped errors that lose type info
	msg := err.Error()
	transientPatterns := []string{
		"connection reset by peer",
		"broken pipe",
		"network is unreachable",
		"no route to host",
		"connection refused",
		"i/o timeout",
		"TLS handshake timeout",
		"use of closed network connection",
		"server misbehaving",       // DNS
		"no such host",             // DNS
		"dial tcp",                 // dial failures
		"write: broken pipe",      // write after disconnect
		"read: connection reset",  // read after disconnect
		"http2: client connection lost",
	}

	for _, p := range transientPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}

	return false
}

// waitForConnectivity blocks until a lightweight probe to the given URL succeeds
// or the context is cancelled / maxWait is exceeded. Returns true if connectivity
// was restored, false if we gave up.
func waitForConnectivity(ctx context.Context, client *http.Client, url string, maxWait time.Duration) bool {
	deadline := time.After(maxWait)
	// Start with a short poll interval, increase exponentially
	interval := 1 * time.Second
	maxInterval := 10 * time.Second

	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			utils.Debug("waitForConnectivity: gave up after %v", maxWait)
			return false
		default:
		}

		// Lightweight probe: HEAD or a 1-byte Range GET
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, url, nil)
		if err != nil {
			cancel()
			return false // URL is invalid, not a network issue
		}

		resp, err := client.Do(req)
		cancel()

		if err == nil {
			// Any response (even 4xx/5xx) means the network is up
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			utils.Debug("waitForConnectivity: network restored")
			return true
		}

		utils.Debug("waitForConnectivity: probe failed: %v (retrying in %v)", err, interval)

		// Wait before next probe
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			return false
		case <-time.After(interval):
		}

		// Exponential backoff
		interval = interval * 2
		if interval > maxInterval {
			interval = maxInterval
		}
	}
}
