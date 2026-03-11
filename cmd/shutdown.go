package cmd

import (
	"fmt"
	"sync"

	"github.com/surge-downloader/surge/internal/utils"
)

var (
	globalShutdownOnce sync.Once
	globalShutdownErr  error
	globalShutdownFn   = defaultGlobalShutdown
)

func defaultGlobalShutdown() error {
	err := currentApp().Shutdown()
	syncLegacyGlobalsFromApp()
	return err
}

func executeGlobalShutdown(reason string) error {
	globalShutdownOnce.Do(func() {
		utils.Debug("Executing graceful shutdown (%s)", reason)
		globalShutdownErr = globalShutdownFn()
		if globalShutdownErr != nil {
			globalShutdownErr = fmt.Errorf("graceful shutdown failed: %w", globalShutdownErr)
		}
	})
	return globalShutdownErr
}

func resetGlobalShutdownCoordinatorForTest(fn func() error) {
	globalShutdownOnce = sync.Once{}
	globalShutdownErr = nil
	globalApp = nil
	resetGlobalEnqueueContext()
	_ = takeLifecycleCleanup()
	if fn != nil {
		globalShutdownFn = fn
		return
	}
	globalShutdownFn = defaultGlobalShutdown
}
