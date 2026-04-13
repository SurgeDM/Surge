package single

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

func TestSingleDownloader_GlobalRateLimit(t *testing.T) {
	tmpDir, cleanup, _ := testutil.TempDir("surge-ratelimit-single")
	defer cleanup()

	fileSize := int64(128 * 1024) // 128KB
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(false),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "ratelimit_single.bin")
	state := types.NewProgressState("ratelimit-single", fileSize)
	runtime := &types.RuntimeConfig{}

	downloader := NewSingleDownloader("ratelimit-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Apply global rate limit of 64KB/s.
	utils.GlobalRateLimiter.SetRate(64 * 1024)
	defer utils.GlobalRateLimiter.SetRate(0)

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	start := time.Now()
	err := downloader.Download(ctx, server.URL(), destPath, fileSize, "ratelimit.bin")
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Errorf("Download completed too fast (%v), global rate limit not applied", elapsed)
	}
}

func TestSingleDownloader_PerTaskRateLimit(t *testing.T) {
	tmpDir, cleanup, _ := testutil.TempDir("surge-ratelimit-single-task")
	defer cleanup()

	fileSize := int64(128 * 1024) // 128KB
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(false),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "ratelimit_single_task.bin")
	state := types.NewProgressState("ratelimit-single-task", fileSize)
	// Apply per-task rate limit of 64KB/s
	state.Limiter = utils.NewTokenBucket(64 * 1024)

	runtime := &types.RuntimeConfig{}

	downloader := NewSingleDownloader("ratelimit-task-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	start := time.Now()
	err := downloader.Download(ctx, server.URL(), destPath, fileSize, "ratelimit.bin")
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Errorf("Download completed too fast (%v), per-task rate limit not applied", elapsed)
	}
}
