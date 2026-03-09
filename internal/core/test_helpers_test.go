package core

import (
	"context"
	"sync"
	"testing"

	"github.com/surge-downloader/surge/internal/processing"
)

// startEventWorkerForTest wires up a LifecycleManager event worker to the
// service's event stream. This is required because DB persistence was moved
// from the Engine into the Processing layer. Tests that expect database state
// to appear after pause/complete must call this.
//
// Returns a wait function. Call it after svc.Shutdown() to block until the
// event worker has drained all buffered events and finished DB writes.
func startEventWorkerForTest(t *testing.T, svc *LocalDownloadService) func() {
	t.Helper()

	mgr := processing.NewLifecycleManager(nil, nil)
	stream, cleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("startEventWorkerForTest: failed to stream events: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mgr.StartEventWorker(stream)
	}()

	return func() {
		cleanup() // closes the channel, causing StartEventWorker to exit
		wg.Wait() // wait for all DB writes to complete
	}
}
