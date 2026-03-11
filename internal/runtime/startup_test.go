package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/state"
)

func TestInitializeState_WithNilSettings(t *testing.T) {
	setupRuntimeTestEnv(t)
	state.CloseDB()
	t.Cleanup(state.CloseDB)

	if err := InitializeState(nil); err != nil {
		t.Fatalf("InitializeState(nil) error = %v", err)
	}

	for _, dir := range []string{
		config.GetSurgeDir(),
		config.GetStateDir(),
		config.GetRuntimeDir(),
		config.GetLogsDir(),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %q to exist, err=%v", dir, err)
		}
	}

	if _, err := state.GetDB(); err != nil {
		t.Fatalf("state.GetDB() error = %v", err)
	}

	dbPath := filepath.Join(config.GetStateDir(), "surge.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected database file at %q: %v", dbPath, err)
	}
}

func TestInitializeState_ReturnsEnsureDirsError(t *testing.T) {
	state.CloseDB()
	t.Cleanup(state.CloseDB)

	badPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", badPath)
	t.Setenv("XDG_STATE_HOME", badPath)
	t.Setenv("XDG_RUNTIME_DIR", badPath)
	t.Setenv("APPDATA", badPath)

	if err := InitializeState(nil); err == nil {
		t.Fatal("expected InitializeState(nil) to fail when EnsureDirs cannot create directories")
	}
}
