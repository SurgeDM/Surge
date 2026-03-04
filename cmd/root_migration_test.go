package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
)

func TestInitializeGlobalState_MigratesLegacyStateDB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows keeps state with config")
	}

	configHome := t.TempDir()
	stateHome := t.TempDir()
	downloads := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	state.CloseDB()
	t.Cleanup(state.CloseDB)

	legacyDBPath := filepath.Join(config.GetSurgeDir(), "state", "surge.db")
	if err := os.MkdirAll(filepath.Dir(legacyDBPath), 0o755); err != nil {
		t.Fatalf("failed to create legacy db dir: %v", err)
	}

	state.Configure(legacyDBPath)

	id := "legacy-migrate-id"
	url := "https://example.com/legacy.bin"
	dest := filepath.Join(downloads, "legacy.bin")
	seed := &types.DownloadState{
		ID:         id,
		URL:        url,
		Filename:   "legacy.bin",
		DestPath:   dest,
		TotalSize:  1000,
		Downloaded: 250,
		CreatedAt:  time.Now().Unix(),
	}
	if err := state.SaveState(url, dest, seed); err != nil {
		t.Fatalf("failed to seed legacy db: %v", err)
	}
	state.CloseDB()

	if err := initializeGlobalState(); err != nil {
		t.Fatalf("initializeGlobalState failed: %v", err)
	}

	entry, err := state.GetDownload(id)
	if err != nil {
		t.Fatalf("failed to read migrated entry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected migrated legacy entry to exist in state db")
	}
	if entry.ID != id {
		t.Fatalf("migrated entry id = %q, want %q", entry.ID, id)
	}

	stateDBPath := filepath.Join(config.GetStateDir(), "surge.db")
	if _, err := os.Stat(stateDBPath); err != nil {
		t.Fatalf("expected state db at %s: %v", stateDBPath, err)
	}
}
