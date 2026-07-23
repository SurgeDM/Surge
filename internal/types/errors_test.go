package types

import (
	"errors"
	"net/http"
	"testing"
)

func TestIsPermanentHTTPError(t *testing.T) {
	if !IsPermanentHTTPError(ErrPermanentHTTP) {
		t.Errorf("Expected IsPermanentHTTPError(ErrPermanentHTTP) to be true")
	}

	wrappedErr := errors.Join(errors.New("some error"), ErrPermanentHTTP)
	if !IsPermanentHTTPError(wrappedErr) {
		t.Errorf("Expected IsPermanentHTTPError to be true for wrapped ErrPermanentHTTP")
	}

	if IsPermanentHTTPError(errors.New("other error")) {
		t.Errorf("Expected IsPermanentHTTPError to be false for other errors")
	}
}

func TestIsPermanentHTTPStatus(t *testing.T) {
	tests := []struct {
		status   int
		expected bool
	}{
		{http.StatusOK, false},
		{http.StatusMovedPermanently, false},
		{http.StatusBadRequest, true},           // 400
		{http.StatusUnauthorized, true},         // 401
		{http.StatusForbidden, true},            // 403
		{http.StatusNotFound, true},             // 404
		{http.StatusTooManyRequests, false},     // 429 is explicitly transient
		{http.StatusInternalServerError, false}, // 500
		{http.StatusServiceUnavailable, false},  // 503
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			if got := IsPermanentHTTPStatus(tt.status); got != tt.expected {
				t.Errorf("IsPermanentHTTPStatus(%d) = %v, want %v", tt.status, got, tt.expected)
			}
		})
	}
}
