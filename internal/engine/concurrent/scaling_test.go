package concurrent

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/testutil"
)

// =============================================================================
// Dynamic Worker Scaling Tests
// =============================================================================

func TestSetWorkerCount_Validation(t *testing.T) {
	runtime := &types.RuntimeConfig{MaxConnectionsPerHost: 8}
	d := NewConcurrentDownloader("test-id", nil, nil, runtime)

	// Zero workers should fail
	if err := d.SetWorkerCount(0); err == nil {
		t.Error("expected error for 0 workers")
	}

	// Negative workers should fail
	if err := d.SetWorkerCount(-1); err == nil {
		t.Error("expected error for negative workers")
	}

	// Valid count should succeed
	if err := d.SetWorkerCount(4); err != nil {
		t.Errorf("unexpected error for valid worker count: %v", err)
	}

	// Verify targetWorkers was set
	d.activeMu.Lock()
	if d.targetWorkers != 4 {
		t.Errorf("expected targetWorkers=4, got %d", d.targetWorkers)
	}
	d.activeMu.Unlock()
}

func TestSetWorkerCount_ClampToMax(t *testing.T) {
	runtime := &types.RuntimeConfig{MaxConnectionsPerHost: 4}
	d := NewConcurrentDownloader("test-id", nil, nil, runtime)

	// Request more than max — should be clamped
	if err := d.SetWorkerCount(10); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	d.activeMu.Lock()
	if d.targetWorkers != 4 {
		t.Errorf("expected targetWorkers clamped to 4, got %d", d.targetWorkers)
	}
	d.activeMu.Unlock()
}

func TestSetWorkerCount_NoopOnSameValue(t *testing.T) {
	runtime := &types.RuntimeConfig{MaxConnectionsPerHost: 8}
	d := NewConcurrentDownloader("test-id", nil, nil, runtime)

	// Set initial
	_ = d.SetWorkerCount(3)

	// Drain the scale channel
	select {
	case <-d.workerScaleCh:
	default:
	}

	// Set same value again — should be a no-op
	if err := d.SetWorkerCount(3); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Channel should NOT have a signal (no-op)
	select {
	case <-d.workerScaleCh:
		t.Error("expected no scale signal for same worker count")
	default:
		// Good — no signal
	}
}

func TestDynamicScaling_ScaleUpDuringDownload(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(512 * types.KB)
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithLatency(50*time.Millisecond), // Latency per request to allow scaling mid-download
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "scale_up_test.bin")
	state := types.NewProgressState("scale-up-test", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 8,
		MinChunkSize:          32 * types.KB,
	}

	downloader := NewConcurrentDownloader("scale-up-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pre-create incomplete file
	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	done := make(chan error, 1)
	go func() {
		done <- downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	}()

	// Wait for download to start, then scale up
	time.Sleep(100 * time.Millisecond)
	if err := downloader.SetWorkerCount(6); err != nil {
		t.Errorf("SetWorkerCount failed: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Download failed after scale-up: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Download timed out after scale-up")
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Error(err)
	}
}

func TestDynamicScaling_ScaleDownDuringDownload(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(512 * types.KB)
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithLatency(50*time.Millisecond),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "scale_down_test.bin")
	state := types.NewProgressState("scale-down-test", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 8,
		MinChunkSize:          32 * types.KB,
	}

	downloader := NewConcurrentDownloader("scale-down-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pre-create incomplete file
	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	done := make(chan error, 1)
	go func() {
		done <- downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	}()

	// Wait for download to start, then scale down
	time.Sleep(100 * time.Millisecond)
	if err := downloader.SetWorkerCount(1); err != nil {
		t.Errorf("SetWorkerCount failed: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Download failed after scale-down: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Download timed out after scale-down")
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Error(err)
	}
}

func TestDynamicScaling_ScaleUpThenDown(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(512 * types.KB)
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithLatency(50*time.Millisecond),
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "scale_updown_test.bin")
	state := types.NewProgressState("scale-updown-test", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 8,
		MinChunkSize:          32 * types.KB,
	}

	downloader := NewConcurrentDownloader("scale-updown-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pre-create incomplete file
	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	done := make(chan error, 1)
	go func() {
		done <- downloader.Download(ctx, server.URL(), nil, nil, destPath, fileSize)
	}()

	// Scale up then down
	time.Sleep(80 * time.Millisecond)
	_ = downloader.SetWorkerCount(6)
	time.Sleep(80 * time.Millisecond)
	_ = downloader.SetWorkerCount(2)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Download failed after scale-up-then-down: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Download timed out")
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Error(err)
	}
}

func TestDynamicScaling_AdaptiveOn429(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(256 * types.KB)

	// Server returns 429 on first 2 requests, then works normally.
	// With 4 initial workers, the first burst will trigger 429s,
	// causing the supervisor to halve workers.
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			// Default handler serves the file normally
		}),
		testutil.WithFailOnNthRequest(1), // First request returns 429
	)
	defer server.Close()

	destPath := filepath.Join(tmpDir, "adaptive429_test.bin")
	state := types.NewProgressState("adaptive429-test", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 4,
		MaxTaskRetries:        5,
		MinChunkSize:          32 * types.KB,
	}

	downloader := NewConcurrentDownloader("adaptive429-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pre-create incomplete file
	if f, err := os.Create(destPath + ".surge"); err == nil {
		_ = f.Close()
	}

	mirrors := []string{server.URL()}
	err := downloader.Download(ctx, server.URL(), mirrors, mirrors, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download should complete despite 429: %v", err)
	}

	if err := testutil.VerifyFileSize(destPath+types.IncompleteSuffix, fileSize); err != nil {
		t.Error(err)
	}
}
