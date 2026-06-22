package core

import (
	"testing"

	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

func findConfigByID(pool *download.WorkerPool, id string) *types.DownloadConfig {
	for _, cfg := range pool.GetAll() {
		if cfg.ID == id {
			return &cfg
		}
	}
	return nil
}

func TestAdd_PerTaskOverride(t *testing.T) {
	tests := []struct {
		name         string
		workers      int
		minChunkSize int64
		wantWorkers  int
		wantMinChunk int64
		checkClamped bool
	}{
		{
			name:         "zero/defaults",
			workers:      0,
			minChunkSize: 0,
			wantWorkers:  0,
			wantMinChunk: types.MinChunk,
		},
		{
			name:         "workers-only",
			workers:      16,
			minChunkSize: 0,
			wantWorkers:  16,
			wantMinChunk: types.MinChunk,
		},
		{
			name:         "minChunk-only",
			workers:      0,
			minChunkSize: 10 * types.MB,
			wantWorkers:  0,
			wantMinChunk: 10 * types.MB,
		},
		{
			name:         "workers-clamped",
			workers:      64,
			minChunkSize: 0,
			checkClamped: true,
			wantMinChunk: types.MinChunk,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan interface{}, 8)
			pool := download.NewWorkerPool(ch, 1)
			svc := NewLocalDownloadServiceWithInput(pool, ch)
			defer func() { _ = svc.Shutdown() }()

			outputDir := t.TempDir()
			id, err := svc.Add("https://example.com/file.bin", outputDir, "file.bin", nil, nil, false, tt.workers, tt.minChunkSize, 0, false)
			if err != nil {
				t.Fatalf("Add failed: %v", err)
			}

			cfg := findConfigByID(pool, id)
			if cfg == nil {
				t.Fatal("expected config in pool")
			}

			if tt.checkClamped {
				maxConns := cfg.Runtime.GetMaxConnectionsPerDownload()
				if cfg.Runtime.Workers != maxConns {
					t.Fatalf("expected Runtime.Workers=%d (clamped to MaxConns), got %d", maxConns, cfg.Runtime.Workers)
				}
				if cfg.Runtime.MinChunkSize != tt.wantMinChunk {
					t.Fatalf("expected Runtime.MinChunkSize=%d (default), got %d", tt.wantMinChunk, cfg.Runtime.MinChunkSize)
				}
				return
			}

			if cfg.Runtime.Workers != tt.wantWorkers {
				t.Fatalf("expected Runtime.Workers=%d, got %d", tt.wantWorkers, cfg.Runtime.Workers)
			}
			if cfg.Runtime.MinChunkSize != tt.wantMinChunk {
				t.Fatalf("expected Runtime.MinChunkSize=%d, got %d", tt.wantMinChunk, cfg.Runtime.MinChunkSize)
			}
		})
	}
}
