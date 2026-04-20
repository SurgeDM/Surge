package download

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

func setupState(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, fmt.Sprintf("%s-surge.db", t.Name()))
	state.Configure(dbPath)
}

func TestTUIDownload_ConcurrentFails_FallsBackToSingle(t *testing.T) {
	setupState(t)

	// Create a mock server that fails when range requests are made, but succeeds on normal GET
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			t.Logf("Mock server: received range request %q, failing", r.Header.Get("Range"))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		t.Logf("Mock server: received normal request, succeeding")
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "file.txt")
	surgePath := outPath + types.IncompleteSuffix

	// Create empty .surge file expected by processors
	f, err := os.Create(surgePath)
	if err != nil {
		t.Fatalf("failed to create surge file: %v", err)
	}
	_ = f.Close()

	cfg := &types.DownloadConfig{
		ID:            "test-concurrent-fail",
		URL:           server.URL,
		OutputPath:    tmpDir,
		Filename:      "file.txt",
		DestPath:      outPath,
		TotalSize:     5,
		SupportsRange: true, // Force concurrent to start
		Runtime: &types.RuntimeConfig{
			WorkerBufferSize:      1024,
			MinChunkSize:          1,
			MaxConnectionsPerHost: 2,
			MaxTaskRetries:        1,
		},
		State: types.NewProgressState("test-concurrent-fail", 5),
	}

	// Pre-fill some state to verify it gets cleared
	cfg.State.Downloaded.Store(1)
	cfg.State.InitBitmap(5, 1)
	cfg.State.UpdateChunkStatus(0, 1, types.ChunkDownloading)

	err = TUIDownload(context.Background(), cfg)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify state reset happened
	if cfg.State.Downloaded.Load() != 5 {
		t.Errorf("expected final downloaded 5, got %d", cfg.State.Downloaded.Load())
	}
	if cfg.State.ChunkBitmap != nil {
		t.Error("expected ChunkBitmap to be nil after successful single-threaded download")
	}

	// Make sure the single downloader successfully ran and populated the incomplete file
	info, err := os.Stat(surgePath)
	if err != nil {
		t.Fatalf("failed to stat output file: %v", err)
	}
	t.Logf("Final file size: %d", info.Size())

	content, err := os.ReadFile(surgePath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	t.Logf("Final file content: %q", string(content))
	if string(content) != "hello" {
		t.Fatalf("expected 'hello', got %q (len=%d)", string(content), len(content))
	}
}

func TestTUIDownload_ProbeFails_FallsBackToSingleGracefully(t *testing.T) {
	setupState(t)

	// If probe failed, SupportsRange will be false and TotalSize will be 0.
	// We want to ensure TUIDownload gracefully goes straight to single-threaded downloader
	// and still downloads successfully.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("world"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "file.txt")
	surgePath := outPath + types.IncompleteSuffix

	f, err := os.Create(surgePath)
	if err != nil {
		t.Fatalf("failed to create surge file: %v", err)
	}
	_ = f.Close()

	cfg := &types.DownloadConfig{
		ID:            "test-probe-fail",
		URL:           server.URL,
		OutputPath:    tmpDir,
		Filename:      "file.txt",
		DestPath:      outPath,
		TotalSize:     0,     // Simulates failed probe
		SupportsRange: false, // Simulates failed probe
		Runtime:       &types.RuntimeConfig{},
		State:         types.NewProgressState("test-probe-fail", 0),
	}

	err = TUIDownload(context.Background(), cfg)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(surgePath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(content) != "world" {
		t.Fatalf("expected 'world', got %q", string(content))
	}
}

func TestTUIDownload_ContextCanceled_AbortsFallback(t *testing.T) {
	setupState(t)

	// Mock server that hangs on the first request to allow time for cancellation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Single downloader request: block until context canceled
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "file.txt")
	surgePath := outPath + types.IncompleteSuffix

	f, err := os.Create(surgePath)
	if err != nil {
		t.Fatalf("failed to create surge file: %v", err)
	}
	_ = f.Close()

	cfg := &types.DownloadConfig{
		ID:            "test-cancel",
		URL:           server.URL,
		DestPath:      outPath,
		TotalSize:     5,
		SupportsRange: true,
		Runtime: &types.RuntimeConfig{
			MaxTaskRetries: 1,
		},
		State: types.NewProgressState("test-cancel", 5),
	}

	// Cancel before single downloader starts
	cancel()

	err = TUIDownload(ctx, cfg)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
}
