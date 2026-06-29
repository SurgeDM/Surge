package concurrent

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

func TestConcurrentDownloader_PrewarmConnections(t *testing.T) {
	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	fileSize := int64(1 * utils.MiB)
	destPath := filepath.Join(tmpDir, "prewarm_test.bin")

	var mu sync.Mutex
	prewarmSeen := false
	downloadSeen := false

	// Create mock server to track request order
	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(fileSize),
		testutil.WithRangeSupport(true),
		testutil.WithHandler(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			rng := r.Header.Get("Range")
			if rng == "bytes=0-0" {
				prewarmSeen = true
			} else if rng != "" {
				// Actual download request usually has a real range
				downloadSeen = true
			}
		}),
	)
	defer server.Close()

	// Ensure incomplete file exists
	if f, err := os.Create(destPath + types.IncompleteSuffix); err == nil {
		_ = f.Close()
	}

	state := progress.New("prewarm-test", fileSize)
	runtime := &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
		DialHedgeCount:            2, // Enable hedging
		MinChunkSize:              256 * utils.KiB,
	}

	downloader := NewConcurrentDownloader("prewarm-id", nil, state, runtime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := downloader.Download(ctx, server.URL(), []string{server.URL()}, []string{server.URL()}, destPath, fileSize)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !prewarmSeen {
		t.Error("Expected to see pre-warm request (bytes=0-0), but none were recorded")
	}
	if !downloadSeen {
		t.Error("Expected to see download requests, but none were recorded")
	}
}
