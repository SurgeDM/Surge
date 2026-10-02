package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/probe"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestProbe206ThenTransfer200RestartsSafely(t *testing.T) {
	for _, mode := range []string{"fresh", "partial-progress", "resume"} {
		t.Run(mode, func(t *testing.T) {
			testutil.SetupStateDB(t)
			dir := t.TempDir()
			payload := bytes.Repeat([]byte("range-transition-content\n"), 32768)
			size := int64(len(payload))
			state := progress.New("transition-"+mode, size)
			var fullGets, progressBeforeFallback atomic.Int64
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rng := r.Header.Get("Range")
				if rng == "bytes=0-0" {
					w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", size))
					w.WriteHeader(http.StatusPartialContent)
					_, _ = w.Write(payload[:1])
					return
				}
				if mode == "partial-progress" && rng != "" {
					var start, end int64
					_, _ = fmt.Sscanf(rng, "bytes=%d-%d", &start, &end)
					if start == 0 {
						w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
						w.WriteHeader(http.StatusPartialContent)
						_, _ = w.Write(payload[start : end+1])
						return
					}
					for state.Bytes.Downloaded.Load() == 0 {
						select {
						case <-r.Context().Done():
							return
						case <-time.After(time.Millisecond):
						}
					}
					progressBeforeFallback.Store(state.Bytes.Downloaded.Load())
				}
				if rng == "" {
					fullGets.Add(1)
				}
				// Vendor headers must not change ordinary HTTP fallback behavior.
				w.Header().Set("Server", "cloudflare")
				w.Header().Set("Cf-Mitigated", "challenge")
				w.Header().Set("Content-Length", fmt.Sprint(size))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(payload)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			metadata, err := probe.ProbeServer(ctx, server.URL+"/file.bin", "", nil)
			if err != nil || !metadata.SupportsRange || metadata.FileSize != size {
				t.Fatalf("probe metadata=%+v err=%v", metadata, err)
			}
			path := filepath.Join(dir, "file.bin")
			cfg := types.DownloadRecord{
				ID: "transition-" + mode, URL: server.URL + "/file.bin", Filename: "file.bin",
				OutputPath: dir, DestPath: path, TotalSize: size, SupportsRange: metadata.SupportsRange,
				ProgressState: state, Runtime: &types.RuntimeConfig{Workers: 2, MaxConnectionsPerDownload: 2, MinChunkSize: 32768},
			}
			initial := []byte(nil)
			if mode == "resume" {
				initial = append(bytes.Clone(payload[:size/2]), bytes.Repeat([]byte("x"), int(size/2))...)
				cfg.IsResume = true
				cfg.Downloaded = size / 2
				cfg.Tasks = []types.Task{{Offset: size / 2, Length: size / 2}}
			}
			if err := os.WriteFile(path+types.IncompleteSuffix, initial, 0600); err != nil {
				t.Fatal(err)
			}
			cfg.Status = "queued"
			if err := store.AddToMasterList(cfg); err != nil {
				t.Fatal(err)
			}
			if mode == "resume" {
				if err := store.SaveStateWithOptions(cfg.URL, path, &cfg, store.SaveStateOptions{SkipFileHash: true}); err != nil {
					t.Fatal(err)
				}
			}
			if err := RunDownload(ctx, &cfg); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path + types.IncompleteSuffix)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("fallback file mismatch: %v", err)
			}
			if cfg.SupportsRange || fullGets.Load() != 1 || state.Bytes.Downloaded.Load() != size {
				t.Fatalf("range=%v full GETs=%d downloaded=%d", cfg.SupportsRange, fullGets.Load(), state.Bytes.Downloaded.Load())
			}
			if mode == "partial-progress" && progressBeforeFallback.Load() == 0 {
				t.Fatal("test did not exercise fallback after partial progress")
			}
			if mode != "fresh" {
				checkpoint, err := store.LoadState(cfg.URL, path)
				if err != nil || checkpoint == nil || len(checkpoint.Tasks) != 0 || len(checkpoint.ChunkBitmap) != 0 || checkpoint.Downloaded != 0 {
					t.Fatalf("stale range checkpoint survived fallback: %+v err=%v", checkpoint, err)
				}
			}
			if cfg.IsResume || len(cfg.Tasks) != 0 || state.TakePendingResumeState() != nil {
				t.Fatal("in-memory range checkpoint survived fallback")
			}
		})
	}
}
