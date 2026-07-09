package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestProgressAggregator_Loop(t *testing.T) {
	// 1. Setup a test server that blocks so the download stays active
	blockCh := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		<-blockCh // block the download
	}))
	defer ts.Close()
	defer close(blockCh)

	// 2. Setup Pool and EventBus
	progressCh := make(chan types.DownloadEvent, 10)
	pool := scheduler.New(progressCh, 1)
	eb := NewEventBus()
	defer eb.Shutdown()

	// 3. Start Aggregator
	settings := config.DefaultSettings()
	agg := NewProgressAggregator(pool, eb, settings)
	defer agg.Shutdown()

	// 4. Start Download
	state := progress.New("agg-test", 1024)
	tmpDir := t.TempDir()
	cfg := types.DownloadRecord{
		ID:            "agg-test",
		URL:           ts.URL,
		OutputPath:    tmpDir,
		Filename:      "test.txt",
		ProgressState: state,
		TotalSize:     1024,
		Runtime:       types.DefaultRuntimeConfig(),
	}
	pool.Add(cfg)

	// Update state manually to simulate progress
	state.Bytes.Downloaded.Store(512)
	state.Bytes.VerifiedProgress.Store(512)

	// 5. Subscribe and check for BatchProgressMsg
	sub, cleanup := eb.Subscribe()
	defer cleanup()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case msg := <-sub:
			if msg.Type == types.EventBatchProgress {
				if len(msg.BatchEvents) > 0 {
					pMsg := msg.BatchEvents[0]
					if pMsg.DownloadID == "agg-test" {
						if pMsg.Downloaded == 512 {
							return // Success!
						}
					}
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for BatchProgressMsg with 512 bytes downloaded")
		}
	}
}

func TestProgressAggregator_Settings(t *testing.T) {
	agg := NewProgressAggregator(nil, nil, nil)
	defer agg.Shutdown()

	if agg.getSpeedEmaAlpha() != SpeedSmoothingAlpha {
		t.Errorf("expected default alpha, got %v", agg.getSpeedEmaAlpha())
	}

	settings := config.DefaultSettings()
	settings.Performance.SpeedEmaAlpha.Value = 0.5
	agg.SetSettings(settings)

	if agg.getSpeedEmaAlpha() != 0.5 {
		t.Errorf("expected 0.5 alpha, got %v", agg.getSpeedEmaAlpha())
	}
}


