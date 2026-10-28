package orchestrator

import (
	"testing"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestObserverInterestAndFiltering(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()
	lifecycle, stopLifecycle := bus.SubscribeObserver(types.ObserverOptions{IDs: []string{"selected"}})
	defer stopLifecycle()
	if progress, chunks := bus.ProgressInterest(); progress || chunks {
		t.Fatal("lifecycle observer enables costly snapshots")
	}
	scalar, stopScalar := bus.SubscribeObserver(types.ObserverOptions{IDs: []string{"selected"}, Progress: true})
	defer stopScalar()
	if progress, chunks := bus.ProgressInterest(); !progress || chunks {
		t.Fatal("scalar observer enables bitmaps")
	}
	if err := bus.Publish(types.DownloadEvent{Type: types.EventQueued, DownloadID: "other"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(types.DownloadEvent{Type: types.EventQueued, DownloadID: "selected"}); err != nil {
		t.Fatal(err)
	}
	if event := <-lifecycle; event.DownloadID != "selected" {
		t.Fatalf("filter: %+v", event)
	}
	if event := <-scalar; event.DownloadID != "selected" {
		t.Fatalf("filter: %+v", event)
	}
	if err := bus.Publish(types.DownloadEvent{Type: types.EventProgress, DownloadID: "selected", ChunkBitmap: []byte{1}, ChunkProgress: []int64{1}}); err != nil {
		t.Fatal(err)
	}
	if event := <-scalar; len(event.ChunkBitmap) != 0 || len(event.ChunkProgress) != 0 {
		t.Fatal("chunk data leaked")
	}
}
