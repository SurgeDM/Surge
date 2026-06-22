package processing

import (
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

func TestBuildResumeConfig_OverridesFromSavedState(t *testing.T) {
	settings := config.DefaultSettings()
	entry := &types.DownloadEntry{
		ID:           "test-id",
		URL:          "http://example.com/file.zip",
		DestPath:     "/tmp/file.zip",
		Filename:     "file.zip",
		TotalSize:    1000,
		Workers:      4,
		MinChunkSize: 5 * types.MB,
	}
	savedState := &types.DownloadState{
		ID:           "test-id",
		URL:          "http://example.com/file.zip",
		DestPath:     "/tmp/file.zip",
		Filename:     "file.zip",
		TotalSize:    1000,
		Downloaded:   500,
		Workers:      8,
		MinChunkSize: 5 * types.MB,
	}

	cfg := buildResumeConfig("test-id", "/tmp", entry, savedState, settings)
	if cfg.Runtime.Workers != 8 {
		t.Fatalf("expected Runtime.Workers=8 (from savedState), got %d", cfg.Runtime.Workers)
	}
	if cfg.Runtime.MinChunkSize != 5*types.MB {
		t.Fatalf("expected Runtime.MinChunkSize=%d (from savedState), got %d", 5*types.MB, cfg.Runtime.MinChunkSize)
	}
}

func TestBuildResumeConfig_OverridesFallbackToEntry(t *testing.T) {
	settings := config.DefaultSettings()
	entry := &types.DownloadEntry{
		ID:           "test-id",
		URL:          "http://example.com/file.zip",
		DestPath:     "/tmp/file.zip",
		Filename:     "file.zip",
		TotalSize:    1000,
		Workers:      8,
		MinChunkSize: 5 * types.MB,
	}

	cfg := buildResumeConfig("test-id", "/tmp", entry, nil, settings)
	if cfg.Runtime.Workers != 8 {
		t.Fatalf("expected Runtime.Workers=8 (from entry fallback), got %d", cfg.Runtime.Workers)
	}
	if cfg.Runtime.MinChunkSize != 5*types.MB {
		t.Fatalf("expected Runtime.MinChunkSize=%d (from entry fallback), got %d", 5*types.MB, cfg.Runtime.MinChunkSize)
	}
}

func TestBuildResumeConfig_SavedStatePriorityForMinChunkSize(t *testing.T) {
	settings := config.DefaultSettings()
	entry := &types.DownloadEntry{
		ID:           "test-id",
		URL:          "http://example.com/file.zip",
		DestPath:     "/tmp/file.zip",
		Filename:     "file.zip",
		TotalSize:    1000,
		Workers:      4,
		MinChunkSize: 2 * types.MB,
	}
	savedState := &types.DownloadState{
		ID:           "test-id",
		URL:          "http://example.com/file.zip",
		DestPath:     "/tmp/file.zip",
		Filename:     "file.zip",
		TotalSize:    1000,
		Downloaded:   500,
		Workers:      8,
		MinChunkSize: 10 * types.MB,
	}

	cfg := buildResumeConfig("test-id", "/tmp", entry, savedState, settings)
	if cfg.Runtime.Workers != 8 {
		t.Fatalf("expected Runtime.Workers=8 (from savedState), got %d", cfg.Runtime.Workers)
	}
	if cfg.Runtime.MinChunkSize != 10*types.MB {
		t.Fatalf("expected Runtime.MinChunkSize=%d (from savedState), got %d", 10*types.MB, cfg.Runtime.MinChunkSize)
	}
}

func TestBuildResumeConfig_NoOverridesUsesDefaults(t *testing.T) {
	settings := config.DefaultSettings()
	entry := &types.DownloadEntry{
		ID:        "test-id",
		URL:       "http://example.com/file.zip",
		DestPath:  "/tmp/file.zip",
		Filename:  "file.zip",
		TotalSize: 1000,
	}
	savedState := &types.DownloadState{
		ID:         "test-id",
		URL:        "http://example.com/file.zip",
		DestPath:   "/tmp/file.zip",
		Filename:   "file.zip",
		TotalSize:  1000,
		Downloaded: 500,
	}

	cfg := buildResumeConfig("test-id", "/tmp", entry, savedState, settings)
	if cfg.Runtime.Workers != 0 {
		t.Fatalf("expected Runtime.Workers=0 (unset), got %d", cfg.Runtime.Workers)
	}
	defaultRuntime := settings.ToRuntimeConfig()
	if cfg.Runtime.MinChunkSize != defaultRuntime.MinChunkSize {
		t.Fatalf("expected Runtime.MinChunkSize=%d (global default), got %d", defaultRuntime.MinChunkSize, cfg.Runtime.MinChunkSize)
	}
}
