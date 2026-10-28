package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

func completedSpeedBps(entry types.DownloadRecord) float64 {
	if entry.Status != "completed" {
		return 0
	}
	if entry.AvgSpeed > 0 {
		return entry.AvgSpeed
	}
	if entry.TimeTaken > 0 {
		return float64(entry.TotalSize) * 1000 / float64(entry.TimeTaken)
	}
	return 0
}

type LocalDownloadService struct {
	lifecycle  *orchestrator.LifecycleManager
	settingsMu sync.Mutex
}

func NewLocalDownloadService(lifecycle *orchestrator.LifecycleManager) *LocalDownloadService {
	return &LocalDownloadService{lifecycle: lifecycle}
}

// ReloadSettingsFromDisk uses the same owner and runtime application as edits.
func (s *LocalDownloadService) ReloadSettingsFromDisk() error {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.lifecycle == nil {
		return types.ErrServiceUnavailable
	}
	settings, err := config.LoadSettings()
	if err != nil {
		return err
	}
	return s.reloadSettings(settings)
}

func (s *LocalDownloadService) ReloadSettings(settings *config.Settings) error {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	return s.reloadSettings(settings)
}

// reloadSettings applies the same live settings for TUI, API and rate updates.
// The caller holds settingsMu through persistence and application.
func (s *LocalDownloadService) reloadSettings(settings *config.Settings) error {
	if s.lifecycle == nil {
		return nil
	}
	previous := s.lifecycle.GetSettings()
	globalRate, err := utils.ParseRateLimitValue(settings.Network.GlobalRateLimit.Value)
	if err != nil {
		return err
	}
	defaultRate, err := utils.ParseRateLimitValue(settings.Network.DefaultDownloadRateLimit.Value)
	if err != nil {
		return err
	}
	s.lifecycle.ApplySettings(settings)
	pool := s.lifecycle.GetScheduler()
	if pool == nil {
		return nil
	}
	pool.SetGlobalRateLimit(globalRate)
	if settings.Network.DefaultDownloadRateLimit.Value == previous.Network.DefaultDownloadRateLimit.Value {
		return nil
	}
	pool.SetDefaultDownloadRateLimit(defaultRate)
	var dbErrs []string
	for _, cfg := range pool.GetAll() {
		if !cfg.RateLimitSet {
			if err := store.UpdateDefaultRateLimit(cfg.ID, defaultRate); err != nil {
				dbErrs = append(dbErrs, fmt.Sprintf("%s: %v", cfg.ID, err))
			}
		}
	}
	if len(dbErrs) > 0 {
		return fmt.Errorf("failed to update default rate limit in DB for some downloads: %s", strings.Join(dbErrs, "; "))
	}
	return nil
}

func (s *LocalDownloadService) GetSettings() (*config.Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.lifecycle == nil {
		return nil, types.ErrServiceUnavailable
	}
	return s.lifecycle.GetSettings(), nil
}

func (s *LocalDownloadService) UpdateSettings(settings *config.Settings) error {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	return s.updateSettings(settings)
}

// updateSettings persists and applies a snapshot while settingsMu is held.
func (s *LocalDownloadService) updateSettings(settings *config.Settings) error {
	var err error
	settings, err = settings.NormalizedClone()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSettings, err)
	}
	if s.lifecycle == nil {
		return types.ErrServiceUnavailable
	}
	if err := settings.ValidateStrict(true); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSettings, err)
	}
	settings.Extension.AuthToken.Value = s.lifecycle.GetSettings().Extension.AuthToken.Value
	if err := config.SaveSettings(settings); err != nil {
		return err
	}
	return s.reloadSettings(settings)
}

func (s *LocalDownloadService) PatchSettings(patch config.SettingsPatch) (*config.Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.lifecycle == nil {
		return nil, types.ErrServiceUnavailable
	}
	current := s.lifecycle.GetSettings()
	if patch.Empty() {
		return current, nil
	}
	updated, err := patch.Apply(current)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSettings, err)
	}
	if err := s.updateSettings(updated); err != nil {
		return nil, err
	}
	return s.lifecycle.GetSettings(), nil
}

