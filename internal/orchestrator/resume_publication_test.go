package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestResumeContinuesWhenEventPublicationFails(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			dir := testutil.SetupStateDB(t)
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "16")
				w.WriteHeader(http.StatusOK)
				<-release
			}))
			defer server.Close()
			testutil.SeedMasterList(t, types.DownloadRecord{ID: "resume", URL: server.URL, DestPath: filepath.Join(dir, "file.bin"), Filename: "file.bin", Status: "paused"})
			bus := NewEventBus()
			pool := scheduler.New(bus.InputCh, 1)
			mgr := NewLifecycleManager(pool, bus, nil)
			defer mgr.Shutdown()
			defer close(release)
			// Cancel publishing without closing InputCh: the scheduler remains usable.
			bus.cancel()
			if batch {
				errs := mgr.ResumeBatch([]string{"resume"})
				if len(errs) != 1 || errs[0] != nil {
					t.Fatalf("resume failed on publication error: %v", errs)
				}
			} else if err := mgr.Resume("resume"); err != nil {
				t.Fatalf("resume failed on publication error: %v", err)
			}
			if pool.GetStatus("resume") == nil {
				t.Fatal("publication failure discarded the resumed download")
			}
		})
	}
}
