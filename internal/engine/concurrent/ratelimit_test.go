package concurrent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/utils"
)

func TestConcurrentDownloader_GlobalRateLimit(t *testing.T) {
	tmpDir, cleanup, _ := testutil.TempDir("surge-ratelimit-conc")
	defer cleanup()

	fileSize := int64(128 * 1024) // 128KB
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "ratelimit_conc.bin")
	state := types.NewProgressState("ratelimit-conc", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 2,
		MinChunkSize:          32 * 1024,
	}

	downloader := NewConcurrentDownloader("ratelimit-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Apply rate limit of 64KB/s via state.Limiter to prevent data race across test packages.
	// First 64KB is instant (burst), next 64KB takes ~1s.
	state.Limiter = utils.NewTokenBucket(64 * 1024)

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	start := time.Now()
	err := downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Errorf("Download completed too fast (%v), global rate limit not applied", elapsed)
	}
}

func TestConcurrentDownloader_PerTaskRateLimit(t *testing.T) {
	tmpDir, cleanup, _ := testutil.TempDir("surge-ratelimit-conc-task")
	defer cleanup()

	fileSize := int64(128 * 1024) // 128KB
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "ratelimit_conc_task.bin")
	state := types.NewProgressState("ratelimit-conc-task", fileSize)
	// Apply per-task rate limit of 64KB/s
	state.Limiter = utils.NewTokenBucket(64 * 1024)

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 2,
		MinChunkSize:          32 * 1024,
	}

	downloader := NewConcurrentDownloader("ratelimit-task-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	start := time.Now()
	err := downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Errorf("Download completed too fast (%v), per task rate limit not applied", elapsed)
	}
}
