package testutil

import (
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/engine/state"
)

// SetupStateDB configures a fresh temp SQLite DB for tests that exercise state persistence.
func SetupStateDB(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	state.CloseDB()
	state.Configure(filepath.Join(tempDir, "surge.db"))
	if _, err := state.GetDB(); err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	t.Cleanup(state.CloseDB)
	return tempDir
}
