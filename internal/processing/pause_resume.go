package processing

import (
	"fmt"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

// EngineHooks defines the minimal callbacks Processing needs to orchestrate the worker pool.
// All management decisions (event emission, DB persistence, state loading) live here in
// LifecycleManager; the pool itself is a pure executor.
type EngineHooks struct {
	// Pause signals the pool to mechanically pause a download (cancel context, set state).
	Pause func(id string) bool
	// ExtractPausedConfig atomically removes a paused download from the pool and returns
	// its config so LifecycleManager can re-enqueue it after hydration from saved state.
	// Returns nil when not found / not paused / still transitioning.
	ExtractPausedConfig func(id string) *types.DownloadConfig
	// GetStatus returns the in-memory status for a download.
	GetStatus func(id string) *types.DownloadStatus
	// AddConfig enqueues a DownloadConfig. The pool sets cfg.ProgressCh when nil.
	AddConfig func(cfg types.DownloadConfig)
	// Cancel mechanically removes a download from the pool and returns removal metadata.
	Cancel func(id string) types.CancelResult
	// UpdateURL updates the in-memory URL only; LifecycleManager persists to DB.
	UpdateURL func(id, newURL string) error
	// PublishEvent sends an event into the service's broadcast channel.
	PublishEvent func(msg interface{}) error
}

// Pause pauses an active download.
func (mgr *LifecycleManager) Pause(id string) error {
	hooks := mgr.getEngineHooks()
	if hooks.Pause == nil {
		return fmt.Errorf("engine not initialized")
	}

	if hooks.Pause(id) {
		return nil
	}

	// Downloads paused in a prior session are not tracked by the in-memory pool;
	// synthesize a paused event so the UI can clear any transient "pausing" spinner.
	entry, err := state.GetDownload(id)
	if err == nil && entry != nil {
		if hooks.PublishEvent != nil {
			_ = hooks.PublishEvent(events.DownloadPausedMsg{
				DownloadID: id,
				Filename:   entry.Filename,
				Downloaded: entry.Downloaded,
			})
		}
		return nil // Already stopped
	}

	return fmt.Errorf("download not found")
}

// hydrateConfigFromDisk loads the latest persisted pause snapshot from disk
// and merges it into cfg so the download resumes at the correct byte offset
// and task list even when the pool's in-memory state is stale.
func hydrateConfigFromDisk(cfg *types.DownloadConfig) {
	if cfg.URL == "" || cfg.DestPath == "" {
		return
	}
	saved, err := state.LoadState(cfg.URL, cfg.DestPath)
	if err != nil || saved == nil {
		return
	}
	cfg.SavedState = saved
	if saved.TotalSize > 0 {
		cfg.TotalSize = saved.TotalSize
	}
	if len(saved.Tasks) > 0 {
		cfg.SupportsRange = true
	}
}

// Resume resumes a paused download.
//
// Hot path: download is still in pool memory (same session) — extract config directly.
// Cold path: download was paused in a prior session, only stored in DB.
func (mgr *LifecycleManager) Resume(id string) error {
	hooks := mgr.getEngineHooks()

	// Guard: still transitioning to paused
	if hooks.GetStatus != nil {
		if st := hooks.GetStatus(id); st != nil && st.Status == "pausing" {
			return fmt.Errorf("download is still pausing, try again in a moment")
		}
	}

	// Hot path: pool still holds the paused download in memory.
	if hooks.ExtractPausedConfig != nil {
		if cfg := hooks.ExtractPausedConfig(id); cfg != nil {
			hydrateConfigFromDisk(cfg)
			cfg.IsResume = true
			if hooks.AddConfig != nil {
				hooks.AddConfig(*cfg)
			}
			if hooks.PublishEvent != nil {
				_ = hooks.PublishEvent(events.DownloadResumedMsg{
					DownloadID: id,
					Filename:   cfg.Filename,
				})
			}
			return nil
		}
	}

	// Cold path: download from a prior session (only in DB).
	entry, err := state.GetDownload(id)
	if err != nil || entry == nil {
		return fmt.Errorf("download not found")
	}

	if entry.Status == "completed" {
		return fmt.Errorf("download already completed")
	}

	settings := mgr.GetSettings()

	outputPath := settings.General.DefaultDownloadDir
	if outputPath == "" {
		outputPath = "."
	}

	savedState, stateErr := state.LoadState(entry.URL, entry.DestPath)
	if stateErr != nil {
		savedState = nil
	}

	cfg := buildResumeConfig(id, outputPath, entry, savedState, settings)

	if hooks.AddConfig != nil {
		hooks.AddConfig(cfg)
	}
	if hooks.PublishEvent != nil {
		_ = hooks.PublishEvent(events.DownloadResumedMsg{
			DownloadID: id,
			Filename:   entry.Filename,
		})
	}
	return nil
}

// ResumeBatch resumes multiple paused downloads efficiently.
func (mgr *LifecycleManager) ResumeBatch(ids []string) []error {
	errs := make([]error, len(ids))

	hooks := mgr.getEngineHooks()

	settings := mgr.GetSettings()
	outputPath := settings.General.DefaultDownloadDir
	if outputPath == "" {
		outputPath = "."
	}

	// Partition: downloads still in pool memory (hot) vs cold (DB-only).
	var coldIDs []string
	coldIdx := make(map[string]int)

	for i, id := range ids {
		if hooks.GetStatus != nil {
			if st := hooks.GetStatus(id); st != nil && st.Status == "pausing" {
				errs[i] = fmt.Errorf("download is still pausing, try again in a moment")
				continue
			}
		}

		// Try hot path first
		if hooks.ExtractPausedConfig != nil {
			if cfg := hooks.ExtractPausedConfig(id); cfg != nil {
				hydrateConfigFromDisk(cfg)
				cfg.IsResume = true
				if hooks.AddConfig != nil {
					hooks.AddConfig(*cfg)
				}
				if hooks.PublishEvent != nil {
					_ = hooks.PublishEvent(events.DownloadResumedMsg{
						DownloadID: id,
						Filename:   cfg.Filename,
					})
				}
				errs[i] = nil
				continue
			}
		}

		// Tag for cold-path batch load
		coldIDs = append(coldIDs, id)
		coldIdx[id] = i
	}

	if len(coldIDs) == 0 {
		return errs
	}

	states, err := state.LoadStates(coldIDs)
	if err != nil {
		for _, id := range coldIDs {
			idx := coldIdx[id]
			errs[idx] = fmt.Errorf("failed to load state: %w", err)
		}
		return errs
	}

	for _, id := range coldIDs {
		idx := coldIdx[id]
		savedState, ok := states[id]
		if !ok {
			errs[idx] = fmt.Errorf("download not found or completed")
			continue
		}

		cfg := buildResumeConfig(id, outputPath, nil, savedState, settings)
		if hooks.AddConfig != nil {
			hooks.AddConfig(cfg)
		}
		if hooks.PublishEvent != nil {
			_ = hooks.PublishEvent(events.DownloadResumedMsg{
				DownloadID: id,
				Filename:   savedState.Filename,
			})
		}
		errs[idx] = nil
	}

	return errs
}

// Cancel stops a download (both pool in-memory and DB) and emits a removal event.
// The event worker handles file cleanup and DB removal via DownloadRemovedMsg.
func (mgr *LifecycleManager) Cancel(id string) error {
	hooks := mgr.getEngineHooks()

	var filename, destPath string
	var completed bool
	var found bool

	// Mechanical cancel via pool
	if hooks.Cancel != nil {
		result := hooks.Cancel(id)
		if result.Found {
			found = true
			filename = result.Filename
			destPath = result.DestPath
			completed = result.Completed
		}
	}

	// Supplement with DB info (covers DB-only / completed entries)
	if entry, err := state.GetDownload(id); err == nil && entry != nil {
		found = true
		if filename == "" {
			filename = entry.Filename
		}
		if destPath == "" {
			destPath = entry.DestPath
		}
		if entry.Status == "completed" {
			completed = true
		}
	}

	if !found {
		return fmt.Errorf("download not found")
	}

	// Emit removal event — event worker handles DB deletion and file cleanup.
	if hooks.PublishEvent != nil {
		_ = hooks.PublishEvent(events.DownloadRemovedMsg{
			DownloadID: id,
			Filename:   filename,
			DestPath:   destPath,
			Completed:  completed,
		})
	}
	return nil
}

// UpdateURL updates the URL of a download in both the pool (in-memory) and the DB.
func (mgr *LifecycleManager) UpdateURL(id string, newURL string) error {
	hooks := mgr.getEngineHooks()

	// Update in-memory state via pool (validates download state too)
	if hooks.UpdateURL != nil {
		if err := hooks.UpdateURL(id, newURL); err != nil {
			return err
		}
		// Pool update succeeded; persist to DB.
		return state.UpdateURL(id, newURL)
	}
	// No pool connected — DB-only update is correct (no in-memory state to sync).
	return state.UpdateURL(id, newURL)
}

// buildResumeConfig constructs a DownloadConfig for a cold-path resume from saved state.
// When entry is non-nil it provides identity fields (URL, filename, destPath); savedState
// takes precedence for progress, elapsed time, and mirror topology. If savedState is nil,
// SupportsRange is false and the download restarts from the entry's Downloaded offset.
func buildResumeConfig(id, outputPath string, entry *types.DownloadEntry, savedState *types.DownloadState, settings *config.Settings) types.DownloadConfig {
	var destPath, url, filename string
	var totalSize, downloaded int64

	if entry != nil {
		destPath = entry.DestPath
		url = entry.URL
		filename = entry.Filename
		totalSize = entry.TotalSize
		downloaded = entry.Downloaded
	} else if savedState != nil {
		destPath = savedState.DestPath
		url = savedState.URL
		filename = savedState.Filename
		totalSize = savedState.TotalSize
		downloaded = savedState.Downloaded
	}

	var mirrorURLs []string
	var dmState *types.ProgressState

	if savedState != nil {
		dmState = types.NewProgressState(id, savedState.TotalSize)
		dmState.Downloaded.Store(savedState.Downloaded)
		dmState.VerifiedProgress.Store(savedState.Downloaded)
		if savedState.Elapsed > 0 {
			dmState.SetSavedElapsed(time.Duration(savedState.Elapsed))
		}
		if len(savedState.Mirrors) > 0 {
			var mirrors []types.MirrorStatus
			for _, u := range savedState.Mirrors {
				mirrors = append(mirrors, types.MirrorStatus{URL: u, Active: true})
				mirrorURLs = append(mirrorURLs, u)
			}
			dmState.SetMirrors(mirrors)
		}
		dmState.DestPath = destPath
		dmState.SyncSessionStart()
	} else {
		dmState = types.NewProgressState(id, totalSize)
		dmState.Downloaded.Store(downloaded)
		dmState.VerifiedProgress.Store(downloaded)
		dmState.DestPath = destPath
		dmState.SyncSessionStart()
		mirrorURLs = []string{url}
	}

	return types.DownloadConfig{
		URL:           url,
		OutputPath:    outputPath,
		DestPath:      destPath,
		ID:            id,
		Filename:      filename,
		TotalSize:     totalSize,
		SupportsRange: savedState != nil && len(savedState.Tasks) > 0,
		IsResume:      true,
		State:         dmState,
		SavedState:    savedState,
		Runtime:       types.ConvertRuntimeConfig(settings.ToRuntimeConfig()),
		Mirrors:       mirrorURLs,
	}
}
