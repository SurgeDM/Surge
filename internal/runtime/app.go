package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
	"github.com/surge-downloader/surge/internal/utils"
)

// Components captures the mutable local backend pieces that Phase 1 moves
// under a runtime-owned application struct. Cmd keeps temporary mirrors of
// these values until later phases remove the compatibility globals entirely.
type Components struct {
	Pool             *download.WorkerPool
	ProgressCh       chan any
	Service          core.DownloadService
	Lifecycle        *processing.LifecycleManager
	LifecycleCleanup func()
}

// App owns the process-local backend state for a single Surge instance.
// In Phase 1 this mostly wraps the existing pool/service/lifecycle wiring.
type App struct {
	settings *config.Settings

	pool       *download.WorkerPool
	progressCh chan any
	service    core.DownloadService

	lifecycle        *processing.LifecycleManager
	lifecycleCleanup func()
	lifecycleMu      sync.Mutex

	enqueueCtx    context.Context
	enqueueCancel context.CancelFunc
	enqueueMu     sync.Mutex

	shutdownOnce sync.Once
	shutdownErr  error
}

func NewEmpty() *App {
	app := &App{}
	app.ResetEnqueueContext()
	return app
}

func NewLocal(settings *config.Settings) *App {
	if settings == nil {
		settings = config.DefaultSettings()
	}

	progressCh := make(chan any, types.ProgressChannelBuffer)
	pool := download.NewWorkerPool(progressCh, settings.Network.MaxConcurrentDownloads)

	app := NewEmpty()
	app.settings = settings
	app.pool = pool
	app.progressCh = progressCh
	return app
}

func (a *App) ApplyComponents(c Components) {
	a.pool = c.Pool
	a.progressCh = c.ProgressCh
	a.service = c.Service

	a.lifecycleMu.Lock()
	a.lifecycle = c.Lifecycle
	a.lifecycleCleanup = c.LifecycleCleanup
	a.lifecycleMu.Unlock()
}

func (a *App) Components() Components {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()

	return Components{
		Pool:             a.pool,
		ProgressCh:       a.progressCh,
		Service:          a.service,
		Lifecycle:        a.lifecycle,
		LifecycleCleanup: a.lifecycleCleanup,
	}
}

func (a *App) Pool() *download.WorkerPool {
	return a.pool
}

func (a *App) ProgressCh() chan any {
	return a.progressCh
}

func (a *App) Service() core.DownloadService {
	return a.service
}

func (a *App) CurrentLifecycle() *processing.LifecycleManager {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	return a.lifecycle
}

func (a *App) TakeLifecycleCleanup() func() {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()

	cleanup := a.lifecycleCleanup
	a.lifecycleCleanup = nil
	return cleanup
}

func (a *App) ResetEnqueueContext() {
	a.enqueueMu.Lock()
	defer a.enqueueMu.Unlock()

	if a.enqueueCancel != nil {
		a.enqueueCancel()
	}
	a.enqueueCtx, a.enqueueCancel = context.WithCancel(context.Background())
}

func (a *App) ensureEnqueueContextLocked() {
	if a.enqueueCtx == nil || a.enqueueCancel == nil {
		a.enqueueCtx, a.enqueueCancel = context.WithCancel(context.Background())
	}
}

func (a *App) EnqueueContext() context.Context {
	a.enqueueMu.Lock()
	defer a.enqueueMu.Unlock()

	a.ensureEnqueueContextLocked()
	return a.enqueueCtx
}

func (a *App) EnqueueCancel() context.CancelFunc {
	a.enqueueMu.Lock()
	defer a.enqueueMu.Unlock()

	a.ensureEnqueueContextLocked()
	return a.enqueueCancel
}

func (a *App) CancelEnqueue() {
	a.enqueueMu.Lock()
	cancel := a.enqueueCancel
	a.enqueueMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) CurrentPoolConfigs() []types.DownloadConfig {
	if a.pool == nil {
		return nil
	}
	return a.pool.GetAll()
}

func (a *App) LifecycleForService(service core.DownloadService) (*processing.LifecycleManager, error) {
	lifecycle := a.CurrentLifecycle()
	if service == nil || a.service == nil || service != a.service {
		return lifecycle, nil
	}
	return a.ensureLocalLifecycle(a.service, a.CurrentPoolConfigs)
}

func (a *App) EnsureLocalServiceAndLifecycle() error {
	if a.service == nil {
		localService := core.NewLocalDownloadServiceWithInput(a.pool, a.progressCh)
		a.service = localService
		if a.progressCh == nil {
			a.progressCh = localService.InputCh
		}
	}

	lifecycle, err := a.ensureLocalLifecycle(a.service, a.CurrentPoolConfigs)
	if err != nil {
		return err
	}

	if localService, ok := a.service.(*core.LocalDownloadService); ok {
		if a.progressCh == nil {
			a.progressCh = localService.InputCh
		}
		if a.pool != nil {
			lifecycle.SetEngineHooks(processing.EngineHooks{
				Pause:        a.pool.Pause,
				Resume:       a.pool.Resume,
				GetStatus:    a.pool.GetStatus,
				AddConfig:    a.pool.Add,
				PublishEvent: localService.Publish,
			})
		}

		localService.PauseFunc = lifecycle.Pause
		localService.ResumeFunc = lifecycle.Resume
		localService.ResumeBatchFunc = lifecycle.ResumeBatch
	}

	return nil
}

func (a *App) Shutdown() error {
	a.shutdownOnce.Do(func() {
		a.CancelEnqueue()

		if a.service != nil {
			a.shutdownErr = a.service.Shutdown()
		} else if a.pool != nil {
			a.pool.GracefulShutdown()
		}

		if cleanup := a.TakeLifecycleCleanup(); cleanup != nil {
			cleanup()
		}
	})
	return a.shutdownErr
}

func buildPoolIsNameActive(getAll func() []types.DownloadConfig) processing.IsNameActiveFunc {
	if getAll == nil {
		return nil
	}

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
			if existingDir == "" || existingName == "" {
				continue
			}
			if utils.EnsureAbsPath(existingDir) == dir && existingName == name {
				return true
			}
		}
		return false
	}
}

func newLocalLifecycleManager(service core.DownloadService, getAll func() []types.DownloadConfig) *processing.LifecycleManager {
	var addFunc processing.AddDownloadFunc
	var addWithIDFunc processing.AddDownloadWithIDFunc
	if service != nil {
		addFunc = service.Add
		addWithIDFunc = service.AddWithID
	}

	return processing.NewLifecycleManager(addFunc, addWithIDFunc, buildPoolIsNameActive(getAll))
}

func startLifecycleEventWorker(service core.DownloadService, mgr *processing.LifecycleManager) (func(), error) {
	if service == nil || mgr == nil {
		return nil, nil
	}

	managerStream, managerCleanup, err := service.StreamEvents(context.Background())
	if err != nil {
		return nil, err
	}
	go mgr.StartEventWorker(managerStream)
	return managerCleanup, nil
}

func (a *App) ensureLocalLifecycle(service core.DownloadService, getAll func() []types.DownloadConfig) (*processing.LifecycleManager, error) {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()

	if a.lifecycle == nil {
		a.lifecycle = newLocalLifecycleManager(service, getAll)
	}
	if a.lifecycleCleanup == nil {
		cleanup, err := startLifecycleEventWorker(service, a.lifecycle)
		if err != nil {
			return nil, err
		}
		a.lifecycleCleanup = cleanup
	}
	return a.lifecycle, nil
}
