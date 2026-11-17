package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestResumeBatchRespectsMaxConcurrentDownloads(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)

	var activeRequests atomic.Int64
	var peakRequests atomic.Int64
	handler := func(started chan<- struct{}, release <-chan struct{}) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			active := activeRequests.Add(1)
			for peak := peakRequests.Load(); active > peak && !peakRequests.CompareAndSwap(peak, active); peak = peakRequests.Load() {
			}
			defer activeRequests.Add(-1)

			started <- struct{}{}
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
			<-release
			_, _ = w.Write([]byte("x"))
		}
	}

	activeStarted := make(chan struct{}, 1)
	resumeAStarted := make(chan struct{}, 1)
	resumeBStarted := make(chan struct{}, 1)
	activeRelease := make(chan struct{})
	resumeARelease := make(chan struct{})
	resumeBRelease := make(chan struct{})

	activeServer := httptest.NewServer(handler(activeStarted, activeRelease))
	defer activeServer.Close()
	resumeAServer := httptest.NewServer(handler(resumeAStarted, resumeARelease))
	defer resumeAServer.Close()
	resumeBServer := httptest.NewServer(handler(resumeBStarted, resumeBRelease))
	defer resumeBServer.Close()

	releaseActive := sync.OnceFunc(func() { close(activeRelease) })
	releaseResumeA := sync.OnceFunc(func() { close(resumeARelease) })
	releaseResumeB := sync.OnceFunc(func() { close(resumeBRelease) })

	pool := scheduler.New(make(chan types.DownloadEvent, 64), 1)
	mgr := NewLifecycleManager(pool, nil, nil)
	defer mgr.Shutdown()
	defer func() {
		releaseActive()
		releaseResumeA()
		releaseResumeB()
	}()

	activeDest := filepath.Join(tmpDir, "active.bin")
	if err := os.WriteFile(activeDest+types.IncompleteSuffix, nil, 0o644); err != nil {
		t.Fatalf("failed to pre-create active working file: %v", err)
	}
	if !pool.Add(types.DownloadRecord{
		ID:            "active",
		URL:           activeServer.URL,
		OutputPath:    tmpDir,
		Filename:      "active.bin",
		ProgressState: progress.New("active", 1),
		Runtime:       &types.RuntimeConfig{},
	}) {
		t.Fatal("active download was refused")
	}
	select {
	case <-activeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("active download did not start")
	}

	for _, tc := range []struct {
		id       string
		filename string
		url      string
	}{
		{id: "resume-a", filename: "resume-a.bin", url: resumeAServer.URL},
		{id: "resume-b", filename: "resume-b.bin", url: resumeBServer.URL},
	} {
		destPath := filepath.Join(tmpDir, tc.filename)
		if err := os.WriteFile(destPath+types.IncompleteSuffix, nil, 0o644); err != nil {
			t.Fatalf("failed to pre-create resume working file: %v", err)
		}
		testutil.SeedMasterList(t, types.DownloadRecord{
			ID:       tc.id,
			URL:      tc.url,
			DestPath: destPath,
			Filename: tc.filename,
			Status:   "paused",
		})
	}

	errs := mgr.ResumeBatch([]string{"resume-a", "resume-b"})
	if len(errs) != 2 || errs[0] != nil || errs[1] != nil {
		t.Fatalf("ResumeBatch errors = %v, want two successful admissions", errs)
	}
	for _, id := range []string{"resume-a", "resume-b"} {
		status := pool.GetStatus(id)
		if status == nil || status.Status != "queued" {
			t.Fatalf("%s status = %+v, want queued", id, status)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if got := peakRequests.Load(); got != 1 {
		t.Fatalf("peak active requests = %d, want 1 while worker is occupied", got)
	}
	select {
	case <-resumeAStarted:
		t.Fatal("first resumed download bypassed the occupied worker")
	case <-resumeBStarted:
		t.Fatal("second resumed download bypassed the occupied worker")
	default:
	}

	releaseActive()
	select {
	case <-resumeAStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first resumed download did not start after the worker became available")
	}
	if got := peakRequests.Load(); got != 1 {
		t.Fatalf("peak active requests = %d, want 1 after first resume started", got)
	}
	if status := pool.GetStatus("resume-b"); status == nil || status.Status != "queued" {
		t.Fatalf("resume-b status = %+v, want queued", status)
	}

	releaseResumeA()
	select {
	case <-resumeBStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("second resumed download did not start after the first resumed download finished")
	}
	releaseResumeB()
}
