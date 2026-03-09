package processing

import (
	"path/filepath"
	"time"

	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
)

// StartEventWorker listens to engine events and handles database persistence
// and file cleanup, ensuring the core engine remains stateless.
func (mgr *LifecycleManager) StartEventWorker(ch <-chan interface{}) {
	for msg := range ch {
		switch m := msg.(type) {

		case events.DownloadStartedMsg:
			// Persist the active download state to trigger initial UI rendering via DB loader
			if err := state.AddToMasterList(types.DownloadEntry{
				ID:         m.DownloadID,
				URL:        m.URL,
				URLHash:    state.URLHash(m.URL),
				DestPath:   m.DestPath,
				Filename:   m.Filename,
				Status:     "downloading",
				TotalSize:  m.Total,
				Downloaded: 0,
			}); err != nil {
				utils.Debug("Lifecycle: Failed to save initial download state: %v", err)
			}

		case events.DownloadPausedMsg:
			// Save the detailed pause state for resuming later
			if m.State != nil {
				// Re-derive destPath if missing
				destPath := m.State.DestPath
				if destPath == "" {
					destPath = filepath.Join(m.State.DestPath, m.Filename)
				}
				if err := state.SaveState(m.State.URL, destPath, m.State); err != nil {
					utils.Debug("Lifecycle: Failed to save pause state: %v", err)
				}
			}

		case events.DownloadCompleteMsg:
			// Calculate avg speed if possible (we don't have exact elapsed in processing layer unless msg has it)
			var avgSpeed float64
			if m.Elapsed.Seconds() > 0 {
				avgSpeed = float64(m.Total) / m.Elapsed.Seconds()
			}

			// Add/Update to master list as completed
			destPath := "" // Best guess, engine should ideally provide it or we look it up
			// We can look it up from the DB quickly
			existing, _ := state.GetDownload(m.DownloadID)
			var url, urlHash string
			if existing != nil {
				destPath = existing.DestPath
				url = existing.URL
				urlHash = existing.URLHash
			}

			if err := state.AddToMasterList(types.DownloadEntry{
				ID:          m.DownloadID,
				URL:         url,
				URLHash:     urlHash,
				DestPath:    destPath,
				Filename:    m.Filename,
				Status:      "completed",
				TotalSize:   m.Total,
				Downloaded:  m.Total,
				CompletedAt: time.Now().Unix(),
				TimeTaken:   m.Elapsed.Milliseconds(),
				AvgSpeed:    avgSpeed,
			}); err != nil {
				utils.Debug("Lifecycle: Failed to persist completed download: %v", err)
			}

		case events.DownloadErrorMsg:
			// Update master list as errored
			existing, _ := state.GetDownload(m.DownloadID)
			if existing != nil {
				existing.Status = "error"
				if err := state.AddToMasterList(*existing); err != nil {
					utils.Debug("Lifecycle: Failed to persist error state: %v", err)
				}
			}

		case events.DownloadRemovedMsg:
			// Delete the state from SQLite
			if err := state.DeleteState(m.DownloadID); err != nil {
				utils.Debug("Lifecycle: Failed to delete state: %v", err)
			}
			if err := state.RemoveFromMasterList(m.DownloadID); err != nil {
				utils.Debug("Lifecycle: Failed to remove from master list: %v", err)
			}

			// NOTE: File deletion for .surge and final files upon UI demand
			// will be handled explicitly via a LifecycleManager.Remove function later.
			// This event just means the engine discarded it.

		case events.BatchProgressMsg:
			// No-op for persistence, handled by TUI

		case events.ProgressMsg:
			// No-op for persistence
		}
	}
}
