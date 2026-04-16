package concurrent

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/testutil"
)

func TestConcurrentDownloader_PrewarmConnections(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1 * types.MB)
	destPath := filepath.Join(tmpDir, "prewarm_test.bin")

	var mu sync.Mutex
	prewarmSeen := false

	// Create dummy data to serve
	dummyData := make([]byte, fileSize)

	// Create mock server to track request order
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			rng := r.Header.Get("Range")
			if rng == "bytes=0-0" {
				prewarmSeen = true
			}
			mu.Unlock()

			// Default response for range requests
			if rng != "" {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusPartialContent)
				// We don't strictly need to serve the whole chunk for this test to pass pre-warm check,
				// but we must not hang the client.
				_, _ = w.Write([]byte{0})
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(dummyData)
			}
		}),
	)
	defer server.Close()

	// Ensure incomplete file exists
	if f, err := os.Create(destPath + types.IncompleteSuffix); err == nil {
		_ = f.Close()
	}

	progressState := types.NewProgressState("prewarm-test", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 2,
		DialHedgeCount:        2, // Enable hedging
		MinChunkSize:          256 * types.KB,
	}

	downloader := NewConcurrentDownloader("prewarm-id", nil, progressState, runtime)

	// Use a more generous timeout for CI
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run download - it might fail due to dummy data, but we only care about pre-warm observation
	_ = downloader.Download(ctx, server.URL(), []string{server.URL()}, []string{server.URL()}, destPath, fileSize)

	mu.Lock()
	defer mu.Unlock()

	if !prewarmSeen {
		t.Error("Expected to see pre-warm request (bytes=0-0), but none were recorded")
	}
}
