package cmd

import (
	"context"

	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/types"
)

// Observe scalar events while later inputs are probing. This path never reads
// files, queries every job, formats frames, or writes to the terminal.
func relayFetchProgress(ctx context.Context, owner service.ObserverService, ids []string, renderer *fetchProgress) func() {
	ctx, cancel := context.WithCancel(ctx)
	stream, cleanup, err := owner.StreamObserver(ctx, types.ObserverOptions{IDs: ids, Progress: true})
	if err != nil {
		cancel()
		return func() {}
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-stream:
				if !ok {
					return
				}
				batch := []types.DownloadEvent{event}
				if event.Type == types.EventBatchProgress {
					batch = event.BatchEvents
				}
				for _, item := range batch {
					state := ""
					switch item.Type {
					case types.EventStarted, types.EventProgress:
						state = "downloading"
					case types.EventQueued:
						state = "queued"
					case types.EventComplete:
						state = "finalizing"
					case types.EventPaused:
						state = "paused"
					case types.EventError:
						state = "error"
					case types.EventRemoved:
						state = "cancelled"
					}
					if state != "" {
						renderer.update(&types.DownloadStatus{ID: item.DownloadID, Filename: item.Filename, Status: state, Phase: item.Phase, Downloaded: item.Downloaded, TotalSize: item.Total, Speed: item.Speed, Connections: item.Connections})
					}
				}
			}
		}
	}()
	return func() { cancel(); cleanup() }
}
