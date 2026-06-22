package processing

import (
	"context"
	"testing"
)

func TestEnqueue_PerTaskOverrideForwarding(t *testing.T) {
	tests := []struct {
		name         string
		workers      int
		minChunkSize int64
		wantWorkers  int
		wantMinChunk int64
	}{
		{
			name:         "zero values stay zero",
			workers:      0,
			minChunkSize: 0,
			wantWorkers:  0,
			wantMinChunk: 0,
		},
		{
			name:         "Enqueue forwards both fields",
			workers:      8,
			minChunkSize: 5 * 1024 * 1024,
			wantWorkers:  8,
			wantMinChunk: 5 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newLifecycleManagerForTest()

			var gotWorkers int
			var gotMinChunkSize int64
			mgr.addFunc = func(_, _, _ string, _ []string, _ map[string]string, _ bool, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
				gotWorkers = workers
				gotMinChunkSize = minChunkSize
				return "id", nil
			}

			server := newProbeTestServer(t, 1024)
			defer server.Close()

			_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
				URL:          server.URL,
				Filename:     "test.bin",
				Path:         t.TempDir(),
				Workers:      tt.workers,
				MinChunkSize: tt.minChunkSize,
			})
			if err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
			if gotWorkers != tt.wantWorkers {
				t.Fatalf("expected workers=%d, got %d", tt.wantWorkers, gotWorkers)
			}
			if gotMinChunkSize != tt.wantMinChunk {
				t.Fatalf("expected minChunkSize=%d, got %d", tt.wantMinChunk, gotMinChunkSize)
			}
		})
	}
}

func TestEnqueueWithID_PerTaskOverrideForwarding(t *testing.T) {
	tests := []struct {
		name         string
		workers      int
		minChunkSize int64
		wantWorkers  int
		wantMinChunk int64
	}{
		{
			name:         "zero values stay zero",
			workers:      0,
			minChunkSize: 0,
			wantWorkers:  0,
			wantMinChunk: 0,
		},
		{
			name:         "EnqueueWithID forwards both fields",
			workers:      8,
			minChunkSize: 5 * 1024 * 1024,
			wantWorkers:  8,
			wantMinChunk: 5 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newLifecycleManagerForTest()

			var gotWorkers int
			var gotMinChunkSize int64
			mgr.addWithIDFunc = func(_, _, _ string, _ []string, _ map[string]string, _ string, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
				gotWorkers = workers
				gotMinChunkSize = minChunkSize
				return "id", nil
			}

			server := newProbeTestServer(t, 1024)
			defer server.Close()

			_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
				URL:          server.URL,
				Filename:     "test.bin",
				Path:         t.TempDir(),
				Workers:      tt.workers,
				MinChunkSize: tt.minChunkSize,
			}, "req-1")
			if err != nil {
				t.Fatalf("EnqueueWithID failed: %v", err)
			}
			if gotWorkers != tt.wantWorkers {
				t.Fatalf("expected workers=%d, got %d", tt.wantWorkers, gotWorkers)
			}
			if gotMinChunkSize != tt.wantMinChunk {
				t.Fatalf("expected minChunkSize=%d, got %d", tt.wantMinChunk, gotMinChunkSize)
			}
		})
	}
}
