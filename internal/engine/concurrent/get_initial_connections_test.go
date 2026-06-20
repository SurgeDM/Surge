package concurrent

import (
	"testing"

	"github.com/SurgeDM/Surge/internal/engine/types"
)

func TestGetInitialConnections_SqrtHeuristic(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{},
	}
	// 100 MB file → √100 = 10 workers
	fileSize := int64(100 * types.MB)
	got := d.getInitialConnections(fileSize)
	if got != 10 {
		t.Fatalf("Workers=0, 100MB file: got %d, want 10 (√size)", got)
	}
}

func TestGetInitialConnections_WorkersOverrideBypassesSqrt(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{
			Workers: 8,
		},
	}
	// 100 MB file would give √100=10, but Workers=8 should bypass
	fileSize := int64(100 * types.MB)
	got := d.getInitialConnections(fileSize)
	if got != 8 {
		t.Fatalf("Workers=8, 100MB file: got %d, want 8 (bypass √size)", got)
	}
}

func TestGetInitialConnections_WorkersClampedByMaxConnections(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{
			MaxConnectionsPerDownload: 32,
			Workers:                   64,
		},
	}
	fileSize := int64(100 * types.MB)
	got := d.getInitialConnections(fileSize)
	if got != 32 {
		t.Fatalf("Workers=64, MaxConns=32: got %d, want 32 (clamped by ceiling)", got)
	}
}

func TestGetInitialConnections_WorkersClampedByMinChunkSize(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{
			Workers:      16,
			MinChunkSize: 2 * types.MB,
		},
	}
	// 10 MB file / 2 MB min chunk = 5 max chunks
	fileSize := int64(10 * types.MB)
	got := d.getInitialConnections(fileSize)
	if got != 5 {
		t.Fatalf("Workers=16, 10MB file, MinChunk=2MB: got %d, want 5 (clamped by minChunkSize)", got)
	}
}

func TestGetInitialConnections_WorkersMinimumOne(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{
			Workers: 1,
		},
	}
	fileSize := int64(100 * types.MB)
	got := d.getInitialConnections(fileSize)
	if got != 1 {
		t.Fatalf("Workers=1: got %d, want 1", got)
	}
}

func TestGetInitialConnections_ZeroFileSize(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{},
	}
	got := d.getInitialConnections(0)
	if got != 1 {
		t.Fatalf("fileSize=0: got %d, want 1", got)
	}
}
