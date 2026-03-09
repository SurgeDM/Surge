package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
	"github.com/surge-downloader/surge/internal/testutil"
)

type countingLifecycleService struct {
	streamCalls atomic.Int32
	streamCh    chan interface{}
	cleanupMu   sync.Mutex
	cleaned     bool
	logs        []string
}

var _ core.DownloadService = (*countingLifecycleService)(nil)

func (s *countingLifecycleService) List() ([]types.DownloadStatus, error)   { return nil, nil }
func (s *countingLifecycleService) History() ([]types.DownloadEntry, error) { return nil, nil }
func (s *countingLifecycleService) Add(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
	return "", nil
}
func (s *countingLifecycleService) AddWithID(string, string, string, []string, map[string]string, string, int64, bool) (string, error) {
	return "", nil
}
func (s *countingLifecycleService) Pause(string) error             { return nil }
func (s *countingLifecycleService) Resume(string) error            { return nil }
func (s *countingLifecycleService) ResumeBatch([]string) []error   { return nil }
func (s *countingLifecycleService) UpdateURL(string, string) error { return nil }
func (s *countingLifecycleService) Delete(string) error            { return nil }
func (s *countingLifecycleService) Publish(msg interface{}) error {
	if log, ok := msg.(events.SystemLogMsg); ok {
		s.cleanupMu.Lock()
		s.logs = append(s.logs, log.Message)
		s.cleanupMu.Unlock()
	}
	return nil
}
func (s *countingLifecycleService) GetStatus(string) (*types.DownloadStatus, error) { return nil, nil }
func (s *countingLifecycleService) Shutdown() error                                 { return nil }

func (s *countingLifecycleService) StreamEvents(context.Context) (<-chan interface{}, func(), error) {
	s.streamCalls.Add(1)
	ch := make(chan interface{})
	s.streamCh = ch
	cleanup := func() {
		s.cleanupMu.Lock()
		defer s.cleanupMu.Unlock()
		if s.cleaned {
			return
		}
		close(ch)
		s.cleaned = true
	}
	return ch, cleanup, nil
}

func TestBuildPoolIsNameActive(t *testing.T) {
	getAll := func() []types.DownloadConfig {
		state := types.NewProgressState("dl-2", 0)
		state.SetFilename("from-state.iso")
		state.SetDestPath("/downloads/from-state.iso")

		return []types.DownloadConfig{
			{Filename: "queued.zip", OutputPath: "/downloads"},
			{DestPath: "/downloads/from-path.mp4"},
			{State: state},
		}
	}

	isNameActive := buildPoolIsNameActive(getAll)
	if isNameActive == nil {
		t.Fatal("expected name activity callback")
	}

	for _, name := range []string{"queued.zip", "from-path.mp4", "from-state.iso"} {
		if !isNameActive("/downloads", name) {
			t.Fatalf("expected %q to be active", name)
		}
	}

	if isNameActive("/downloads", "missing.bin") {
		t.Fatal("did not expect unrelated filename to be active")
	}
	if isNameActive("/other", "queued.zip") {
		t.Fatal("did not expect same filename in different directory to conflict")
	}
}

func TestNewLocalLifecycleManager_WiresNameActivityCheck(t *testing.T) {
	getAll := func() []types.DownloadConfig {
		return []types.DownloadConfig{{Filename: "active.bin", OutputPath: "."}}
	}

	mgr := newLocalLifecycleManager(nil, getAll)
	if mgr.IsNameActive == nil {
		t.Fatal("expected IsNameActive to be wired")
	}
	if !mgr.IsNameActive(".", "active.bin") {
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

func TestEnsureLocalLifecycle_ConcurrentInitializationStartsOneStream(t *testing.T) {
	setupIsolatedCmdState(t)
	GlobalLifecycle = nil
	GlobalLifecycleCleanup = nil

	service := &countingLifecycleService{}

	const callers = 12
	results := make(chan *processing.LifecycleManager, callers)
	errs := make(chan error, callers)

	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr, err := ensureLocalLifecycle(service, nil)
			if err != nil {
				errs <- err
				return
			}
			results <- mgr
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("ensureLocalLifecycle returned error: %v", err)
	}

	var first *processing.LifecycleManager
	for mgr := range results {
		if first == nil {
			first = mgr
			continue
		}
		if mgr != first {
			t.Fatal("expected all callers to receive the same lifecycle manager")
		}
	}
	if first == nil {
		t.Fatal("expected lifecycle manager to be created")
	}
	if got := service.streamCalls.Load(); got != 1 {
		t.Fatalf("StreamEvents calls = %d, want 1", got)
	}

	if cleanup := takeLifecycleCleanup(); cleanup != nil {
		cleanup()
	}
	GlobalLifecycle = nil
}

func TestProcessDownloads_UsesSharedEnqueueContext(t *testing.T) {
	setupIsolatedCmdState(t)
	service := &countingLifecycleService{}
	GlobalService = service
	GlobalPool = download.NewWorkerPool(nil, 1)
	GlobalLifecycleCleanup = nil
	t.Cleanup(func() {
		GlobalService = nil
		GlobalPool = nil
		GlobalLifecycle = nil
		if cleanup := takeLifecycleCleanup(); cleanup != nil {
			cleanup()
		}
	})

	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	dispatchCalled := false
	GlobalLifecycle = processing.NewLifecycleManager(
		func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
			dispatchCalled = true
			return "", nil
		},
		nil,
	)

	cancelGlobalEnqueue()
	count := processDownloads([]string{server.URL + "/shared-context.bin"}, t.TempDir(), 0)
	if count != 0 {
		t.Fatalf("count = %d, want 0 after canceled enqueue context", count)
	}
	if dispatchCalled {
		t.Fatal("expected canceled enqueue context to stop before dispatch")
	}
	if len(service.logs) == 0 {
		t.Fatal("expected enqueue failure to be published as a system log")
	}
}

func TestIsExplicitOutputPath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tempDir := t.TempDir()

	tests := []struct {
		name       string
		outPath    string
		defaultDir string
		want       bool
	}{
		{
			name:       "relative and absolute current dir are equal",
			outPath:    ".",
			defaultDir: cwd,
			want:       false,
		},
		{
			name:       "trailing slash is ignored",
			outPath:    tempDir + string(filepath.Separator),
			defaultDir: tempDir,
			want:       false,
		},
		{
			name:       "different directories stay explicit",
			outPath:    filepath.Join(tempDir, "other"),
			defaultDir: tempDir,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExplicitOutputPath(tt.outPath, tt.defaultDir); got != tt.want {
				t.Fatalf("isExplicitOutputPath(%q, %q) = %v, want %v", tt.outPath, tt.defaultDir, got, tt.want)
			}
		})
	}
}
