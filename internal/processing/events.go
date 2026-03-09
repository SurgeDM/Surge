package processing

import (
	"os"
	"time"

	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
)

// advanceRemainingTasks consumes bytes from the front of remaining tasks.
func advanceRemainingTasks(tasks []types.Task, consumed int64) []types.Task {
	if consumed <= 0 || len(tasks) == 0 {
		return tasks
	}

	out := make([]types.Task, 0, len(tasks))
	left := consumed
	for _, task := range tasks {
		if left <= 0 {
			out = append(out, task)
			continue
		}
		if task.Length <= left {
			left -= task.Length
			continue
		}
		task.Offset += left
		task.Length -= left
		left = 0
		out = append(out, task)
	}
	return out
}

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
			// Update master list with paused status and progress
			if m.State == nil {
				existing, _ := state.GetDownload(m.DownloadID)
				if existing == nil {
					if err := state.UpdateStatus(m.DownloadID, "paused"); err != nil {
						utils.Debug("Lifecycle: Failed to update pause status: %v", err)
					}
					break
				}

				entry := *existing
				entry.Status = "paused"
				if m.Downloaded > 0 {
					entry.Downloaded = m.Downloaded
				}
				if err := state.AddToMasterList(entry); err != nil {
					utils.Debug("Lifecycle: Failed to persist paused fallback entry: %v", err)
				}

				if existing.URL != "" && existing.DestPath != "" {
					saved, err := state.LoadState(existing.URL, existing.DestPath)
					if err == nil && saved != nil {
						prevDownloaded := saved.Downloaded
						prevElapsed := saved.Elapsed

						if m.Downloaded > saved.Downloaded {
							delta := m.Downloaded - saved.Downloaded
							saved.Tasks = advanceRemainingTasks(saved.Tasks, delta)
							saved.Downloaded = m.Downloaded
						}
						if existing.TimeTaken > 0 {
							candidateElapsed := existing.TimeTaken * int64(time.Millisecond)
							if candidateElapsed > saved.Elapsed {
								saved.Elapsed = candidateElapsed
							}
						}
						if saved.Downloaded > prevDownloaded && saved.Elapsed <= prevElapsed {
							saved.Elapsed = prevElapsed + int64(time.Millisecond)
						}

						if err := state.SaveStateWithOptions(existing.URL, existing.DestPath, saved, state.SaveStateOptions{SkipFileHash: true}); err != nil {
							utils.Debug("Lifecycle: Failed to persist paused fallback state: %v", err)
						}
					}
				}
				break
			}

			// Resolve destPath: prefer state, fallback to DB entry
			destPath := m.State.DestPath
			url := m.State.URL

			existing, _ := state.GetDownload(m.DownloadID)
			if existing != nil {
				if destPath == "" {
					destPath = existing.DestPath
				}
				if url == "" {
					url = existing.URL
				}
			}

			// Full upsert with downloaded progress
			entry := types.DownloadEntry{
				ID:         m.DownloadID,
				Status:     "paused",
				Downloaded: m.State.Downloaded,
				DestPath:   destPath,
				Filename:   m.Filename,
				TotalSize:  m.State.TotalSize,
				TimeTaken:  m.State.Elapsed / int64(time.Millisecond),
			}
			if existing != nil {
				entry.URL = existing.URL
				entry.URLHash = existing.URLHash
			}
			if err := state.AddToMasterList(entry); err != nil {
				utils.Debug("Lifecycle: Failed to persist paused state: %v", err)
			}

			// Save detailed pause state for resuming later.
			// Skip if destPath is empty — SaveState with a bare filename
			// corrupts the state DB key and breaks resume.
			if destPath != "" && url != "" {
				// Keep pause persistence fast so lifecycle events don't back up and get dropped.
				if err := state.SaveStateWithOptions(url, destPath, m.State, state.SaveStateOptions{
					SkipFileHash: true,
				}); err != nil {
					utils.Debug("Lifecycle: Failed to save pause state: %v", err)
				}
			} else {
				utils.Debug("Lifecycle: Skipping SaveState for %s: destPath=%q url=%q", m.DownloadID, destPath, url)
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
			filename := m.Filename
			if existing != nil {
				destPath = existing.DestPath
				url = existing.URL
				urlHash = existing.URLHash
				if filename == "" {
					filename = existing.Filename
				}
			}

			// Rename from .surge to final destination
			if destPath != "" {
				finalPath := destPath
				surgePath := finalPath + types.IncompleteSuffix
				if err := os.Rename(surgePath, finalPath); err != nil {
					// Might have already been renamed, or cross-device link error
					if _, statErr := os.Stat(finalPath); statErr != nil {
						utils.Debug("Lifecycle: Failed to rename completed file from %s to %s: %v", surgePath, finalPath, err)
					}
				}
			}

			if err := state.AddToMasterList(types.DownloadEntry{
				ID:          m.DownloadID,
				URL:         url,
				URLHash:     urlHash,
				DestPath:    destPath,
				Filename:    filename,
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
				if existing.DestPath != "" {
					if err := RemoveIncompleteFile(existing.DestPath); err != nil {
						utils.Debug("Lifecycle: Failed to remove incomplete file after error: %v", err)
					}
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

			// Delete the incomplete .surge file if present
			if m.DestPath != "" && !m.Completed {
				if err := RemoveIncompleteFile(m.DestPath); err != nil {
					utils.Debug("Lifecycle: Failed to remove incomplete file: %v", err)
				}
			}

		case events.DownloadQueuedMsg:
			// Persist queued download so it survives shutdown
			if err := state.AddToMasterList(types.DownloadEntry{
				ID:       m.DownloadID,
				URL:      m.URL,
				URLHash:  state.URLHash(m.URL),
				DestPath: m.DestPath,
				Filename: m.Filename,
				Status:   "queued",
			}); err != nil {
				utils.Debug("Lifecycle: Failed to persist queued download: %v", err)
			}

		case events.BatchProgressMsg:
			// No-op for persistence, handled by TUI

		case events.ProgressMsg:
			// No-op for persistence
		}
	}
}
