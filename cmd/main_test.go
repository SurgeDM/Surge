package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/state"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "surge-cmd-test-*")
	if err == nil {
		_ = os.Setenv("XDG_CONFIG_HOME", tmpDir)
		_ = os.Setenv("XDG_DATA_HOME", tmpDir)
		_ = os.Setenv("XDG_STATE_HOME", tmpDir)
		_ = os.Setenv("XDG_CACHE_HOME", tmpDir)
		_ = os.Setenv("XDG_RUNTIME_DIR", tmpDir)
		_ = os.Setenv("HOME", tmpDir)
		_ = os.Setenv("APPDATA", tmpDir)
		_ = os.Setenv("USERPROFILE", tmpDir)

		if ensureErr := config.EnsureDirs(); ensureErr == nil {
			state.CloseDB()
			state.Configure(filepath.Join(config.GetStateDir(), "surge.db"))
		}
	}

	code := m.Run()

	if err == nil {
		state.CloseDB()
		_ = os.RemoveAll(tmpDir)
	}
	os.Exit(code)
}
