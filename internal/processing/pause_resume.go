package processing

import (
	"fmt"
	"time"

	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
)

// EngineHooks defines the minimal callbacks Processing needs to orchestrate the worker pool.
type EngineHooks struct {
	Pause        func(id string) bool
	Resume       func(id string) bool
	GetStatus    func(id string) *types.DownloadStatus
	AddConfig    func(cfg types.DownloadConfig)
	PublishEvent func(msg interface{}) error
}

// Pause pauses an active download.
func (mgr *LifecycleManager) Pause(id string) error {
	if mgr.engineHooks.Pause == nil {
		return fmt.Errorf("engine not initialized")
	}

	if mgr.engineHooks.Pause(id) {
		return nil
	}

	// Downloads paused in a prior session are not tracked by the in-memory pool;
	// synthesize a paused event so the UI can clear any transient "pausing" spinner.
	entry, err := state.GetDownload(id)
	if err == nil && entry != nil {
		// Emit paused event so UI clears "pausing" state
		if mgr.engineHooks.PublishEvent != nil {
			_ = mgr.engineHooks.PublishEvent(events.DownloadPausedMsg{
				DownloadID: id,
				Filename:   entry.Filename,
				Downloaded: entry.Downloaded,
			})
		}
		return nil // Already stopped
	}

	return fmt.Errorf("download not found")
}

// Resume resumes a paused download.
func (mgr *LifecycleManager) Resume(id string) error {
	if mgr.engineHooks.Resume == nil {
		return fmt.Errorf("engine not initialized")
	}

	if mgr.engineHooks.GetStatus != nil {
		if st := mgr.engineHooks.GetStatus(id); st != nil && st.Status == "pausing" {
			return fmt.Errorf("download is still pausing, try again in a moment")
		}
	}

	if mgr.engineHooks.Resume(id) {
		return nil
	}

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

	var mirrorURLs []string
	var dmState *types.ProgressState

	if stateErr == nil && savedState != nil {
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
		dmState.DestPath = entry.DestPath
		dmState.SyncSessionStart()
	} else {
		dmState = types.NewProgressState(id, entry.TotalSize)
		dmState.Downloaded.Store(entry.Downloaded)
		dmState.VerifiedProgress.Store(entry.Downloaded)
		dmState.DestPath = entry.DestPath
		dmState.SyncSessionStart()
		mirrorURLs = []string{entry.URL}
	}

	cfg := types.DownloadConfig{
		URL:           entry.URL,
		OutputPath:    outputPath,
		DestPath:      entry.DestPath,
		ID:            id,
		Filename:      entry.Filename,
		TotalSize:     entry.TotalSize,
		SupportsRange: savedState != nil && len(savedState.Tasks) > 0,
		IsResume:      true,
		State:         dmState,
		SavedState:    savedState, // Pass loaded state to avoid re-query
		Runtime:       types.ConvertRuntimeConfig(settings.ToRuntimeConfig()),
		Mirrors:       mirrorURLs,
	}

	mgr.engineHooks.AddConfig(cfg)
	if mgr.engineHooks.PublishEvent != nil {
		_ = mgr.engineHooks.PublishEvent(events.DownloadResumedMsg{
			DownloadID: id,
			Filename:   entry.Filename,
		})
	}
	return nil
}

// ResumeBatch resumes multiple paused downloads efficiently.
func (mgr *LifecycleManager) ResumeBatch(ids []string) []error {
	errs := make([]error, len(ids))

	if mgr.engineHooks.Resume == nil {
		for i := range errs {
			errs[i] = fmt.Errorf("engine not initialized")
		}
		return errs
	}

	var toLoad []string
	idMap := make(map[string]int)

	for i, id := range ids {
		if st := mgr.engineHooks.GetStatus(id); st != nil && st.Status == "pausing" {
			errs[i] = fmt.Errorf("download is still pausing, try again in a moment")
			continue
		}

		if mgr.engineHooks.Resume(id) {
			errs[i] = nil
		} else {
			toLoad = append(toLoad, id)
			idMap[id] = i
		}
	}

	if len(toLoad) == 0 {
		return errs
	}

	settings := mgr.GetSettings()

	outputPath := settings.General.DefaultDownloadDir
	if outputPath == "" {
		outputPath = "."
	}

	states, err := state.LoadStates(toLoad)
	if err != nil {
		for _, id := range toLoad {
			idx := idMap[id]
			errs[idx] = fmt.Errorf("failed to load state: %w", err)
		}
		return errs
	}

	for _, id := range toLoad {
		idx := idMap[id]
		savedState, ok := states[id]
		if !ok {
			errs[idx] = fmt.Errorf("download not found or completed")
			continue
		}

		var dmState *types.ProgressState
		var mirrorURLs []string

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
		dmState.DestPath = savedState.DestPath
		dmState.SyncSessionStart()

		cfg := types.DownloadConfig{
			URL:           savedState.URL,
			OutputPath:    outputPath,
			DestPath:      savedState.DestPath,
			ID:            id,
			Filename:      savedState.Filename,
			TotalSize:     savedState.TotalSize,
			SupportsRange: len(savedState.Tasks) > 0,
			IsResume:      true,
			State:         dmState,
			SavedState:    savedState, // Pass loaded state to avoid re-query
			Runtime:       types.ConvertRuntimeConfig(settings.ToRuntimeConfig()),
			Mirrors:       mirrorURLs,
		}

		mgr.engineHooks.AddConfig(cfg)
		errs[idx] = nil
	}

	return errs
}
