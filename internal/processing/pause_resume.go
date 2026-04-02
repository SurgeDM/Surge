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
type EngineHooks struct {
	Pause     func(id string) bool
	Resume    func(id string) bool
	GetStatus func(id string) *types.DownloadStatus
	// AddConfig enqueues a download config. Implementations must ensure cfg.ProgressCh
	// is set; pool.Add fills it from p.progressCh when nil.
	AddConfig    func(cfg types.DownloadConfig)
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

// Resume resumes a paused download.
func (mgr *LifecycleManager) Resume(id string) error {
	hooks := mgr.getEngineHooks()
	if hooks.Resume == nil {
		return fmt.Errorf("engine not initialized")
	}

	if hooks.GetStatus != nil {
		if st := hooks.GetStatus(id); st != nil && st.Status == "pausing" {
			return fmt.Errorf("download is still pausing, try again in a moment")
		}
	}

	if hooks.Resume(id) {
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
	if hooks.Resume == nil {
		for i := range errs {
			errs[i] = fmt.Errorf("engine not initialized")
		}
		return errs
	}

	var toLoad []string
	idMap := make(map[string]int)

	for i, id := range ids {
		if hooks.GetStatus != nil {
			if st := hooks.GetStatus(id); st != nil && st.Status == "pausing" {
				errs[i] = fmt.Errorf("download is still pausing, try again in a moment")
				continue
			}
		}

		if hooks.Resume(id) {
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