// StreamLifecycleEvents is the internal reliable persistence subscription.
func (s *LocalDownloadService) StreamLifecycleEvents(ctx context.Context) (<-chan types.DownloadEvent, func(), error) {
	if s.lifecycle == nil || s.lifecycle.GetEventBus() == nil {
		return nil, nil, fmt.Errorf("event bus not initialized")
	}
	stream, cleanup := s.lifecycle.GetEventBus().SubscribeLifecycle()
	if ctx == nil {
		ctx = context.Background()
	}
	stop := context.AfterFunc(ctx, cleanup)
	return stream, func() { stop(); cleanup() }, nil
}

func (s *LocalDownloadService) StreamEvents(ctx context.Context) (<-chan types.DownloadEvent, func(), error) {
	if s.lifecycle == nil || s.lifecycle.GetEventBus() == nil {
		return nil, nil, fmt.Errorf("event bus not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	bus := s.lifecycle.GetEventBus()
	stream, cleanup := bus.Subscribe()
	out := make(chan types.DownloadEvent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		defer cancel()
		defer func() { cleanup() }()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-stream:
				if !ok {
					cleanup()
					if bus.Closed() {
						return
					}
					stream, cleanup = bus.Subscribe()
					msg = types.DownloadEvent{Type: types.EventSystem, ResyncRequired: true, Message: "Live updates fell behind; refreshing download state. Pending approval requests may need resubmission."}
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, func() { cancel(); <-done }, nil
}

func (s *LocalDownloadService) Publish(msg types.DownloadEvent) error {
	if s.lifecycle != nil && s.lifecycle.GetEventBus() != nil {
		return s.lifecycle.GetEventBus().Publish(msg)
	}
	return fmt.Errorf("event bus not initialized")
}

func (s *LocalDownloadService) Shutdown() error {
	if s.lifecycle != nil {
		s.lifecycle.Shutdown()
	}
	return nil
}

func (s *LocalDownloadService) Add(url string, path string, filename string, mirrors []string, headers map[string]string, isExplicitCategory bool, workers int, minChunkSize int64) (string, error) {
	if s.lifecycle == nil {
		return "", types.ErrServiceUnavailable
	}
	req := &orchestrator.DownloadRequest{
		URL:                url,
		Path:               path,
		Filename:           filename,
		Mirrors:            mirrors,
		Headers:            headers,
		IsExplicitCategory: isExplicitCategory,
		Workers:            workers,
		MinChunkSize:       minChunkSize,
	}
	id, _, err := s.lifecycle.Enqueue(context.Background(), req)
	return id, err
}

func (s *LocalDownloadService) AddWithID(url string, path string, filename string, mirrors []string, headers map[string]string, id string, isExplicitCategory bool, workers int, minChunkSize int64) (string, error) {
	if s.lifecycle == nil {
		return "", types.ErrServiceUnavailable
	}
	req := &orchestrator.DownloadRequest{
		URL:                url,
		Path:               path,
		Filename:           filename,
		Mirrors:            mirrors,
		Headers:            headers,
		IsExplicitCategory: isExplicitCategory,
		Workers:            workers,
		MinChunkSize:       minChunkSize,
	}
	newID, _, err := s.lifecycle.EnqueueWithID(context.Background(), req, id)
	return newID, err
}

func (s *LocalDownloadService) Pause(id string) error {
	finish, changeErr := store.BeginDownloadChange(id)
	if changeErr != nil {
		return changeErr
	}
	defer finish()
	if s.lifecycle == nil {
		return types.ErrServiceUnavailable
	}
	return s.lifecycle.Pause(id)
}

func (s *LocalDownloadService) Resume(id string) error {
	finish, changeErr := store.BeginDownloadChange(id)
	if changeErr != nil {
		return changeErr
	}
	defer finish()
	if s.lifecycle == nil {
		return types.ErrServiceUnavailable
	}
	return s.lifecycle.Resume(id)
}

func (s *LocalDownloadService) ResumeBatch(ids []string) []error {
	if s.lifecycle == nil {
		return []error{types.ErrServiceUnavailable}
	}
	return s.lifecycle.ResumeBatch(ids)
}

func (s *LocalDownloadService) UpdateURL(id string, newURL string) error {
	finish, changeErr := store.BeginDownloadChange(id)
	if changeErr != nil {
		return changeErr
	}
	defer finish()
	if s.lifecycle == nil {
		return types.ErrServiceUnavailable
	}
	return s.lifecycle.UpdateURL(id, newURL)
}

func (s *LocalDownloadService) Delete(id string) error {
	finish, changeErr := store.BeginDownloadChange(id)
	if changeErr != nil {
		return changeErr
	}
	defer finish()
	if s.lifecycle == nil {
		return types.ErrServiceUnavailable
	}
	return s.lifecycle.Cancel(id)
}

func (s *LocalDownloadService) Purge(id string) error {
	finish, changeErr := store.BeginDownloadChange(id)
	if changeErr != nil {
		return changeErr
	}
	defer finish()
	destPath := ""
	status, err := s.GetStatus(id)
	if err == nil && status != nil {
		destPath = filepath.Clean(status.DestPath)
	} else {
		history, err := s.History()
		if err == nil {
			for _, entry := range history {
				if entry.ID == id {
					destPath = filepath.Clean(entry.DestPath)
					break
				}
			}
		}
	}
	if err := s.Delete(id); err != nil {
		return err
	}
	if destPath != "" && destPath != "." {
		var errs []string
		if err := utils.RemoveFile(destPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err.Error())
		}
		if err := utils.RemoveFile(destPath + types.IncompleteSuffix); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err.Error())
		}
		if len(errs) > 0 {
			return fmt.Errorf("failed to delete files: %s", strings.Join(errs, ", "))
		}
	}
	return nil
}

func (s *LocalDownloadService) GetStatus(id string) (*types.DownloadStatus, error) {
	if id == "" {
		return nil, fmt.Errorf("missing id")
	}
	if s.lifecycle != nil && s.lifecycle.GetScheduler() != nil {
		if status := s.lifecycle.GetScheduler().GetStatus(id); status != nil {
			if entry, err := store.GetDownload(id); err == nil && entry != nil {
				status.CategoryOverride = entry.CategoryOverride
				if entry.Status == "completed" {
					status.DestPath = entry.DestPath
				}
			}
			status.Moving = store.IsMoving(id)
			status.FileMissing = completedFileMissing(status.Status, status.DestPath)
			return status, nil
		}
	}
	entry, err := store.GetDownload(id)
	if err == nil && entry != nil {
		var progress float64
		if entry.TotalSize > 0 {
			progress = float64(entry.Downloaded) * 100 / float64(entry.TotalSize)
		} else if entry.Status == "completed" {
			progress = 100.0
		}
		status := types.DownloadStatus{
			Moving:           store.IsMoving(id),
			CategoryOverride: entry.CategoryOverride,
			ID:               entry.ID,
			URL:              entry.URL,
			Filename:         entry.Filename,
			DestPath:         entry.DestPath,
			TotalSize:        entry.TotalSize,
			Downloaded:       entry.Downloaded,
			Progress:         progress,
			Speed:            completedSpeedBps(*entry),
			Status:           entry.Status,
			Error:            entry.Error,
			TimeTaken:        entry.TimeTaken,
			AvgSpeed:         entry.AvgSpeed,
			RateLimit:        entry.RateLimit,
			RateLimitSet:     entry.RateLimitSet,
		}
		status.FileMissing = completedFileMissing(status.Status, status.DestPath)
		return &status, nil
	}
	return nil, types.ErrNotFound
}

func (s *LocalDownloadService) History() ([]types.DownloadRecord, error) {
	return store.LoadCompletedDownloads()
}
func (s *LocalDownloadService) ClearCompleted() (int64, error) {
	return store.RemoveCompletedDownloads()
}
func (s *LocalDownloadService) ClearFailed() (int64, error) {
	return store.RemoveFailedDownloads()
}

func (s *LocalDownloadService) SetRateLimit(id string, rate int64) error {
	finish, changeErr := store.BeginDownloadChange(id)
	if changeErr != nil {
		return changeErr
	}
	defer finish()
	if rate < 0 {
		return fmt.Errorf("rate limit must be non-negative")
	}
	if s.lifecycle == nil || s.lifecycle.GetScheduler() == nil {
		return types.ErrPoolNotInit
	}

	entry, err := store.GetDownload(id)
	if err != nil && !errors.Is(err, types.ErrNotFound) {
		return err
	}

	poolStatus := s.lifecycle.GetScheduler().GetStatus(id)
	if poolStatus == nil && (entry == nil || entry.Status == "completed") {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}

	err = store.UpdateRateLimit(id, rate)
	if err != nil && !errors.Is(err, types.ErrNotFound) {
		return err
	}

	foundInPool := s.lifecycle.GetScheduler().SetDownloadRateLimit(id, rate)
	if err != nil && !foundInPool {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}
	return nil
}

func (s *LocalDownloadService) ClearRateLimit(id string) error {
	finish, changeErr := store.BeginDownloadChange(id)
	if changeErr != nil {
		return changeErr
	}
	defer finish()
	if s.lifecycle == nil || s.lifecycle.GetScheduler() == nil {
		return types.ErrPoolNotInit
	}

	entry, err := store.GetDownload(id)
	if err != nil && !errors.Is(err, types.ErrNotFound) {
		return err
	}

	poolStatus := s.lifecycle.GetScheduler().GetStatus(id)
	if poolStatus == nil && (entry == nil || entry.Status == "completed") {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}

	err = store.ClearRateLimit(id)
	if err != nil && !errors.Is(err, types.ErrNotFound) {
		return err
	}

	foundInPool := s.lifecycle.GetScheduler().ClearDownloadRateLimit(id)
	if err != nil && !foundInPool {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}
	return nil
}

func (s *LocalDownloadService) SetGlobalRateLimit(rate int64) error {
	if rate < 0 {
		return fmt.Errorf("rate limit must be non-negative")
	}
	if s.lifecycle == nil || s.lifecycle.GetScheduler() == nil {
		return types.ErrPoolNotInit
	}

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	settings := s.lifecycle.GetSettings()
	settings.Network.GlobalRateLimit.Value = fmt.Sprintf("%d B/s", rate)
	return s.updateSettings(settings)
}

func (s *LocalDownloadService) SetDefaultRateLimit(rate int64) error {
	if rate < 0 {
		return fmt.Errorf("rate limit must be non-negative")
	}
	if s.lifecycle == nil || s.lifecycle.GetScheduler() == nil {
		return types.ErrPoolNotInit
	}

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	settings := s.lifecycle.GetSettings()
	settings.Network.DefaultDownloadRateLimit.Value = fmt.Sprintf("%d B/s", rate)
	return s.updateSettings(settings)
}

func (s *LocalDownloadService) List() ([]types.DownloadStatus, error) {
	var statuses []types.DownloadStatus
	if s.lifecycle != nil && s.lifecycle.GetScheduler() != nil {
		activeConfigs := s.lifecycle.GetScheduler().GetAll()
		for _, cfg := range activeConfigs {
			statusStr := "downloading"
			var errStr string
			if st := s.lifecycle.GetScheduler().GetStatus(cfg.ID); st != nil {
				statusStr = st.Status
				errStr = st.Error
			}
			status := types.DownloadStatus{
				ID:           cfg.ID,
				URL:          cfg.URL,
				Filename:     cfg.Filename,
				Status:       statusStr,
				Error:        errStr,
				RateLimit:    cfg.RateLimit,
				RateLimitSet: cfg.RateLimitSet,
			}
			if cfg.ProgressState != nil {
				cp := progress.CfgProgress(&cfg)
				if cp != nil {
					downloaded, totalSize, _, sessionElapsed, connections, sessionStart := cp.GetProgress()
					status.TotalSize = totalSize
					status.Downloaded = downloaded
					if dp := cp.GetDestPath(); dp != "" {
						status.DestPath = dp
					}
					if status.TotalSize > 0 {
						status.Progress = float64(status.Downloaded) * 100 / float64(status.TotalSize)
					}
					status.Connections = int(connections)
					if cp.IsPausing() {
						status.Status = "pausing"
					} else if cp.IsPaused() {
						status.Status = "paused"
					} else if cp.Done.Load() {
						status.Status = "completed"
					}
					// GetStatus and metric reads are separate snapshots; recheck errors
					// so a worker failure between those reads is not reported as downloading.
					if err := cp.GetError(); err != nil {
						status.Status = "error"
						status.Error = err.Error()
					}
					if status.Status == "downloading" {
						sessionDownloaded := downloaded - sessionStart
						if sessionElapsed.Seconds() > 0 && sessionDownloaded > 0 {
							status.Speed = float64(sessionDownloaded) / sessionElapsed.Seconds()
							remaining := status.TotalSize - status.Downloaded
							if remaining > 0 && status.Speed > 0 {
								status.ETA = int64(float64(remaining) / status.Speed)
							}
						}
					}
				}
			}
			statuses = append(statuses, status)
		}
	}
	dbDownloads, err := store.ListAllDownloads()
	if err == nil {
		existingIDs := make(map[string]bool)
		for _, s := range statuses {
			existingIDs[s.ID] = true
		}
		for _, d := range dbDownloads {
			if existingIDs[d.ID] {
				continue
			}
			if d.Status == "completed" {
				if info, err := os.Stat(d.DestPath); err == nil && info.Mode().IsRegular() {
					d.TotalSize = info.Size()
					d.Downloaded = info.Size()
				}
			}
			var progress float64
			if d.TotalSize > 0 {
				progress = float64(d.Downloaded) * 100 / float64(d.TotalSize)
			} else if d.Status == "completed" {
				progress = 100.0
			}
			statuses = append(statuses, types.DownloadStatus{
				CategoryOverride: d.CategoryOverride,
				ID:               d.ID,
				URL:              d.URL,
				Filename:         d.Filename,
				DestPath:         d.DestPath,
				Status:           d.Status,
				Error:            d.Error,
				TotalSize:        d.TotalSize,
				Downloaded:       d.Downloaded,
				Progress:         progress,
				Speed:            completedSpeedBps(d),
				Connections:      0,
				TimeTaken:        d.TimeTaken,
				AvgSpeed:         d.AvgSpeed,
				RateLimit:        d.RateLimit,
				RateLimitSet:     d.RateLimitSet,
			})
		}
	}
	persisted := make(map[string]types.DownloadRecord, len(dbDownloads))
	for _, entry := range dbDownloads {
		persisted[entry.ID] = entry
	}
	for i := range statuses {
		if entry, ok := persisted[statuses[i].ID]; ok {
			statuses[i].CategoryOverride = entry.CategoryOverride
			if entry.Status == "completed" {
				statuses[i].DestPath = entry.DestPath
			}
		}
	}

	for i := range statuses {
		statuses[i].Moving = store.IsMoving(statuses[i].ID)
		statuses[i].FileMissing = completedFileMissing(statuses[i].Status, statuses[i].DestPath)
	}
	return statuses, nil
}

func (s *LocalDownloadService) AssignCategory(id, category string, move bool) error {
	if store.IsMoving(id) {
		return types.ErrMoving
	}
	if s.lifecycle == nil {
		return types.ErrServiceUnavailable
	}
	settings := s.lifecycle.GetSettings()
	directory := config.Resolve[string](settings.General.DefaultDownloadDir)
	if category != "" {
		found := false
		for _, rule := range settings.Categories.Categories {
			if rule.Name == category {
				directory = config.ResolveCategoryPath(&rule, directory)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown category %q", category)
		}
	}
	var err error
	if move && !filepath.IsAbs(directory) {
		return fmt.Errorf("category destination must be an absolute path")
	}
	if move {
		var entry *types.DownloadRecord
		entry, err = store.GetDownload(id)
		if err == nil && entry == nil {
			return types.ErrNotFound
		}
		if err == nil {
			if entry.Status != "completed" {
				err = store.ScheduleCategoryMove(id, category, directory)
			} else {
				err = store.MoveCompletedCategory(id, category, directory, func(moving bool) {
					_ = s.Publish(types.DownloadEvent{Type: types.EventSystem, DownloadID: id, Moving: &moving})
				})
			}
		}
	} else {
		err = store.SetCategory(id, category)
	}
	if err == nil {
		entry, _ := store.GetDownload(id)
		event := types.DownloadEvent{Type: types.EventSystem, DownloadID: id, CategoryOverride: &category}
		if entry != nil {
			event.DestPath = entry.DestPath
		}
		_ = s.Publish(event)
	}
	return err
}

// Permission and temporary I/O errors do not establish that a file is missing.
func completedFileMissing(status, path string) bool {
	if status != "completed" || path == "" {
		return false
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
