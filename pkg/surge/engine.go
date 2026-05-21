package surge

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/utils"
)

// LocalEngineOptions configures an embedded Surge download engine.
type LocalEngineOptions struct {
	ProgressCh   chan any
	MaxDownloads int
}

// LocalEngine is a fully wired embedded download engine.
type LocalEngine struct {
	Pool      *WorkerPool
	Service   *LocalDownloadService
	Lifecycle *processing.LifecycleManager

	cleanup       func()
	lifecycleDone chan struct{}
}

// NewLocalEngine creates an embedded download engine with lifecycle processing enabled.
func NewLocalEngine(opts LocalEngineOptions) (*LocalEngine, error) {
	progressCh := opts.ProgressCh
	if progressCh == nil {
		progressCh = make(chan any, 100)
	}

	pool := NewWorkerPool(progressCh, opts.MaxDownloads)
	service := NewLocalDownloadServiceWithInput(pool, progressCh)
	lifecycle := processing.NewLifecycleManager(service.Add, service.AddWithID, isNameActive(pool.GetAll))
	lifecycle.SetEngineHooks(processing.EngineHooks{
		Pause:               pool.Pause,
		ExtractPausedConfig: pool.ExtractPausedConfig,
		GetStatus:           pool.GetStatus,
		AddConfig:           pool.Add,
		Cancel:              pool.Cancel,
		UpdateURL:           pool.UpdateURL,
		PublishEvent:        service.Publish,
	})
	service.SetLifecycleHooks(LifecycleHooks{
		Pause:       lifecycle.Pause,
		Resume:      lifecycle.Resume,
		ResumeBatch: lifecycle.ResumeBatch,
		Cancel:      lifecycle.Cancel,
		UpdateURL:   lifecycle.UpdateURL,
	})

	stream, cleanup, err := service.StreamEvents(context.Background())
	if err != nil {
		_ = service.Shutdown()
		return nil, err
	}
	lifecycleDone := make(chan struct{})
	go func() {
		defer close(lifecycleDone)
		lifecycle.StartEventWorker(stream)
	}()

	return &LocalEngine{
		Pool:          pool,
		Service:       service,
		Lifecycle:     lifecycle,
		cleanup:       cleanup,
		lifecycleDone: lifecycleDone,
	}, nil
}

// Shutdown stops lifecycle processing and gracefully shuts down the embedded service.
func (e *LocalEngine) Shutdown() error {
	if e == nil {
		return nil
	}
	var err error
	if e.Service != nil {
		err = e.Service.Shutdown()
	}
	if e.lifecycleDone != nil {
		<-e.lifecycleDone
		e.lifecycleDone = nil
	}
	if e.cleanup != nil {
		e.cleanup()
		e.cleanup = nil
	}
	return err
}

func isNameActive(getAll func() []DownloadConfig) processing.IsNameActiveFunc {
	return func(dir, name string) bool {
		dir = utils.EnsureAbsPath(strings.TrimSpace(dir))
		name = strings.TrimSpace(name)
		if dir == "" || name == "" {
			return false
		}

		for _, cfg := range getAll() {
			existingName := strings.TrimSpace(cfg.Filename)
			existingDir := strings.TrimSpace(cfg.OutputPath)
			if cfg.DestPath != "" {
				existingDir = filepath.Dir(cfg.DestPath)
				if existingName == "" {
					existingName = filepath.Base(cfg.DestPath)
				}
			}
			if cfg.State != nil {
				if stateName := strings.TrimSpace(cfg.State.GetFilename()); stateName != "" {
					existingName = stateName
				}
				if stateDestPath := strings.TrimSpace(cfg.State.GetDestPath()); stateDestPath != "" {
					existingDir = filepath.Dir(stateDestPath)
					if existingName == "" {
						existingName = filepath.Base(stateDestPath)
					}
				}
			}
			if utils.EnsureAbsPath(existingDir) == dir && existingName == name {
				return true
			}
		}
		return false
	}
}
