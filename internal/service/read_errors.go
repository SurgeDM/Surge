package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
)

// IsTransientReadError identifies snapshot failures worth retrying. It must not
// be used to replay mutations, whose outcome may be unknown after a disconnect.
func IsTransientReadError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var responseError *apiError
	if errors.As(err, &responseError) {
		switch responseError.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return dnsError.IsTimeout || dnsError.IsTemporary
	}
	var networkError *net.OpError
	return errors.As(err, &networkError)
}
