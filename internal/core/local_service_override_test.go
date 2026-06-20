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

func TestAdd_PerTaskOverride_ZeroValues(t *testing.T) {
	ch := make(chan interface{}, 8)
	pool := download.NewWorkerPool(ch, 1)
	svc := NewLocalDownloadServiceWithInput(pool, ch)
	defer func() { _ = svc.Shutdown() }()

	outputDir := t.TempDir()
	id, err := svc.Add("https://example.com/file.bin", outputDir, "file.bin", nil, nil, false, 0, 0, 0, false)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	cfg := findConfigByID(pool, id)
	if cfg == nil {
		t.Fatal("expected config in pool")
	}
	if cfg.Runtime.Workers != 0 {
		t.Fatalf("expected Runtime.Workers=0, got %d", cfg.Runtime.Workers)
	}
	if cfg.Runtime.MinChunkSize != types.MinChunk {
		t.Fatalf("expected Runtime.MinChunkSize=%d (default), got %d", types.MinChunk, cfg.Runtime.MinChunkSize)
	}
}

func TestAdd_PerTaskOverride_WorkersSet(t *testing.T) {
	ch := make(chan interface{}, 8)
	pool := download.NewWorkerPool(ch, 1)
	svc := NewLocalDownloadServiceWithInput(pool, ch)
	defer func() { _ = svc.Shutdown() }()

	outputDir := t.TempDir()
	id, err := svc.Add("https://example.com/file.bin", outputDir, "file.bin", nil, nil, false, 16, 0, 0, false)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	cfg := findConfigByID(pool, id)
	if cfg == nil {
		t.Fatal("expected config in pool")
	}
	if cfg.Runtime.Workers != 16 {
		t.Fatalf("expected Runtime.Workers=16, got %d", cfg.Runtime.Workers)
	}
}

func TestAdd_PerTaskOverride_MinChunkSizeSet(t *testing.T) {
	ch := make(chan interface{}, 8)
	pool := download.NewWorkerPool(ch, 1)
	svc := NewLocalDownloadServiceWithInput(pool, ch)
	defer func() { _ = svc.Shutdown() }()

	outputDir := t.TempDir()
	minChunk := int64(10 * types.MB)
	id, err := svc.Add("https://example.com/file.bin", outputDir, "file.bin", nil, nil, false, 0, minChunk, 0, false)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	cfg := findConfigByID(pool, id)
	if cfg == nil {
		t.Fatal("expected config in pool")
	}
	if cfg.Runtime.MinChunkSize != minChunk {
		t.Fatalf("expected Runtime.MinChunkSize=%d, got %d", minChunk, cfg.Runtime.MinChunkSize)
	}
}

func TestAdd_PerTaskOverride_BothSet(t *testing.T) {
	ch := make(chan interface{}, 8)
	pool := download.NewWorkerPool(ch, 1)
	svc := NewLocalDownloadServiceWithInput(pool, ch)
	defer func() { _ = svc.Shutdown() }()

	outputDir := t.TempDir()
	minChunk := int64(5 * types.MB)
	id, err := svc.Add("https://example.com/file.bin", outputDir, "file.bin", nil, nil, false, 8, minChunk, 0, false)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	cfg := findConfigByID(pool, id)
	if cfg == nil {
		t.Fatal("expected config in pool")
	}
	if cfg.Runtime.Workers != 8 {
		t.Fatalf("expected Runtime.Workers=8, got %d", cfg.Runtime.Workers)
	}
	if cfg.Runtime.MinChunkSize != minChunk {
		t.Fatalf("expected Runtime.MinChunkSize=%d, got %d", minChunk, cfg.Runtime.MinChunkSize)
	}
}
