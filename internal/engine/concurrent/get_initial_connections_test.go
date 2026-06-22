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

func TestGetInitialConnections_LargeFileSizeNoOverflow(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{
			Workers:                   5,
			MaxConnectionsPerDownload: 32,
			MinChunkSize:              1,
		},
	}
	// 10 GB file, 1 byte min chunk → ~1.07e10 chunks, exceeds int32 max (2.1e9)
	// but fits in int64. Workers=5 should still return 5 (not overflowed value).
	got := d.getInitialConnections(int64(10 * 1024 * 1024 * 1024))
	if got != 5 {
		t.Fatalf("expected 5 workers (no overflow), got %d", got)
	}
}

func TestGetInitialConnections_LargeFileSizeSqrtHeuristicNoOverflow(t *testing.T) {
	d := &ConcurrentDownloader{
		Runtime: &types.RuntimeConfig{
			MaxConnectionsPerDownload: 32,
			MinChunkSize:              1,
		},
	}
	// 10 GB file with √size heuristic and 1-byte min chunk.
	// √(10*1024*1024) ≈ 3313, clamped to MaxConns=32.
	// The maxPossibleChunks check should not overflow.
	got := d.getInitialConnections(int64(10 * 1024 * 1024 * 1024))
	if got != 32 {
		t.Fatalf("expected 32 (clamped by MaxConns), got %d", got)
	}
}
