package testutil

import (
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// SuppressNotificationsInTests disables desktop notifications for any test
// that imports this package. Call this from a per-package init() or TestMain.
func SuppressNotificationsInTests() {
	utils.SuppressNotifications = true
}

func init() {
	SuppressNotificationsInTests()
}

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

// SeedMasterList inserts a DownloadEntry into the master list for test setups.
func SeedMasterList(t *testing.T, entry types.DownloadEntry) {
	t.Helper()
	if err := state.AddToMasterList(entry); err != nil {
		t.Fatalf("SeedMasterList failed: %v", err)
	}
}
