package cmd

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// StartHeadlessConsumer starts a goroutine to consume progress messages and log to stdout
func StartHeadlessConsumer(ctx context.Context, service service.DownloadService) {
	go func() {
		if service == nil {
			return
		}
		stream, cleanup, err := service.StreamEvents(ctx)
		if err != nil {
			utils.Debug("Failed to start event stream: %v", err)
			return
		}
		defer cleanup()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-stream:
				if !ok {
					return
				}
				switch msg.Type {
				case types.EventStarted:
					fmt.Printf("Started: %s [%s]\n", msg.Filename, truncateID(msg.DownloadID))
				case types.EventComplete:
					atomic.AddInt32(&activeDownloads, -1)
					fmt.Printf("Completed: %s [%s] (in %s)\n", msg.Filename, truncateID(msg.DownloadID), msg.Elapsed)
				case types.EventError:
					atomic.AddInt32(&activeDownloads, -1)
					fmt.Printf("Error: %s [%s]: %v\n", msg.Filename, truncateID(msg.DownloadID), msg.Err)
				case types.EventQueued:
					fmt.Printf("Queued: %s [%s]\n", msg.Filename, truncateID(msg.DownloadID))
				case types.EventPaused:
					fmt.Printf("Paused: %s [%s]\n", msg.Filename, truncateID(msg.DownloadID))
				case types.EventResumed:
					fmt.Printf("Resumed: %s [%s]\n", msg.Filename, truncateID(msg.DownloadID))
				case types.EventRemoved:
					fmt.Printf("Removed: %s [%s]\n", msg.Filename, truncateID(msg.DownloadID))
				case types.EventProgress, types.EventRequest, types.EventBatchRequest, types.EventBatchProgress, types.EventSystem:
					// Streaming/internal events are intentionally not logged to stdout.
				}
			}
		}
	}()
}

// truncateID shortens a UUID to its first 8 characters for display
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
