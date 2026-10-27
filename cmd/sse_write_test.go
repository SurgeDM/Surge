package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

type failingSSEWriter struct {
	*httptest.ResponseRecorder
	writes    int
	deadlines []time.Time
	flushErr  error
}

func (w *failingSSEWriter) Write(p []byte) (int, error) { w.writes++; return 0, io.ErrClosedPipe }
func (w *failingSSEWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}
func (w *failingSSEWriter) FlushError() error { return w.flushErr }

func TestSSEStopsOnOutputFailureAndSetsDeadline(t *testing.T) {
	for _, flushFailure := range []bool{false, true} {
		name := "write"
		if flushFailure {
			name = "flush"
		}
		t.Run(name, func(t *testing.T) {
			service := &httpAPITestService{streamMsgs: []types.DownloadEvent{{Type: types.EventQueued}, {Type: types.EventComplete}}}
			writer := &failingSSEWriter{ResponseRecorder: httptest.NewRecorder()}
			if flushFailure {
				writer.flushErr = errors.New("flush failed")
			}
			eventsHandler(service)(writer, httptest.NewRequest(http.MethodGet, "/events", nil))
			wantWrites := 1
			if flushFailure {
				wantWrites = 0
			}
			if writer.writes != wantWrites {
				t.Fatalf("writes=%d want=%d", writer.writes, wantWrites)
			}
			if len(writer.deadlines) < 2 || writer.deadlines[0].IsZero() || !writer.deadlines[len(writer.deadlines)-1].IsZero() {
				t.Fatalf("write deadlines not bounded/reset: %v", writer.deadlines)
			}
		})
	}
}

// Emulate the response controller's expired-deadline behavior without waiting
// on a real socket: advancing virtual time past an active deadline is fatal.
type idleSSEWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
	flushes  int
}

func (w *idleSSEWriter) SetWriteDeadline(deadline time.Time) error {
	if !w.deadline.IsZero() && !time.Now().Before(w.deadline) {
		return context.DeadlineExceeded
	}
	w.deadline = deadline
	return nil
}

func (w *idleSSEWriter) FlushError() error {
	if w.deadline.IsZero() || !time.Now().Before(w.deadline) {
		return context.DeadlineExceeded
	}
	w.flushes++
	return nil
}

type idleSSEService struct {
	*httpAPITestService
	stream <-chan types.DownloadEvent
}

func (s *idleSSEService) StreamEvents(context.Context) (<-chan types.DownloadEvent, func(), error) {
	return s.stream, func() {}, nil
}

func TestSSEIdleIntervalsDoNotExpireWriteDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stream := make(chan types.DownloadEvent)
		go func() {
			defer close(stream)
			for _, id := range []string{"first", "second"} {
				select {
				case <-time.After(11 * time.Second):
				case <-ctx.Done():
					return
				}
				select {
				case stream <- types.DownloadEvent{Type: types.EventQueued, DownloadID: id}:
				case <-ctx.Done():
					return
				}
			}
		}()
		writer := &idleSSEWriter{ResponseRecorder: httptest.NewRecorder()}
		service := &idleSSEService{stream: stream}
		eventsHandler(service)(writer, httptest.NewRequest(http.MethodGet, "/events", nil))
		if writer.flushes != 3 || !writer.deadline.IsZero() {
			t.Fatalf("flushes=%d deadline=%s", writer.flushes, writer.deadline)
		}
		for _, id := range []string{"first", "second"} {
			if !strings.Contains(writer.Body.String(), id) {
				t.Fatalf("idle stream lost %q: %s", id, writer.Body)
			}
		}
	})
}
