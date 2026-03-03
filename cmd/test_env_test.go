package cmd

import (
	"testing"

	"github.com/adrg/xdg"
	"github.com/surge-downloader/surge/internal/engine/state"
)

func setupXDGEnvIsolation(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()

	oldConfigHome := xdg.ConfigHome
	oldDataHome := xdg.DataHome
	oldStateHome := xdg.StateHome
	oldCacheHome := xdg.CacheHome
	oldRuntimeDir := xdg.RuntimeDir

	xdg.ConfigHome = tempDir
	xdg.DataHome = tempDir
	xdg.StateHome = tempDir
	xdg.CacheHome = tempDir
	xdg.RuntimeDir = tempDir

	t.Cleanup(func() {
		xdg.ConfigHome = oldConfigHome
		xdg.DataHome = oldDataHome
		xdg.StateHome = oldStateHome
		xdg.CacheHome = oldCacheHome
		xdg.RuntimeDir = oldRuntimeDir
		state.CloseDB()
	})

	t.Setenv("APPDATA", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_DATA_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", tempDir)
	t.Setenv("XDG_RUNTIME_DIR", tempDir)
	t.Setenv("HOME", tempDir)

	return tempDir
}
