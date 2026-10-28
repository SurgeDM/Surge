package clioutput

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestResultContract(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		ok    bool
		count int
	}{
		{"empty", Success[string]("ls", nil), true, 0},
		{"detail", Success("ls", []string{"full-id"}), true, 1},
		{"failure", Failed("ls", NewError(NotFound, "Download not found.", nil)), false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := WriteJSON(&out, tc.value); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(&out)
			var result Result[string]
			if err := decoder.Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result.SchemaVersion != 1 || result.Operation != "ls" || result.OK != tc.ok || result.Items == nil || len(result.Items) != tc.count {
				t.Fatalf("unexpected result: %+v", result)
			}
			if (result.Error != nil) == tc.ok {
				t.Fatalf("unexpected error: %+v", result.Error)
			}
			if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
				t.Fatalf("extra output: %v", err)
			}
		})
	}
}

type failingWriter struct{ calls int }

func (w *failingWriter) Write(p []byte) (int, error) { w.calls++; return len(p) / 2, io.ErrClosedPipe }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestOutputFailures(t *testing.T) {
	for _, write := range []func(io.Writer) error{
		func(w io.Writer) error { return WriteJSON(w, Success("ls", []int{1})) },
		func(w io.Writer) error { return WriteText(w, "download details\n") },
	} {
		w := &failingWriter{}
		err := write(w)
		var outputError *WriteError
		if !errors.As(err, &outputError) || !errors.Is(err, io.ErrClosedPipe) || w.calls != 1 {
			t.Fatalf("write failure: %v, calls=%d", err, w.calls)
		}
		if err := write(shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("short write: %v", err)
		}
	}
	var out bytes.Buffer
	if err := WriteJSON(&out, math.NaN()); err == nil || out.Len() != 0 {
		t.Fatalf("encoding failure wrote output: %q, %v", out.String(), err)
	}
}

func TestExitCodesAndSafeMessages(t *testing.T) {
	for _, tc := range []struct {
		code Code
		exit int
	}{
		{OperationFailed, 1}, {InvalidArguments, 2}, {PartialFailure, 3},
		{NotFound, 4}, {Unauthorized, 5}, {Unavailable, 6}, {Conflict, 7}, {Timeout, 8},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			cause := errors.New("legacy diagnostic with token=secret")
			err := fmt.Errorf("context: %w", NewError(tc.code, "Safe description.", cause))
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), cause.Error()) {
				t.Fatal("lost legacy diagnostic or cause")
			}
			if got := ExitCode(err); got != tc.exit {
				t.Fatalf("exit=%d, want %d", got, tc.exit)
			}
			if got := Describe(err); got.Message != "Safe description." || got.Retryable != (tc.code == Unavailable) {
				t.Fatalf("failure: %+v", got)
			}
		})
	}
	if ExitCode(nil) != 0 {
		t.Fatal("success must exit zero")
	}
	for _, err := range []error{errors.New("https://user:secret@example.com/?token=secret"), fmt.Errorf("request: %w", &net.DNSError{Err: "secret", Name: "secret"}), context.DeadlineExceeded} {
		if strings.Contains(Describe(err).Message, "secret") {
			t.Fatal("diagnostic exposed in machine output")
		}
	}
	if ExitCode(context.DeadlineExceeded) != 8 {
		t.Fatal("deadline must be timeout")
	}
}

func TestNonNetworkURLFailureIsNotRetryable(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://example.com/?token=secret", Err: errors.New("certificate or configuration failure")}
	failure := Describe(err)
	if failure.Code != OperationFailed || failure.Retryable || strings.Contains(failure.Message, "secret") {
		t.Fatalf("unexpected failure: %+v", failure)
	}
}
