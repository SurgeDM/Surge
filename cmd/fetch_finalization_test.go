package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestFetchClearsReceiptWhenFinalArtifactCheckFails(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	owner, err := newFetchOwner("", 1)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			owner.close(nil)
		}
	}()
	id := owner.session.Jobs[0].ID
	owner.trackJob(id)
	path := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.AddToMasterList(types.DownloadRecord{ID: id, URL: "https://example.com/model", Filename: "model.bin", DestPath: path, Status: "completed", Downloaded: 3, TotalSize: 3}); err != nil {
		t.Fatal(err)
	}
	observed, err := observeDownloads(context.Background(), []string{id}, func(_ context.Context, id string) (*types.DownloadStatus, error) { return owner.service.GetResult(id) }, time.Second, "local", "")
	if err != nil || observed[0].Artifact == nil {
		t.Fatalf("initial receipt: %+v %v", observed, err)
	}
	items := []fetchItem{{waitItem: observed[0]}}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !owner.close(nil) {
		t.Fatal("owner did not stop")
	}
	closed = true
	applyFinalFetchResults(items, map[string]int{id: 0}, owner.finalStatuses, owner.finalErrors)
	if items[0].Artifact != nil || items[0].Error == nil || clioutput.ExitCode(fetchBatchFailure(items)) == 0 {
		t.Fatalf("stale success receipt survived final check: %+v", items)
	}
}
func TestFetchCheckpointConfirmationMatchesSavedDetail(t *testing.T) {
	cases := []struct {
		name                                  string
		savedBytes, masterBytes, masterMillis int64
		want                                  bool
	}{
		{"zero current", 0, 0, 1, true},
		{"zero stale", 0, 0, 2, false},
		{"nonzero current", 42, 42, 1, true},
		{"nonzero stale", 0, 42, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupIsolatedCmdState(t)
			resetCommandConnectionState(t)
			owner, err := newFetchOwner("", 2)
			if err != nil {
				t.Fatal(err)
			}
			closed := false
			defer func() {
				if !closed {
					owner.close(nil)
				}
			}()
			id := owner.session.Jobs[0].ID
			owner.trackJob(id)
			path := filepath.Join(t.TempDir(), "model.bin")
			saved := types.DownloadRecord{ID: id, URL: "https://example.com/model", DestPath: path, Filename: "model.bin", Status: "paused", TotalSize: 100, Downloaded: tc.savedBytes, Elapsed: int64(time.Millisecond)}
			if err := store.SaveStateWithOptions(saved.URL, path, &saved, store.SaveStateOptions{SkipFileHash: true}); err != nil {
				t.Fatal(err)
			}
			record := saved
			record.Downloaded = tc.masterBytes
			record.TimeTaken = tc.masterMillis
			if err := store.AddToMasterList(record); err != nil {
				t.Fatal(err)
			}
			if !owner.close(nil) {
				t.Fatal("owner did not stop")
			}
			closed = true
			if owner.checkpointsValid != tc.want {
				t.Fatalf("checkpoint confirmation=%t, want %t", owner.checkpointsValid, tc.want)
			}
		})
	}
}

func TestFetchReuseMetricsPreserveV1TypesAndExposeUncertainty(t *testing.T) {
	items := []fetchItem{{waitItem: waitItem{ID: "resumed", Artifact: &waitArtifact{SizeBytes: "100"}}}, {waitItem: waitItem{ID: "fresh", Artifact: &waitArtifact{SizeBytes: "100"}}}}
	finishFetchMetrics(items, map[string]int64{"resumed": 25}, time.Second)
	if items[0].ReusedBytes != "25" || items[0].ReusedBytesExact || items[0].AverageBytesPerSecond != nil {
		t.Fatalf("unconfirmed reuse claimed exact: %+v", items[0])
	}
	if items[1].ReusedBytes != "0" || !items[1].ReusedBytesExact || items[1].AverageBytesPerSecond == nil {
		t.Fatalf("fresh metrics unavailable: %+v", items[1])
	}
}

func TestFetchCheckpointConfirmationFailsForLostTrackedRecord(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	owner, err := newFetchOwner("", 2)
	if err != nil {
		t.Fatal(err)
	}
	owner.trackJob(owner.session.Jobs[0].ID)
	if !owner.close(nil) {
		t.Fatal("owner did not stop")
	}
	if owner.checkpointsValid || owner.finalErrors[owner.session.Jobs[0].ID] == nil {
		t.Fatal("missing tracked record was ignored")
	}
	if owner.finalErrors[owner.session.Jobs[1].ID] != nil {
		t.Fatal("unsubmitted job was treated as a checkpoint failure")
	}
}
