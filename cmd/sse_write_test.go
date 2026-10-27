package cmd

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
