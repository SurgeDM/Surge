package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
)

func TestAppSnapshot_NilReceiver(t *testing.T) {
	var app *App

	if got := app.Snapshot(); got != (RuntimeSnapshot{}) {
		t.Fatalf("Snapshot() = %+v, want zero value", got)
	}
}

func TestAppSnapshot_WithPopulatedPool(t *testing.T) {
	settings := config.DefaultSettings()
	app := NewLocal(settings)
	app.ApplyComponents(Components{
		Pool:       app.Pool(),
		ProgressCh: app.ProgressCh(),
		Lifecycle:  processing.NewLifecycleManager(nil, nil),
	})

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-release
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	outputDir := t.TempDir()
	app.Pool().Add(types.DownloadConfig{
		ID:         "snapshot-id",
		URL:        server.URL,
		OutputPath: outputDir,
		Filename:   "snapshot.bin",
		State:      types.NewProgressState("snapshot-id", 1),
		Runtime:    &types.RuntimeConfig{},
	})

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker to start")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := app.Snapshot()
		if snapshot.SettingsLoaded && !snapshot.ServiceReady && snapshot.LifecycleReady &&
			snapshot.ActiveCount > 0 && snapshot.DownloadCount > 0 {
			close(release)
			if err := app.Shutdown(); err != nil {
				t.Fatalf("Shutdown() error = %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(release)
	_ = app.Shutdown()
	t.Fatalf("expected populated snapshot, got %+v", app.Snapshot())
}
