// lint:ignore-leak-check
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/utils"
)

func resetSharedStateDB() error {
	// Reset any pre-existing global DB state (e.g. left by an init or an
	// isolated test cleanup) before pointing the package at the shared suite DB.
	store.CloseDB()
	if err := config.EnsureDirs(); err != nil {
		return err
	}
	store.Configure(filepath.Join(config.GetStateDir(), "surge.db"))
	return nil
}

func TestMain(m *testing.M) {
	utils.SuppressNotifications = true
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
		_ = os.Setenv("SystemRoot", tmpDir)

		if ensureErr := resetSharedStateDB(); ensureErr != nil {
			fmt.Fprintf(os.Stderr, "TestMain: failed to create isolated Surge test directories: %v\n", ensureErr)
			_ = os.RemoveAll(tmpDir)
			os.Exit(1)
		}
	}

	code := m.Run()

	if err == nil {
		store.CloseDB()
		_ = os.RemoveAll(tmpDir)
	}
	os.Exit(code)
}
