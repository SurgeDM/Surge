package single

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
	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestThrottleBudgetPreservesSharedCooldown(t *testing.T) {
	var requests atomic.Int32
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "cooldown.bin")
	for i := 0; i < 2; i++ {
		d := NewSingleDownloader("cooldown", nil, nil, nil)
		if err := d.Download(ctx, server.URL, path, 0, "cooldown.bin"); !errors.Is(err, types.ErrRetryBudgetExceeded) {
			t.Fatalf("attempt %d: error=%v, want budget exceeded", i, err)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d, second download bypassed cooldown", requests.Load())
	}
	if until := transport.DefaultHostRateLimiter.BlockedUntil(transport.MirrorHost(server.URL), time.Now()); until.Before(started.Add(time.Hour)) {
		t.Fatalf("host deadline=%v, want at least one hour", until)
	}
}

func TestThrottleBudgetSurvivesDownloaderRecreation(t *testing.T) {
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	cfg := &types.DownloadRecord{ThrottleEpisodeStart: time.Now().Add(-11 * time.Minute)}
	first := NewSingleDownloader("budget", nil, nil, nil)
	first.ImportThrottleState(cfg)
	first.ExportThrottleState(cfg)
	second := NewSingleDownloader("budget", nil, nil, nil)
	second.ImportThrottleState(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := second.Download(ctx, server.URL, filepath.Join(t.TempDir(), "budget.bin"), 0, "budget.bin"); !errors.Is(err, types.ErrRetryBudgetExceeded) {
		t.Fatalf("error=%v, want inherited budget exhausted", err)
	}
}

func TestByteProgressResetsThrottleBudget(t *testing.T) {
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("file bytes"))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "progress.bin")
	if err := os.WriteFile(path+types.IncompleteSuffix, nil, 0600); err != nil {
		t.Fatal(err)
	}
	d := NewSingleDownloader("progress", nil, nil, nil)
	cfg := &types.DownloadRecord{ThrottleEpisodeStart: time.Now().Add(-11 * time.Minute)}
	d.ImportThrottleState(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Download(ctx, server.URL, path, 0, "progress.bin"); err != nil {
		t.Fatal(err)
	}
	d.ExportThrottleState(cfg)
	if !cfg.ThrottleEpisodeStart.IsZero() {
		t.Fatal("byte progress did not reset throttle episode")
	}
}
