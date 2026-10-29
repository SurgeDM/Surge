package concurrent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
)

type lifecycleTransport func(*http.Request) (*http.Response, error)

func (f lifecycleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type lifecycleBody struct {
	io.Reader
	closed bool
}

func (b *lifecycleBody) Close() error { b.closed = true; return nil }

type cancelingReader struct{ cancel context.CancelFunc }

func (r cancelingReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

func TestWorkerTaskLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name               string
		status             int
		retryAfter         string
		staleEpisode       bool
		cancelAfterPartial bool
		diskFull           bool
		payload            string
		wantErr            error
		wantWritten        int64
		wantPreserved      bool
	}{
		{name: "success", status: 206, payload: "abcdefgh", wantWritten: 8},
		{name: "retry-after-budget", status: 429, retryAfter: "3600", wantErr: types.ErrRetryBudgetExceeded},
		{name: "sustained-throttle-budget", status: 503, staleEpisode: true, wantErr: types.ErrRetryBudgetExceeded},
		{name: "cancel-after-write", status: 206, payload: "abcd", cancelAfterPartial: true, wantErr: context.Canceled, wantWritten: 4, wantPreserved: true},
		{name: "disk-full", status: 206, payload: "abcdefgh", diskFull: true, wantErr: types.ErrInsufficientDiskSpace, wantPreserved: true},
		{name: "truncated-body", status: 206, payload: "abcd", wantWritten: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			file, err := os.CreateTemp(t.TempDir(), "worker")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			state := progress.New(tc.name, 8)
			d := NewConcurrentDownloader(tc.name, nil, state, &types.RuntimeConfig{MaxTaskRetries: 1})
			d.hostLimiter = transport.NewHostRateLimiter()
			d.concurrencyGate = newAdaptiveConcurrencyGate(1, 0)
			if tc.staleEpisode {
				d.throttleEpisodeStart = time.Now().Add(-11 * time.Minute)
			}
			if tc.diskFull {
				previous := writeAtFn
				writeAtFn = func(*os.File, []byte, int64) (int, error) { return 0, types.ErrInsufficientDiskSpace }
				t.Cleanup(func() { writeAtFn = previous })
			}
			var reader io.Reader = strings.NewReader(tc.payload)
			if tc.cancelAfterPartial {
				reader = io.MultiReader(reader, cancelingReader{cancel})
			}
			body := &lifecycleBody{Reader: reader}
			requests := 0
			client := &http.Client{Transport: lifecycleTransport(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": []string{tc.retryAfter}}, Body: body}, nil
			})}
			queue := NewTaskQueue()
			queue.Push(types.Task{Length: 8})
			queue.Close()
			err = d.worker(ctx, 0, []string{"https://example.invalid/file"}, file, queue, 8, client)
			if tc.name == "truncated-body" {
				if err == nil {
					t.Fatal("truncated response accepted")
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v, want %v", err, tc.wantErr)
			}
			if requests != 1 || !body.closed {
				t.Fatalf("requests=%d, body closed=%v", requests, body.closed)
			}
			if state.ActiveWorkers.Load() != 0 || d.concurrencyGate.admitted != 0 || len(d.activeTasks) != 0 {
				t.Fatalf("task resources leaked: workers=%d admitted=%d active=%d", state.ActiveWorkers.Load(), d.concurrencyGate.admitted, len(d.activeTasks))
			}
			if got := state.Bytes.Downloaded.Load(); got != tc.wantWritten {
				t.Fatalf("written=%d, want %d", got, tc.wantWritten)
			}
			queued := queue.DrainRemaining()
			preserved := d.abandonedRemaining
			if tc.wantPreserved {
				if len(queued) != 0 {
					t.Fatalf("terminal residual requeued: %v", queued)
				}
			} else if len(preserved) != 0 {
				t.Fatalf("unexpected preserved residual: %v", preserved)
			}
			remaining := append(queued, preserved...)
			if tc.wantWritten == 8 {
				if len(remaining) != 0 {
					t.Fatalf("completed range duplicated: %v", remaining)
				}
			} else if len(remaining) != 1 || remaining[0] != (types.Task{Offset: tc.wantWritten, Length: 8 - tc.wantWritten}) {
				t.Fatalf("unfinished range lost or duplicated: %v", remaining)
			}
		})
	}
}
