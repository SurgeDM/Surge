package core

import (
	"context"
	"testing"

	"github.com/surge-downloader/surge/internal/processing"
)

// startEventWorkerForTest wires up a LifecycleManager event worker to the
// service's event stream. This is required because DB persistence was moved
// from the Engine into the Processing layer. Tests that expect database state
// to appear after pause/complete must call this.
func startEventWorkerForTest(t *testing.T, svc *LocalDownloadService) func() {
	t.Helper()

	mgr := processing.NewLifecycleManager(nil, nil)
	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("startEventWorkerForTest: failed to stream events: %v", err)
	}

	go mgr.StartEventWorker(stream)

	return cleanup
}
