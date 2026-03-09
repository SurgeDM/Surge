package cmd

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/testutil"
)

func TestBuildPoolIsNameActive(t *testing.T) {
	getAll := func() []types.DownloadConfig {
		state := types.NewProgressState("dl-2", 0)
		state.SetFilename("from-state.iso")

		return []types.DownloadConfig{
			{Filename: "queued.zip"},
			{DestPath: "/downloads/from-path.mp4"},
			{State: state},
		}
	}

	isNameActive := buildPoolIsNameActive(getAll)
	if isNameActive == nil {
		t.Fatal("expected name activity callback")
	}

	for _, name := range []string{"queued.zip", "from-path.mp4", "from-state.iso"} {
		if !isNameActive(name) {
			t.Fatalf("expected %q to be active", name)
		}
	}

	if isNameActive("missing.bin") {
		t.Fatal("did not expect unrelated filename to be active")
	}
}

func TestNewLocalLifecycleManager_WiresNameActivityCheck(t *testing.T) {
	getAll := func() []types.DownloadConfig {
		return []types.DownloadConfig{{Filename: "active.bin"}}
	}

	mgr := newLocalLifecycleManager(nil, getAll)
	if mgr.IsNameActive == nil {
		t.Fatal("expected IsNameActive to be wired")
	}
	if !mgr.IsNameActive("active.bin") {
		t.Fatal("expected wired IsNameActive callback to inspect active downloads")
	}
}

func TestEnsureLocalLifecycle_StartsEventWorker(t *testing.T) {
	setupIsolatedCmdState(t)
	GlobalLifecycle = nil
	GlobalLifecycleCleanup = nil
	GlobalProgressCh = make(chan any, 32)
	GlobalPool = download.NewWorkerPool(GlobalProgressCh, 1)
	GlobalService = core.NewLocalDownloadServiceWithInput(GlobalPool, GlobalProgressCh)
	t.Cleanup(func() {
		if GlobalLifecycleCleanup != nil {
			GlobalLifecycleCleanup()
			GlobalLifecycleCleanup = nil
		}
		if GlobalService != nil {
			_ = GlobalService.Shutdown()
			GlobalService = nil
		}
		GlobalLifecycle = nil
		GlobalPool = nil
		GlobalProgressCh = nil
	})

	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	outDir := t.TempDir()
	count := processDownloads([]string{server.URL + "/local.bin"}, outDir, 0)
	if count != 1 {
		t.Fatalf("expected 1 successful local add, got %d", count)
	}
	if GlobalLifecycle == nil {
		t.Fatal("expected fallback lifecycle manager to be created")
	}
	if GlobalLifecycleCleanup == nil {
		t.Fatal("expected fallback lifecycle manager to start an event worker")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := state.ListAllDownloads()
		if err == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.DestPath, fmt.Sprintf("%clocal.bin", filepath.Separator)) {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	entries, err := state.ListAllDownloads()
	if err != nil {
		t.Fatalf("failed to list downloads: %v", err)
	}
	t.Fatalf("expected persisted download entry, got %+v", entries)
}
