package scheduler

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestSingleStreamSchedulerAttemptsPreserveThrottleBudget(t *testing.T) {
	var requests atomic.Int32
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "budget.bin")
	if err := os.WriteFile(path+types.IncompleteSuffix, nil, 0600); err != nil {
		t.Fatal(err)
	}
	anchor := time.Now().Add(-10*time.Minute + 3*time.Second)
	cfg := &types.DownloadRecord{
		ID: "single-budget", URL: server.URL, OutputPath: dir, Filename: "budget.bin",
		ThrottleEpisodeStart: anchor,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := RunDownload(ctx, cfg); err == nil || errors.Is(err, types.ErrRetryBudgetExceeded) {
		t.Fatalf("first attempt error=%v, want generic HTTP failure", err)
	}
	if cfg.ThrottleEpisodeStart != anchor {
		t.Fatal("first scheduler attempt reset the throttle budget")
	}
	err := RunDownload(ctx, cfg)
	if !errors.Is(err, types.ErrRetryBudgetExceeded) {
		t.Fatalf("second attempt error=%v, want inherited budget exceeded", err)
	}
	if shouldRetryFailedDownload(false, err, 0) {
		t.Fatal("scheduler would requeue an exhausted single-stream budget")
	}
}
