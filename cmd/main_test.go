// lint:ignore-leak-check
package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/utils"
	"go.uber.org/goleak"
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
		resetGlobalShutdownCoordinatorForTest(nil)
		_ = executeGlobalShutdown("TestMain cleanup")
		store.CloseDB()
		_ = os.RemoveAll(tmpDir)
	}

	if code == 0 {
		if t, ok := http.DefaultTransport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
		transport.DefaultNetworkPool.CloseAll()
		if leakErr := goleak.Find(); leakErr != nil {
			fmt.Fprintf(os.Stderr, "goleak: Errors on successful test run: %v\n", leakErr)
			code = 1
		}
	}

	os.Exit(code)
}
