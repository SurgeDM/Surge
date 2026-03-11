package cmd

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
	runtimeapp "github.com/surge-downloader/surge/internal/runtime"
	"github.com/surge-downloader/surge/internal/utils"
)

// Legacy compatibility shims for cmd and cmd tests. The runtime-owned App is the
// authoritative source of this state; these mirrors exist so Phase 1 can land
// without rewriting every cmd caller at once.
var (
	GlobalPool             *download.WorkerPool
	GlobalProgressCh       chan any
	GlobalService          core.DownloadService
	GlobalLifecycleCleanup func()
	GlobalLifecycle        *processing.LifecycleManager

	globalApp *runtimeapp.App

	legacyLifecycleMu sync.Mutex
)

func syncLegacyGlobalsFromApp() {
	if globalApp == nil {
		return
	}

	components := globalApp.Components()
	GlobalPool = components.Pool
	GlobalProgressCh = components.ProgressCh
	GlobalService = components.Service
	GlobalLifecycle = components.Lifecycle
	GlobalLifecycleCleanup = components.LifecycleCleanup
}

func currentApp() *runtimeapp.App {
	var previous runtimeapp.Components
	if globalApp != nil {
		previous = globalApp.Components()
	}
	executionChanged := executionComponentsOutOfSync()
	if globalApp == nil || executionChanged {
		globalApp = runtimeapp.NewEmpty()
	}

	lifecycle := GlobalLifecycle
	lifecycleCleanup := GlobalLifecycleCleanup
	if executionChanged && previous.Lifecycle != nil && lifecycle == previous.Lifecycle {
		// A lifecycle manager captures service Add/AddWithID callbacks. If the
		// service or pool changes but callers forgot to clear the legacy lifecycle
		// mirror, it becomes unsafe to reuse.
		lifecycle = nil
		GlobalLifecycle = nil
	}

	globalApp.ApplyComponents(runtimeapp.Components{
		Pool:             GlobalPool,
		ProgressCh:       GlobalProgressCh,
		Service:          GlobalService,
		Lifecycle:        lifecycle,
		LifecycleCleanup: lifecycleCleanup,
	})
	return globalApp
}

func executionComponentsOutOfSync() bool {
	if globalApp == nil {
		return false
	}

	components := globalApp.Components()
	return components.Pool != GlobalPool ||
		components.ProgressCh != GlobalProgressCh ||
		components.Service != GlobalService
}

func initLocalRuntime(settings *config.Settings) {
	globalApp = runtimeapp.NewLocal(settings)
	syncLegacyGlobalsFromApp()
}

func currentLifecycle() *processing.LifecycleManager {
	lifecycle := currentApp().CurrentLifecycle()
	syncLegacyGlobalsFromApp()
	return lifecycle
}

func resetGlobalEnqueueContext() {
	currentApp().ResetEnqueueContext()
}

func currentEnqueueContext() context.Context {
	return currentApp().EnqueueContext()
}

func currentEnqueueCancel() context.CancelFunc {
	return currentApp().EnqueueCancel()
}

func cancelGlobalEnqueue() {
	currentApp().CancelEnqueue()
}

func takeLifecycleCleanup() func() {
	cleanup := currentApp().TakeLifecycleCleanup()
	syncLegacyGlobalsFromApp()
	return cleanup
}

func currentPoolConfigs() []types.DownloadConfig {
	return currentApp().CurrentPoolConfigs()
}

func lifecycleForLocalService(service core.DownloadService) (*processing.LifecycleManager, error) {
	lifecycle, err := currentApp().LifecycleForService(service)
	syncLegacyGlobalsFromApp()
	return lifecycle, err
}

func ensureGlobalLocalServiceAndLifecycle() error {
	err := currentApp().EnsureLocalServiceAndLifecycle()
	syncLegacyGlobalsFromApp()
	return err
}

// Transitional compatibility helpers retained for cmd tests during Phase 1.
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

func ensureLocalLifecycle(service core.DownloadService, getAll func() []types.DownloadConfig) (*processing.LifecycleManager, error) {
	legacyLifecycleMu.Lock()
	defer legacyLifecycleMu.Unlock()

	if GlobalLifecycle == nil {
		GlobalLifecycle = newLocalLifecycleManager(service, getAll)
	}
	if GlobalLifecycleCleanup == nil {
		cleanup, err := startLifecycleEventWorker(service, GlobalLifecycle)
		if err != nil {
			return nil, err
		}
		GlobalLifecycleCleanup = cleanup
	}
	return GlobalLifecycle, nil
}
