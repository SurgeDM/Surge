package tui

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	downloadservice "github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestResyncUpdatesTerminalStatesAndRemovesDeletedRows(t *testing.T) {
	model := benchModel(2)
	model.SelectedDownloadID = model.downloads[0].ID
	id := model.SelectedDownloadID
	updated, cmd := model.updateEvents(resyncDownloadsMsg{statuses: []types.DownloadStatus{{ID: id, Filename: "completed.bin", Status: "completed", TotalSize: 100, Downloaded: 100}}})
	got := updated.(RootModel)
	if cmd != nil {
		t.Fatal("resync should not start transfers")
	}
	if got.cachedTotalSpeed != 0 {
		t.Fatalf("terminal snapshot retained stale transfer speed: %d", got.cachedTotalSpeed)
	}
	if len(got.downloads) != 1 || got.downloads[0].ID != id || !got.downloads[0].done || got.downloads[0].Downloaded != 100 {
		t.Fatalf("resync did not reconcile: %+v", got.downloads)
	}
}

func TestResyncDoesNotApplySnapshotOlderThanLifecycleTransition(t *testing.T) {
	model := benchModel(1)
	model.lifecycleRevision = 2
	model.resyncing = true
	updated, cmd := model.updateEvents(resyncDownloadsMsg{revision: 1})
	got := updated.(RootModel)
	if cmd == nil || len(got.downloads) != 1 || !got.resyncing {
		t.Fatal("stale snapshot applied instead of requesting a fresh one")
	}
}

func TestReconnectInvalidatesRefreshAlreadyInFlight(t *testing.T) {
	model := benchModel(1)
	model.resyncing = true
	updated, _ := model.updateEvents(types.DownloadEvent{Type: types.EventSystem, ResyncRequired: true, Message: "reconnected"})
	got := updated.(RootModel)
	if got.lifecycleRevision != 1 {
		t.Fatal("new resync requirement did not invalidate the in-flight query")
	}
	// Even a failed response from the earlier connection must trigger a fresh
	// query rather than consuming the reconnect notice and leaving stale rows.
	refreshed, cmd := got.updateEvents(resyncDownloadsMsg{revision: 0})
	if cmd == nil || !refreshed.(RootModel).resyncing {
		t.Fatal("reconnect recovery was lost behind an older refresh")
	}
}

type resyncTestService struct {
	downloadservice.DownloadService
	list func() ([]types.DownloadStatus, error)
}

func (s *resyncTestService) List() ([]types.DownloadStatus, error) { return s.list() }

func TestResyncRetriesTransientFailuresAndReconcilesWithoutAnotherSignal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model := benchModel(1)
		id := model.downloads[0].ID
		calls := 0
		model.Service = &resyncTestService{list: func() ([]types.DownloadStatus, error) {
			calls++
			if calls < 8 {
				return nil, context.DeadlineExceeded
			}
			return []types.DownloadStatus{{ID: id, Status: "completed", Downloaded: 100, TotalSize: 100}}, nil
		}}
		updated, cmd := model.updateEvents(types.DownloadEvent{Type: types.EventSystem, ResyncRequired: true})
		current := updated.(RootModel)
		initialLogs := len(current.logEntries)
		for _, delay := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second} {
			if cmd == nil {
				t.Fatal("missing snapshot request")
			}
			updated, retry := current.updateEvents(cmd())
			current = updated.(RootModel)
			if retry == nil || !current.resyncing || current.resyncRetryDelay != delay {
				t.Fatalf("recovery not pending: retry=%v delay=%s", retry != nil, current.resyncRetryDelay)
			}
			start := time.Now()
			updated, cmd = current.updateEvents(retry())
			current = updated.(RootModel)
			if time.Since(start) != delay {
				t.Fatalf("retry delay=%s want=%s", time.Since(start), delay)
			}
		}
		if len(current.logEntries) != initialLogs+1 {
			t.Fatal("recovery retries flooded the activity log")
		}
		updated, next := current.updateEvents(cmd())
		current = updated.(RootModel)
		if next != nil || current.resyncing || current.resyncRetryDelay != 0 || !current.downloads[0].done || current.cachedTotalSpeed != 0 || calls != 8 {
			t.Fatalf("recovery did not converge: calls=%d pending=%v speed=%d", calls, current.resyncing, current.cachedTotalSpeed)
		}
	})
}

func TestResyncDoesNotRetryPermanentErrorsOrObsoleteTimers(t *testing.T) {
	model := benchModel(1)
	model.resyncing = true
	model.resyncGeneration = 1
	updated, cmd := model.updateEvents(resyncDownloadsMsg{err: errors.New("invalid snapshot")})
	current := updated.(RootModel)
	if cmd != nil || current.resyncing {
		t.Fatal("permanent snapshot error should stop automatic retries")
	}
	// A delayed retry from an earlier recovery must not query a later cycle.
	current.resyncing, current.resyncGeneration = true, 2
	_, cmd = current.updateEvents(resyncRetryMsg{generation: 1})
	if cmd != nil {
		t.Fatal("obsolete retry timer queried the service")
	}
}

func TestResyncRetryIncludesInterveningLifecycleChanges(t *testing.T) {
	model := benchModel(1)
	model.resyncing, model.resyncGeneration = true, 1
	model.lifecycleRevision = 5
	model.Service = &resyncTestService{list: func() ([]types.DownloadStatus, error) { return nil, nil }}
	_, cmd := model.updateEvents(resyncRetryMsg{generation: 1})
	msg := cmd().(resyncDownloadsMsg)
	if msg.revision != 5 {
		t.Fatalf("retry used an old lifecycle revision: %d", msg.revision)
	}
}

func TestResyncRecalculatesActiveTransferSpeed(t *testing.T) {
	model := benchModel(1)
	updated, _ := model.updateEvents(resyncDownloadsMsg{statuses: []types.DownloadStatus{{ID: "active", Status: "downloading", Speed: 1234}}})
	if got := updated.(RootModel).cachedTotalSpeed; got != 1234 {
		t.Fatalf("active speed=%d want=1234", got)
	}
}
