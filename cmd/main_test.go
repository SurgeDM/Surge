package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/state"
)

func resetSharedStateDB() error {
	// Reset any pre-existing global DB state (e.g. left by an init or an
	// isolated test cleanup) before pointing the package at the shared suite DB.
	state.CloseDB()
	err := config.EnsureDirs()
	state.Configure(filepath.Join(config.GetStateDir(), "surge.db"))
	return err
}

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

		if ensureErr := resetSharedStateDB(); ensureErr != nil {
			fmt.Fprintf(os.Stderr, "TestMain: failed to create isolated Surge test directories: %v\n", ensureErr)
			os.Exit(1)
		}
	}

	code := m.Run()

	if err == nil {
		state.CloseDB()
		_ = os.RemoveAll(tmpDir)
	}
	os.Exit(code)
}
