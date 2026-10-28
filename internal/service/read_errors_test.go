package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

func TestTransientReadErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{context.DeadlineExceeded, true}, {context.Canceled, false},
		{io.EOF, true}, {io.ErrUnexpectedEOF, true}, {errors.New("invalid JSON"), false},
		{&net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{&net.OpError{Op: "read", Err: context.Canceled}, false},
		{&net.DNSError{IsNotFound: true}, false},
		{&net.OpError{Op: "dial", Err: &net.DNSError{IsNotFound: true}}, false}, {&net.DNSError{IsTemporary: true}, true},
		{&apiError{StatusCode: 401}, false}, {&apiError{StatusCode: 403}, false},
		{&apiError{StatusCode: 408}, true}, {&apiError{StatusCode: 429}, true},
		{&apiError{StatusCode: 500}, true}, {&apiError{StatusCode: 501}, false},
		{&apiError{StatusCode: 502}, true}, {&apiError{StatusCode: 503}, true}, {&apiError{StatusCode: 504}, true},
	} {
		if got := IsTransientReadError(fmt.Errorf("list snapshot: %w", tc.err)); got != tc.want {
			t.Errorf("error=%v: retryable=%v, want %v", tc.err, got, tc.want)
		}
	}
}
