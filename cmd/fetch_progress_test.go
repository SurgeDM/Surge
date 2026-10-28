package cmd

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
	"github.com/charmbracelet/x/ansi"
)

type blockedProgressWriter struct {
	entered, release chan struct{}
	once             sync.Once
}

func (w *blockedProgressWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(data), nil
}

func TestProgressWriterCannotBlockSnapshotUpdatesOrShutdown(t *testing.T) {
	writer := &blockedProgressWriter{entered: make(chan struct{}), release: make(chan struct{})}
	command := newFetchCommand()
	command.SetErr(writer)
	_ = command.Flags().Set("progress", "plain")
	progress := startFetchProgress(command)
	defer func() { close(writer.release); progress.stop(); <-progress.done }()
	progress.update(&types.DownloadStatus{ID: "owned", Filename: "file", Status: "downloading", Downloaded: 1, TotalSize: 100, Speed: 1})
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("renderer did not start")
	}
	done := make(chan struct{})
	go func() {
		for i := range 1000 {
			progress.update(&types.DownloadStatus{ID: "owned", Status: "downloading", Downloaded: int64(i)})
		}
		progress.stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked terminal stalled updates or shutdown")
	}
}
func TestProgressFramesHandleTinyResizesAndHostileNames(t *testing.T) {
	progress := &fetchProgress{mode: "bar", color: "never", ascii: true, writer: io.Discard}
	snapshots := []progressSnapshot{{ID: "id", Filename: "\x1b]0;title\a" + strings.Repeat("large界", 40) + ".gguf", State: "downloading", Downloaded: 100, Total: 1000, Speed: 10, Connections: 2, Capacity: 8}}
	for _, size := range [][2]int{{0, 0}, {1, 1}, {20, 3}, {40, 8}, {120, 30}} {
		for _, line := range progress.progressFrame(snapshots, size[0], size[1]) {
			if strings.ContainsAny(line, "\x1b\a\n") {
				t.Fatalf("control characters leaked: %q", line)
			}
			if ansi.StringWidth(line) > max(0, size[0]-1) {
				t.Fatalf("frame overflow at %v: %q", size, line)
			}
		}
	}
	if got := truncateProgressFilename(strings.Repeat("x", 100)+".gguf", 20, true); !strings.HasSuffix(got, ".gguf") {
		t.Fatalf("extension lost: %q", got)
	}
	unknown := progress.progressLine(progressSnapshot{ID: "id", State: "downloading", Downloaded: 200}, 0)
	if strings.Contains(unknown, "%") || !strings.Contains(unknown, "ETA --") {
		t.Fatalf("unknown-size progress fabricated completion: %s", unknown)
	}
}
func TestQuietDoesNotSuppressRequestedJSON(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	code, _, out := executeFetchTest(t, context.Background(), "--quiet")
	if code != 2 || !strings.Contains(out, "invalid_arguments") {
		t.Fatalf("quiet hid JSON failure: %d %s", code, out)
	}
}
