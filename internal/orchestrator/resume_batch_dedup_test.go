package orchestrator

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

// Duplicate IDs inside one ResumeBatch must not double-enqueue: the first
// occurrence resumes normally, repeats report ErrAlreadyActive, and the pool
// ends up with exactly one entry.
func TestResumeBatch_DuplicateIDsGetAlreadyActive(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)

	// Block the response body so the resumed download stays in-flight and the
	// pool-size assertion below stays deterministic.
	unblock := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "16")
		w.WriteHeader(http.StatusOK)
		<-unblock
	}))
	defer ts.Close()

	const id = "dup-resume"
	testutil.SeedMasterList(t, types.DownloadRecord{
		ID:       id,
		URL:      ts.URL,
		DestPath: filepath.Join(tmpDir, "dup.bin"),
		Filename: "dup.bin",
		Status:   "paused",
	})

	pool := scheduler.New(make(chan types.DownloadEvent, 16), 1)
	mgr := NewLifecycleManager(pool, nil, nil)
	defer mgr.Shutdown()
	// Release the handler before shutdown so the worker cannot linger in
	// RunDownload while GracefulShutdown waits on the wait group.
	defer close(unblock)

	errs := mgr.ResumeBatch([]string{id, id})
	if len(errs) != 2 {
		t.Fatalf("len(errs) = %d, want 2", len(errs))
	}
	if errs[0] != nil {
		t.Fatalf("errs[0] = %v, want nil", errs[0])
	}
	if !errors.Is(errs[1], types.ErrAlreadyActive) {
		t.Fatalf("errs[1] = %v, want ErrAlreadyActive", errs[1])
	}

	if all := pool.GetAll(); len(all) != 1 {
		t.Fatalf("pool holds %d entries, want exactly 1", len(all))
	}
}
